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
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/mux"

	httppkg "github.com/fatedier/frp/pkg/util/http"
	"github.com/fatedier/frp/pkg/util/log"
)

const (
	sessionCookie = "frp_session"
	stateCookie   = "frp_oidc_state"
	// csrfHeader must be sent with every request that changes something.
	// A cross-site form cannot set it, and a cross-site fetch that sets it
	// needs a CORS preflight that frps does not answer.
	csrfHeader = "X-FRP-Request"
)

type ctxKey struct{}

// SessionFrom returns the session the middleware attached to r.
func SessionFrom(r *http.Request) (*Session, bool) {
	s, ok := r.Context().Value(ctxKey{}).(*Session)
	return s, ok
}

// Actor returns the user name behind r, for audit records.
func Actor(r *http.Request) string {
	if s, ok := SessionFrom(r); ok {
		return s.Username
	}
	return ""
}

func secureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func writeJSON(w http.ResponseWriter, code int, msg string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(httppkg.V2Response{Code: code, Msg: msg, Data: data})
}

// Middleware requires a session, and the CSRF header on changes.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		var sess *Session
		ok := false
		if err == nil {
			sess, ok = s.Authenticate(c.Value)
		}
		if !ok {
			writeJSON(w, http.StatusUnauthorized, "未登录或登录已过期", nil)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get(csrfHeader) == "" {
				writeJSON(w, http.StatusForbidden, "missing "+csrfHeader+" header", nil)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess)))
	})
}

func (s *Service) setSessionCookie(w http.ResponseWriter, r *http.Request, sess *Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sess.Token,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HttpOnly: true,
		Secure:   secureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteLaxMode,
	})
}

// CallbackPath is where the OIDC provider sends the browser back, taken from
// the configured redirect URL.
func (s *Service) CallbackPath() string {
	if !s.cfg.OIDC.OIDCEnabled() {
		return ""
	}
	u, err := url.Parse(s.cfg.OIDC.RedirectURL)
	if err != nil {
		return ""
	}
	return u.Path
}

// RegisterPublic adds the routes that work without a session.
func (s *Service) RegisterPublic(r *mux.Router) {
	r.HandleFunc("/api/auth/config", s.handleConfig).Methods(http.MethodGet)
	r.HandleFunc("/api/auth/login", s.handleLogin).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/enroll", s.handleEnroll).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/logout", s.handleLogout).Methods(http.MethodPost)
	r.HandleFunc("/api/auth/me", s.handleMe).Methods(http.MethodGet)
	if s.cfg.OIDC.OIDCEnabled() {
		r.HandleFunc("/api/auth/oidc/login", s.handleOIDCLogin).Methods(http.MethodGet)
		if p := s.CallbackPath(); p != "" {
			r.HandleFunc(p, s.handleOIDCCallback).Methods(http.MethodGet)
		}
	}
}

// RegisterPrivate adds the routes that need a session.
func (s *Service) RegisterPrivate(r *mux.Router) {
	r.HandleFunc("/api/admin/users", httppkg.MakeHTTPHandlerFuncV2(s.apiUsers)).Methods(http.MethodGet)
	r.HandleFunc("/api/admin/users", httppkg.MakeHTTPHandlerFuncV2(s.apiAddUser)).Methods(http.MethodPost)
	r.HandleFunc("/api/admin/users/{name}", httppkg.MakeHTTPHandlerFuncV2(s.apiDeleteUser)).Methods(http.MethodDelete)
	r.HandleFunc("/api/admin/users/{name}/{action}", httppkg.MakeHTTPHandlerFuncV2(s.apiUserAction)).Methods(http.MethodPost)
	r.HandleFunc("/api/admin/local/recovery-codes", httppkg.MakeHTTPHandlerFuncV2(s.apiRegenerateRecovery)).Methods(http.MethodPost)
	r.HandleFunc("/api/admin/local/reset-2fa", httppkg.MakeHTTPHandlerFuncV2(s.apiResetLocal2FA)).Methods(http.MethodPost)
}

type authConfig struct {
	Enabled  bool   `json:"enabled"`
	OIDC     bool   `json:"oidc"`
	OIDCName string `json:"oidcName,omitempty"`
}

func (s *Service) handleConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, "success", authConfig{Enabled: true, OIDC: s.cfg.OIDC.OIDCEnabled(), OIDCName: s.cfg.OIDC.DisplayName})
}

type loginReq struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Code        string `json:"code"`
	EnrollToken string `json:"enrollToken"`
}

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	return json.NewDecoder(r.Body).Decode(v)
}

func loginError(w http.ResponseWriter, err error) {
	var le *LockedError
	switch {
	case errors.As(err, &le):
		writeJSON(w, http.StatusTooManyRequests, err.Error(), nil)
	case errors.Is(err, ErrBadCredentials), errors.Is(err, ErrBadCode):
		// Slow down guessing.
		time.Sleep(300 * time.Millisecond)
		writeJSON(w, http.StatusUnauthorized, err.Error(), nil)
	default:
		writeJSON(w, http.StatusBadRequest, err.Error(), nil)
	}
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(csrfHeader) == "" {
		writeJSON(w, http.StatusForbidden, "missing "+csrfHeader+" header", nil)
		return
	}
	var req loginReq
	if err := decode(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, "invalid request body", nil)
		return
	}
	res, err := s.LoginLocal(req.Username, req.Password, req.Code, ClientIP(r))
	if err != nil {
		loginError(w, err)
		return
	}
	if res.Session != nil {
		s.setSessionCookie(w, r, res.Session)
	}
	writeJSON(w, http.StatusOK, "success", res)
}

func (s *Service) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(csrfHeader) == "" {
		writeJSON(w, http.StatusForbidden, "missing "+csrfHeader+" header", nil)
		return
	}
	var req loginReq
	if err := decode(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, "invalid request body", nil)
		return
	}
	res, err := s.ConfirmEnroll(req.EnrollToken, req.Code, ClientIP(r))
	if err != nil {
		loginError(w, err)
		return
	}
	s.setSessionCookie(w, r, res.Session)
	writeJSON(w, http.StatusOK, "success", res)
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if sess, ok := s.Authenticate(c.Value); ok {
			s.record(sess.Username, "auth.logout", sess.Username, "", ClientIP(r), true)
		}
		s.Logout(c.Value)
	}
	clearCookie(w, r, sessionCookie)
	writeJSON(w, http.StatusOK, "success", nil)
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, "未登录", nil)
		return
	}
	sess, ok := s.Authenticate(c.Value)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, "未登录或登录已过期", nil)
		return
	}
	writeJSON(w, http.StatusOK, "success", sess)
}

func (s *Service) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	authURL, state, err := s.StartOIDC(r.Context())
	if err != nil {
		log.Warnf("start OIDC login: %v", err)
		redirectLogin(w, r, "error", "无法连接 SSO 服务："+err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/", MaxAge: int(oidcFlowTTL.Seconds()),
		HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, authURL, http.StatusFound)
}

// redirectLogin sends the browser to the dashboard's login page with the
// outcome of an OIDC sign-in.
func redirectLogin(w http.ResponseWriter, r *http.Request, status, msg string) {
	q := url.Values{}
	q.Set("sso", status)
	if msg != "" {
		q.Set("msg", msg)
	}
	http.Redirect(w, r, "/static/#/login?"+q.Encode(), http.StatusFound)
}

func (s *Service) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clearCookie(w, r, stateCookie)
	if e := q.Get("error"); e != "" {
		msg := e
		if d := q.Get("error_description"); d != "" {
			msg += ": " + d
		}
		s.record("", "auth.oidc.login", "", msg, ClientIP(r), false)
		redirectLogin(w, r, "error", "SSO 登录失败："+msg)
		return
	}
	cookieState := ""
	if c, err := r.Cookie(stateCookie); err == nil {
		cookieState = c.Value
	}
	out, err := s.FinishOIDC(r.Context(), q.Get("state"), cookieState, q.Get("code"), ClientIP(r))
	if err != nil {
		log.Warnf("OIDC login: %v", err)
		redirectLogin(w, r, "error", err.Error())
		return
	}
	switch out.Status {
	case "ok":
		s.setSessionCookie(w, r, out.Session)
		http.Redirect(w, r, "/static/#/", http.StatusFound)
	case "pending":
		redirectLogin(w, r, "pending", out.Username)
	default:
		redirectLogin(w, r, out.Status, out.Username)
	}
}

// ---- private APIs ----

func actorAndIP(ctx *httppkg.Context) (string, string) {
	return Actor(ctx.Req), ClientIP(ctx.Req)
}

func apiError(err error) error {
	switch {
	case errors.Is(err, errUserNotFound):
		return httppkg.NewError(http.StatusNotFound, err.Error())
	default:
		return httppkg.NewError(http.StatusBadRequest, err.Error())
	}
}

func (s *Service) apiUsers(*httppkg.Context) (any, error) {
	return s.Users(), nil
}

type addUserReq struct {
	Username string `json:"username"`
	Note     string `json:"note"`
}

func (s *Service) apiAddUser(ctx *httppkg.Context) (any, error) {
	var req addUserReq
	if err := ctx.BindJSON(&req); err != nil {
		return nil, httppkg.NewError(http.StatusBadRequest, "invalid request body")
	}
	actor, ip := actorAndIP(ctx)
	u, err := s.AddUser(actor, req.Username, req.Note, ip)
	if err != nil {
		return nil, apiError(err)
	}
	return u, nil
}

func (s *Service) apiUserAction(ctx *httppkg.Context) (any, error) {
	actor, ip := actorAndIP(ctx)
	name := ctx.Param("name")
	var (
		u   User
		err error
	)
	switch ctx.Param("action") {
	case "approve", "enable":
		u, err = s.SetUserStatus(actor, name, UserApproved, ip)
	case "reject":
		u, err = s.SetUserStatus(actor, name, UserRejected, ip)
	case "disable":
		u, err = s.SetUserStatus(actor, name, UserDisabled, ip)
	case "note":
		var req addUserReq
		if berr := ctx.BindJSON(&req); berr != nil {
			return nil, httppkg.NewError(http.StatusBadRequest, "invalid request body")
		}
		u, err = s.SetUserNote(actor, name, req.Note, ip)
	default:
		return nil, httppkg.NewError(http.StatusNotFound, "unknown action")
	}
	if err != nil {
		return nil, apiError(err)
	}
	return u, nil
}

func (s *Service) apiDeleteUser(ctx *httppkg.Context) (any, error) {
	actor, ip := actorAndIP(ctx)
	if err := s.DeleteUser(actor, ctx.Param("name"), ip); err != nil {
		return nil, apiError(err)
	}
	return map[string]string{"username": ctx.Param("name")}, nil
}

func (s *Service) apiRegenerateRecovery(ctx *httppkg.Context) (any, error) {
	sess, ok := SessionFrom(ctx.Req)
	if !ok || sess.Source != SourceLocal {
		return nil, httppkg.NewError(http.StatusForbidden, "只有本地账号可以重新生成自己的恢复密钥")
	}
	codes, err := s.RegenerateRecovery(sess.Username, ClientIP(ctx.Req))
	if err != nil {
		return nil, apiError(err)
	}
	return map[string]any{"recoveryCodes": codes}, nil
}

func (s *Service) apiResetLocal2FA(ctx *httppkg.Context) (any, error) {
	actor, ip := actorAndIP(ctx)
	if err := s.ResetLocal2FA(actor, ip); err != nil {
		return nil, apiError(err)
	}
	return map[string]string{"username": s.cfg.LocalUser.Username}, nil
}
