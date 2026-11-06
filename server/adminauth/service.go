// Copyright 2026 The frp Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adminauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/util/log"
	"github.com/fatedier/frp/server/audit"
)

// Source tells how a session signed in.
const (
	SourceLocal = "local"
	SourceOIDC  = "oidc"
)

// Session is a signed-in dashboard user.
type Session struct {
	Token     string    `json:"-"`
	Username  string    `json:"username"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	IP        string    `json:"-"`
}

type enrollment struct {
	secret  string
	expires time.Time
}

type oidcFlow struct {
	nonce    string
	verifier string
	expires  time.Time
}

type failure struct {
	count int
	first time.Time
	until time.Time
}

const (
	maxFailures     = 5
	failureWindow   = 15 * time.Minute
	lockoutDuration = 15 * time.Minute
	enrollTTL       = 10 * time.Minute
	oidcFlowTTL     = 10 * time.Minute
	recoveryCount   = 10
)

// Service holds the accounts, the sessions and the OIDC client.
type Service struct {
	cfg   v1.AdminAuthConfig
	store *store
	audit *audit.Logger
	ttl   time.Duration
	now   func() time.Time

	mu       sync.Mutex
	sessions map[string]*Session
	enrolls  map[string]*enrollment
	flows    map[string]*oidcFlow
	failures map[string]*failure

	oidcMu       sync.Mutex
	oidcProvider *oidc.Provider
	oidcHTTP     *http.Client
}

// New opens the account store. The OIDC provider is discovered on first use.
func New(cfg v1.AdminAuthConfig, auditLog *audit.Logger) (*Service, error) {
	st, err := openStore(cfg.StorePath)
	if err != nil {
		return nil, err
	}
	s := &Service{
		cfg:      cfg,
		store:    st,
		audit:    auditLog,
		ttl:      time.Duration(cfg.SessionTTLHours) * time.Hour,
		now:      time.Now,
		sessions: map[string]*Session{},
		enrolls:  map[string]*enrollment{},
		flows:    map[string]*oidcFlow{},
		failures: map[string]*failure{},
	}
	if s.ttl <= 0 {
		s.ttl = 12 * time.Hour
	}
	if cfg.OIDC.OIDCEnabled() {
		s.oidcHTTP, err = oidcHTTPClient(cfg.OIDC)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

func oidcHTTPClient(c v1.AdminOIDCConfig) (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.TrustedCaFile != "" {
		pem, err := os.ReadFile(c.TrustedCaFile)
		if err != nil {
			return nil, fmt.Errorf("read oidc trustedCaFile: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("oidc trustedCaFile has no certificate")
		}
		tlsCfg.RootCAs = pool
	}
	if c.InsecureSkipVerify {
		tlsCfg.InsecureSkipVerify = true //nolint:gosec // opted in by configuration
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = tlsCfg
	return &http.Client{Transport: tr, Timeout: 15 * time.Second}, nil
}

func randToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (s *Service) record(actor, action, target, detail, ip string, ok bool) {
	s.audit.Record(audit.Entry{Actor: actor, Action: action, Target: target, Detail: detail, IP: ip, Success: ok})
}

// LocalUsername is the name of the local account.
func (s *Service) LocalUsername() string { return s.cfg.LocalUser.Username }

func (s *Service) checkPassword(password string) bool {
	if h := s.cfg.LocalUser.PasswordHash; h != "" {
		return bcrypt.CompareHashAndPassword([]byte(h), []byte(password)) == nil
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(s.cfg.LocalUser.Password)) == 1
}

// throttled reports whether key is locked out after failures.
func (s *Service) throttled(key string) (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.failures[key]
	if !ok {
		return 0, false
	}
	now := s.now()
	if now.Before(f.until) {
		return f.until.Sub(now), true
	}
	return 0, false
}

func (s *Service) fail(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	f, ok := s.failures[key]
	if !ok || now.Sub(f.first) > failureWindow {
		f = &failure{first: now}
		s.failures[key] = f
	}
	f.count++
	if f.count >= maxFailures {
		f.until = now.Add(lockoutDuration)
		f.count = 0
		f.first = now
	}
}

func (s *Service) succeed(key string) {
	s.mu.Lock()
	delete(s.failures, key)
	s.mu.Unlock()
}

// LoginResult is the outcome of a local login step.
type LoginResult struct {
	// Status is "ok", "totp_required" or "enroll".
	Status      string   `json:"status"`
	EnrollToken string   `json:"enrollToken,omitempty"`
	Secret      string   `json:"secret,omitempty"`
	OTPAuthURL  string   `json:"otpauthURL,omitempty"`
	Recovery    []string `json:"recoveryCodes,omitempty"`
	Session     *Session `json:"session,omitempty"`
	// RecoveryUsed is set when a recovery code signed in; RecoveryLeft says
	// how many remain.
	RecoveryUsed bool `json:"recoveryUsed,omitempty"`
	RecoveryLeft int  `json:"recoveryLeft,omitempty"`
}

// ErrBadCredentials is returned for a wrong user name or password.
var ErrBadCredentials = errors.New("用户名或密码错误")

// ErrBadCode is returned for a wrong second factor.
var ErrBadCode = errors.New("动态验证码或恢复密钥错误")

// LockedError is returned while a login is locked out.
type LockedError struct{ Wait time.Duration }

func (e *LockedError) Error() string {
	return fmt.Sprintf("失败次数过多，请 %d 分钟后再试", int(e.Wait.Minutes())+1)
}

// LoginLocal checks the password and the second factor. The first login
// after deployment returns "enroll" with a new TOTP secret to confirm with
// ConfirmEnroll; later logins need a TOTP code or a recovery code.
func (s *Service) LoginLocal(username, password, code, ip string) (*LoginResult, error) {
	key := "local|" + ip
	if wait, locked := s.throttled(key); locked {
		return nil, &LockedError{Wait: wait}
	}
	if userKey(username) != userKey(s.cfg.LocalUser.Username) || !s.checkPassword(password) {
		s.fail(key)
		s.record(username, "auth.local.login", username, "wrong user name or password", ip, false)
		return nil, ErrBadCredentials
	}
	local := s.store.local()
	if !local.TOTPEnabled {
		secret, err := newTOTPSecret()
		if err != nil {
			return nil, err
		}
		token := randToken(16)
		s.mu.Lock()
		s.enrolls[token] = &enrollment{secret: secret, expires: s.now().Add(enrollTTL)}
		s.mu.Unlock()
		s.record(s.cfg.LocalUser.Username, "auth.local.enroll_start", s.cfg.LocalUser.Username, "first login, second factor not set up", ip, true)
		return &LoginResult{
			Status:      "enroll",
			EnrollToken: token,
			Secret:      secret,
			OTPAuthURL:  otpauthURL(s.cfg.TOTPIssuer, s.cfg.LocalUser.Username, secret),
		}, nil
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return &LoginResult{Status: "totp_required"}, nil
	}

	var (
		usedRecovery bool
		left         int
	)
	err := s.store.update(func(d *storeData) error {
		if step, ok := verifyTOTP(d.Local.TOTPSecret, code, s.now(), d.Local.LastTOTPStep); ok {
			d.Local.LastTOTPStep = step
		} else {
			h := hashRecoveryCode(code)
			idx := slices.IndexFunc(d.Local.RecoveryHashes, func(x string) bool {
				return subtle.ConstantTimeCompare([]byte(x), []byte(h)) == 1
			})
			if idx < 0 {
				return ErrBadCode
			}
			d.Local.RecoveryHashes = slices.Delete(d.Local.RecoveryHashes, idx, idx+1)
			usedRecovery = true
			left = len(d.Local.RecoveryHashes)
		}
		d.Local.LastLoginAt = s.now()
		d.Local.LastLoginIP = ip
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrBadCode) {
			s.fail(key)
			s.record(s.cfg.LocalUser.Username, "auth.local.login", s.cfg.LocalUser.Username, "wrong second factor", ip, false)
		}
		return nil, err
	}
	s.succeed(key)
	detail := "TOTP"
	if usedRecovery {
		detail = fmt.Sprintf("recovery code, %d left", left)
	}
	s.record(s.cfg.LocalUser.Username, "auth.local.login", s.cfg.LocalUser.Username, detail, ip, true)
	sess := s.newSession(s.cfg.LocalUser.Username, SourceLocal, ip)
	return &LoginResult{Status: "ok", Session: sess, RecoveryUsed: usedRecovery, RecoveryLeft: left}, nil
}

// ConfirmEnroll turns on the second factor of the local user once the code
// from the authenticator app matches, and signs in.
func (s *Service) ConfirmEnroll(token, code, ip string) (*LoginResult, error) {
	key := "local|" + ip
	if wait, locked := s.throttled(key); locked {
		return nil, &LockedError{Wait: wait}
	}
	s.mu.Lock()
	e, ok := s.enrolls[token]
	if ok && s.now().After(e.expires) {
		delete(s.enrolls, token)
		ok = false
	}
	s.mu.Unlock()
	if !ok {
		return nil, errors.New("设置已过期，请重新登录")
	}
	step, valid := verifyTOTP(e.secret, code, s.now(), 0)
	if !valid {
		s.fail(key)
		return nil, errors.New("动态验证码错误，请确认手机时间准确")
	}
	codes, err := newRecoveryCodes(recoveryCount)
	if err != nil {
		return nil, err
	}
	err = s.store.update(func(d *storeData) error {
		if d.Local.TOTPEnabled {
			return errors.New("二次验证已设置，请重新登录")
		}
		d.Local = LocalState{
			TOTPSecret:   e.secret,
			TOTPEnabled:  true,
			LastTOTPStep: step,
			EnrolledAt:   s.now(),
			LastLoginAt:  s.now(),
			LastLoginIP:  ip,
		}
		for _, c := range codes {
			d.Local.RecoveryHashes = append(d.Local.RecoveryHashes, hashRecoveryCode(c))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	delete(s.enrolls, token)
	s.mu.Unlock()
	s.succeed(key)
	s.record(s.cfg.LocalUser.Username, "auth.local.enroll", s.cfg.LocalUser.Username, "second factor set up", ip, true)
	sess := s.newSession(s.cfg.LocalUser.Username, SourceLocal, ip)
	return &LoginResult{Status: "ok", Session: sess, Recovery: codes}, nil
}

// RegenerateRecovery replaces the recovery codes of the local user.
func (s *Service) RegenerateRecovery(actor, ip string) ([]string, error) {
	codes, err := newRecoveryCodes(recoveryCount)
	if err != nil {
		return nil, err
	}
	err = s.store.update(func(d *storeData) error {
		if !d.Local.TOTPEnabled {
			return errors.New("尚未设置二次验证")
		}
		d.Local.RecoveryHashes = nil
		for _, c := range codes {
			d.Local.RecoveryHashes = append(d.Local.RecoveryHashes, hashRecoveryCode(c))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.record(actor, "auth.local.recovery_regenerate", s.cfg.LocalUser.Username, "", ip, true)
	return codes, nil
}

// ResetLocal2FA clears the second factor of the local user, who sets it up
// again at the next login, and signs the local user out.
func (s *Service) ResetLocal2FA(actor, ip string) error {
	if err := s.store.update(func(d *storeData) error {
		d.Local.TOTPSecret = ""
		d.Local.TOTPEnabled = false
		d.Local.LastTOTPStep = 0
		d.Local.RecoveryHashes = nil
		d.Local.EnrolledAt = time.Time{}
		return nil
	}); err != nil {
		return err
	}
	s.revokeUser(s.cfg.LocalUser.Username, SourceLocal)
	s.record(actor, "auth.local.reset_2fa", s.cfg.LocalUser.Username, "", ip, true)
	return nil
}

func (s *Service) newSession(username, source, ip string) *Session {
	now := s.now()
	sess := &Session{
		Token:     randToken(32),
		Username:  username,
		Source:    source,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
		IP:        ip,
	}
	s.mu.Lock()
	s.sessions[sess.Token] = sess
	// Drop expired sessions, enrollments and OIDC flows.
	for k, v := range s.sessions {
		if now.After(v.ExpiresAt) {
			delete(s.sessions, k)
		}
	}
	for k, v := range s.enrolls {
		if now.After(v.expires) {
			delete(s.enrolls, k)
		}
	}
	for k, v := range s.flows {
		if now.After(v.expires) {
			delete(s.flows, k)
		}
	}
	s.mu.Unlock()
	return sess
}

// Authenticate returns the session of token while its user may still sign in.
func (s *Service) Authenticate(token string) (*Session, bool) {
	if token == "" {
		return nil, false
	}
	s.mu.Lock()
	sess, ok := s.sessions[token]
	if ok && s.now().After(sess.ExpiresAt) {
		delete(s.sessions, token)
		ok = false
	}
	s.mu.Unlock()
	if !ok {
		return nil, false
	}
	if sess.Source == SourceOIDC {
		u, found := s.store.user(sess.Username)
		if !found || u.Status != UserApproved {
			s.Logout(token)
			return nil, false
		}
	}
	cp := *sess
	return &cp, true
}

// Logout ends a session.
func (s *Service) Logout(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func (s *Service) revokeUser(username, source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.sessions {
		if v.Source == source && userKey(v.Username) == userKey(username) {
			delete(s.sessions, k)
		}
	}
}

// ---- OIDC ----

func (s *Service) provider(ctx context.Context) (*oidc.Provider, error) {
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	if s.oidcProvider != nil {
		return s.oidcProvider, nil
	}
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, s.oidcHTTP), s.cfg.OIDC.Issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider %s: %w", s.cfg.OIDC.Issuer, err)
	}
	s.oidcProvider = p
	return p, nil
}

func (s *Service) oauthConfig(p *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.cfg.OIDC.ClientID,
		ClientSecret: s.cfg.OIDC.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  s.cfg.OIDC.RedirectURL,
		Scopes:       s.cfg.OIDC.Scopes,
	}
}

// StartOIDC returns the provider URL to send the browser to, and the state
// to keep in a cookie.
func (s *Service) StartOIDC(ctx context.Context) (authURL, state string, err error) {
	p, err := s.provider(ctx)
	if err != nil {
		return "", "", err
	}
	state = randToken(16)
	flow := &oidcFlow{nonce: randToken(16), verifier: oauth2.GenerateVerifier(), expires: s.now().Add(oidcFlowTTL)}
	s.mu.Lock()
	s.flows[state] = flow
	s.mu.Unlock()
	authURL = s.oauthConfig(p).AuthCodeURL(state, oidc.Nonce(flow.nonce), oauth2.S256ChallengeOption(flow.verifier))
	return authURL, state, nil
}

// OIDCOutcome is what an OIDC sign-in led to.
type OIDCOutcome struct {
	// Status is "ok", "pending", "rejected" or "disabled".
	Status   string
	Username string
	Session  *Session
}

func claimString(claims map[string]any, name string) string {
	if v, ok := claims[name].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func (s *Service) usernameFrom(claims map[string]any) string {
	if c := s.cfg.OIDC.UsernameClaim; c != "" {
		return claimString(claims, c)
	}
	for _, c := range []string{"preferred_username", "username", "email"} {
		if v := claimString(claims, c); v != "" {
			return v
		}
	}
	return ""
}

// FinishOIDC exchanges the code, verifies the ID token and matches its user
// name. An unknown user name becomes a request that another administrator
// approves.
func (s *Service) FinishOIDC(ctx context.Context, state, cookieState, code, ip string) (*OIDCOutcome, error) {
	if state == "" || cookieState == "" || subtle.ConstantTimeCompare([]byte(state), []byte(cookieState)) != 1 {
		return nil, errors.New("登录状态无效，请重新登录")
	}
	s.mu.Lock()
	flow, ok := s.flows[state]
	delete(s.flows, state)
	s.mu.Unlock()
	if !ok || s.now().After(flow.expires) {
		return nil, errors.New("登录已过期，请重新登录")
	}
	p, err := s.provider(ctx)
	if err != nil {
		return nil, err
	}
	ctx = oidc.ClientContext(ctx, s.oidcHTTP)
	tok, err := s.oauthConfig(p).Exchange(ctx, code, oauth2.VerifierOption(flow.verifier))
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) {
			body := string(re.Body)
			if len(body) > 512 {
				body = body[:512]
			}
			log.Warnf("OIDC token endpoint answered %d: %s", re.Response.StatusCode, body)
		}
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	rawID, _ := tok.Extra("id_token").(string)
	if rawID == "" {
		return nil, errors.New("provider returned no id_token")
	}
	idt, err := p.Verifier(&oidc.Config{ClientID: s.cfg.OIDC.ClientID}).Verify(ctx, rawID)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}
	if idt.Nonce != flow.nonce {
		return nil, errors.New("id_token nonce mismatch")
	}
	claims := map[string]any{}
	if err := idt.Claims(&claims); err != nil {
		return nil, err
	}
	username := s.usernameFrom(claims)
	if username == "" {
		if ui, uerr := p.UserInfo(ctx, oauth2.StaticTokenSource(tok)); uerr == nil {
			extra := map[string]any{}
			if ui.Claims(&extra) == nil {
				for k, v := range extra {
					if _, exists := claims[k]; !exists {
						claims[k] = v
					}
				}
				username = s.usernameFrom(claims)
			}
		}
	}
	if username == "" {
		return nil, errors.New("OIDC 身份中没有用户名，请检查 usernameClaim 配置")
	}
	if userKey(username) == userKey(s.cfg.LocalUser.Username) {
		s.record(username, "auth.oidc.login", username, "user name of the local account", ip, false)
		return nil, errors.New("该用户名是本地账号，不能通过 SSO 登录")
	}
	return s.matchOIDCUser(username, claimString(claims, "email"), idt.Subject, ip)
}

func (s *Service) matchOIDCUser(username, email, subject, ip string) (*OIDCOutcome, error) {
	var outcome OIDCOutcome
	now := s.now()
	err := s.store.update(func(d *storeData) error {
		k := userKey(username)
		u, ok := d.Users[k]
		if !ok {
			d.Users[k] = &User{
				Username:    username,
				Status:      UserPending,
				Email:       email,
				Subject:     subject,
				RequestedAt: now,
				CreatedAt:   now,
				LastLoginIP: ip,
			}
			outcome = OIDCOutcome{Status: "pending", Username: username}
			return nil
		}
		if u.Subject != "" && subject != "" && u.Subject != subject {
			return fmt.Errorf("用户 %s 的 OIDC 身份与已登记的不一致", username)
		}
		if u.Subject == "" {
			u.Subject = subject
		}
		if email != "" {
			u.Email = email
		}
		switch u.Status {
		case UserApproved:
			u.LastLoginAt = now
			u.LastLoginIP = ip
			outcome = OIDCOutcome{Status: "ok", Username: u.Username}
		case UserPending:
			if u.RequestedAt.IsZero() {
				u.RequestedAt = now
			}
			outcome = OIDCOutcome{Status: "pending", Username: u.Username}
		case UserRejected:
			outcome = OIDCOutcome{Status: "rejected", Username: u.Username}
		default:
			outcome = OIDCOutcome{Status: "disabled", Username: u.Username}
		}
		return nil
	})
	if err != nil {
		s.record(username, "auth.oidc.login", username, err.Error(), ip, false)
		return nil, err
	}
	switch outcome.Status {
	case "ok":
		outcome.Session = s.newSession(outcome.Username, SourceOIDC, ip)
		s.record(outcome.Username, "auth.oidc.login", outcome.Username, "", ip, true)
	case "pending":
		s.record(outcome.Username, "user.request", outcome.Username, "waiting for approval by another administrator", ip, true)
	default:
		s.record(outcome.Username, "auth.oidc.login", outcome.Username, "user is "+outcome.Status, ip, false)
	}
	return &outcome, nil
}

// ---- users ----

// LocalInfo describes the local account for the dashboard.
type LocalInfo struct {
	Username     string    `json:"username"`
	TOTPEnabled  bool      `json:"totpEnabled"`
	EnrolledAt   time.Time `json:"enrolledAt,omitzero"`
	RecoveryLeft int       `json:"recoveryLeft"`
	LastLoginAt  time.Time `json:"lastLoginAt,omitzero"`
	LastLoginIP  string    `json:"lastLoginIP,omitempty"`
}

// UserList is the local account and the OIDC users.
type UserList struct {
	Local LocalInfo `json:"local"`
	OIDC  bool      `json:"oidcEnabled"`
	Users []User    `json:"users"`
}

// Users lists the accounts.
func (s *Service) Users() UserList {
	l := s.store.local()
	return UserList{
		Local: LocalInfo{
			Username:     s.cfg.LocalUser.Username,
			TOTPEnabled:  l.TOTPEnabled,
			EnrolledAt:   l.EnrolledAt,
			RecoveryLeft: len(l.RecoveryHashes),
			LastLoginAt:  l.LastLoginAt,
			LastLoginIP:  l.LastLoginIP,
		},
		OIDC:  s.cfg.OIDC.OIDCEnabled(),
		Users: s.store.users(),
	}
}

// PendingUsers counts the users waiting for approval.
func (s *Service) PendingUsers() int { return s.store.countPending() }

// AddUser registers an approved OIDC user name ahead of their first login.
func (s *Service) AddUser(actor, username, note, ip string) (User, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 128 || strings.ContainsAny(username, " \t\r\n") {
		return User{}, errors.New("用户名无效")
	}
	if userKey(username) == userKey(s.cfg.LocalUser.Username) {
		return User{}, errors.New("不能与本地账号同名")
	}
	now := s.now()
	u := User{Username: username, Status: UserApproved, Note: strings.TrimSpace(note), CreatedAt: now, DecidedBy: actor, DecidedAt: now}
	err := s.store.update(func(d *storeData) error {
		if _, ok := d.Users[userKey(username)]; ok {
			return errUserExists
		}
		cp := u
		d.Users[userKey(username)] = &cp
		return nil
	})
	if err != nil {
		return User{}, err
	}
	s.record(actor, "user.add", username, u.Note, ip, true)
	return u, nil
}

// SetUserStatus approves, rejects, disables or enables a user. Users other
// than approved ones are signed out.
func (s *Service) SetUserStatus(actor, username string, status UserStatus, ip string) (User, error) {
	if userKey(actor) == userKey(username) && status != UserApproved {
		return User{}, errors.New("不能停用自己的账号")
	}
	var out User
	err := s.store.update(func(d *storeData) error {
		u, ok := d.Users[userKey(username)]
		if !ok {
			return errUserNotFound
		}
		u.Status = status
		u.DecidedBy = actor
		u.DecidedAt = s.now()
		out = *u
		return nil
	})
	if err != nil {
		return User{}, err
	}
	if status != UserApproved {
		s.revokeUser(username, SourceOIDC)
	}
	s.record(actor, "user."+string(status), out.Username, "", ip, true)
	return out, nil
}

// SetUserNote changes the note of a user.
func (s *Service) SetUserNote(actor, username, note, ip string) (User, error) {
	var out User
	err := s.store.update(func(d *storeData) error {
		u, ok := d.Users[userKey(username)]
		if !ok {
			return errUserNotFound
		}
		u.Note = strings.TrimSpace(note)
		out = *u
		return nil
	})
	if err == nil {
		s.record(actor, "user.note", out.Username, out.Note, ip, true)
	}
	return out, err
}

// DeleteUser removes a user and signs them out.
func (s *Service) DeleteUser(actor, username, ip string) error {
	if userKey(actor) == userKey(username) {
		return errors.New("不能删除自己的账号")
	}
	err := s.store.update(func(d *storeData) error {
		if _, ok := d.Users[userKey(username)]; !ok {
			return errUserNotFound
		}
		delete(d.Users, userKey(username))
		return nil
	})
	if err != nil {
		return err
	}
	s.revokeUser(username, SourceOIDC)
	s.record(actor, "user.delete", username, "", ip, true)
	return nil
}

// ClientIP returns the client address of r.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
