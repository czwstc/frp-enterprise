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
	"crypto/rand"
	"crypto/rsa"
	"encoding/base32"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/server/audit"
)

func TestTOTPRFC6238(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	// RFC 6238 appendix B, SHA1, truncated to 6 digits.
	cases := map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"}
	for ts, want := range cases {
		got, err := totpAt(secret, totpStep(time.Unix(ts, 0)))
		require.NoError(t, err)
		require.Equal(t, want, got, "t=%d", ts)
	}
	step, ok := verifyTOTP(secret, "287082", time.Unix(59, 0), 0)
	require.True(t, ok)
	_, ok = verifyTOTP(secret, "287082", time.Unix(59, 0), step)
	require.False(t, ok, "a code is used once")
	_, ok = verifyTOTP(secret, "000000", time.Unix(59, 0), 0)
	require.False(t, ok)
	require.Contains(t, otpauthURL("FRP Enterprise", "admin", secret), "otpauth://totp/FRP%20Enterprise:admin?")
}

func newTestService(t *testing.T, oidcCfg v1.AdminOIDCConfig) (*Service, *audit.Logger) {
	t.Helper()
	dir := t.TempDir()
	a, err := audit.Open(filepath.Join(dir, "audit.log"))
	require.NoError(t, err)
	t.Cleanup(func() { a.Close() })
	cfg := v1.AdminAuthConfig{
		Enabled:         true,
		StorePath:       filepath.Join(dir, "admin.json"),
		SessionTTLHours: 1,
		TOTPIssuer:      "FRP Enterprise",
		LocalUser:       v1.AdminLocalUserConfig{Username: "admin", Password: "correct-horse"},
		OIDC:            oidcCfg,
	}
	s, err := New(cfg, a)
	require.NoError(t, err)
	return s, a
}

func codeAt(t *testing.T, secret string, ts time.Time) string {
	c, err := totpAt(secret, totpStep(ts))
	require.NoError(t, err)
	return c
}

func TestLocalLoginEnrollTOTPAndRecovery(t *testing.T) {
	s, a := newTestService(t, v1.AdminOIDCConfig{})
	now := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return now }

	_, err := s.LoginLocal("admin", "wrong", "", "1.1.1.1")
	require.ErrorIs(t, err, ErrBadCredentials)
	_, err = s.LoginLocal("root", "correct-horse", "", "1.1.1.1")
	require.ErrorIs(t, err, ErrBadCredentials)

	// First login after deployment sets up the second factor.
	res, err := s.LoginLocal("ADMIN", "correct-horse", "", "1.1.1.1")
	require.NoError(t, err)
	require.Equal(t, "enroll", res.Status)
	require.NotEmpty(t, res.Secret)
	require.Nil(t, res.Session)
	_, err = s.ConfirmEnroll(res.EnrollToken, "000000", "1.1.1.1")
	require.Error(t, err)
	done, err := s.ConfirmEnroll(res.EnrollToken, codeAt(t, res.Secret, now), "1.1.1.1")
	require.NoError(t, err)
	require.NotNil(t, done.Session)
	require.Len(t, done.Recovery, recoveryCount)
	_, err = s.ConfirmEnroll(res.EnrollToken, codeAt(t, res.Secret, now), "1.1.1.1")
	require.Error(t, err, "an enrollment token is used once")

	// Later logins need the second factor.
	res, err = s.LoginLocal("admin", "correct-horse", "", "1.1.1.1")
	require.NoError(t, err)
	require.Equal(t, "totp_required", res.Status)
	// The code used to enroll cannot be used again.
	secret := s.store.local().TOTPSecret
	_, err = s.LoginLocal("admin", "correct-horse", codeAt(t, secret, now), "1.1.1.1")
	require.ErrorIs(t, err, ErrBadCode)

	now = now.Add(30 * time.Second)
	res, err = s.LoginLocal("admin", "correct-horse", codeAt(t, secret, now), "1.1.1.1")
	require.NoError(t, err)
	require.Equal(t, "ok", res.Status)

	// A recovery code works once.
	res, err = s.LoginLocal("admin", "correct-horse", strings.ToUpper(strings.ReplaceAll(done.Recovery[3], "-", "")), "1.1.1.1")
	require.NoError(t, err)
	require.True(t, res.RecoveryUsed)
	require.Equal(t, recoveryCount-1, res.RecoveryLeft)
	_, err = s.LoginLocal("admin", "correct-horse", done.Recovery[3], "1.1.1.1")
	require.ErrorIs(t, err, ErrBadCode)

	// Sessions authenticate until they expire.
	sess, ok := s.Authenticate(res.Session.Token)
	require.True(t, ok)
	require.Equal(t, SourceLocal, sess.Source)
	now = now.Add(2 * time.Hour)
	_, ok = s.Authenticate(res.Session.Token)
	require.False(t, ok)

	// Reset makes the next login enroll again.
	require.NoError(t, s.ResetLocal2FA("alice", "1.1.1.1"))
	res, err = s.LoginLocal("admin", "correct-horse", "", "1.1.1.1")
	require.NoError(t, err)
	require.Equal(t, "enroll", res.Status)

	page := a.List(audit.Query{Action: "auth.local"})
	require.Greater(t, page.Total, 5)
}

func TestLocalLoginLockout(t *testing.T) {
	s, _ := newTestService(t, v1.AdminOIDCConfig{})
	for range maxFailures {
		_, err := s.LoginLocal("admin", "nope", "", "2.2.2.2")
		require.ErrorIs(t, err, ErrBadCredentials)
	}
	_, err := s.LoginLocal("admin", "correct-horse", "", "2.2.2.2")
	var le *LockedError
	require.ErrorAs(t, err, &le)
	// Another address is not locked.
	_, err = s.LoginLocal("admin", "correct-horse", "", "3.3.3.3")
	require.NoError(t, err)
}

// fakeProvider is an OIDC provider for the authorization code flow.
type fakeProvider struct {
	srv      *httptest.Server
	key      *rsa.PrivateKey
	mu       sync.Mutex
	codes    map[string]map[string]any // code -> claims
	username string
	subject  string
}

func newFakeProvider(t *testing.T) *fakeProvider {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	p := &fakeProvider{key: key, codes: map[string]map[string]any{}, subject: "sub-1"}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                p.srv.URL,
			"authorization_endpoint":                p.srv.URL + "/authorize",
			"token_endpoint":                        p.srv.URL + "/token",
			"jwks_uri":                              p.srv.URL + "/jwks",
			"userinfo_endpoint":                     p.srv.URL + "/userinfo",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		p.mu.Lock()
		claims, ok := p.codes[r.Form.Get("code")]
		delete(p.codes, r.Form.Get("code"))
		p.mu.Unlock()
		if !ok || r.Form.Get("code_verifier") == "" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
		raw, _ := jwt.Signed(signer).Claims(claims).Serialize()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": raw, "expires_in": 3600})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

// authorize plays the provider's login page: it issues a code for the
// auth URL and returns the callback query.
func (p *fakeProvider) authorize(t *testing.T, authURL string) url.Values {
	u, err := url.Parse(authURL)
	require.NoError(t, err)
	q := u.Query()
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	code := randToken(8)
	claims := map[string]any{
		"iss": p.srv.URL, "aud": q.Get("client_id"), "sub": p.subject,
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
		"nonce": q.Get("nonce"), "username": p.username, "email": p.username + "@example.com",
	}
	p.mu.Lock()
	p.codes[code] = claims
	p.mu.Unlock()
	return url.Values{"code": {code}, "state": {q.Get("state")}}
}

func TestOIDCLoginNeedsApprovalByAnotherAdmin(t *testing.T) {
	p := newFakeProvider(t)
	s, _ := newTestService(t, v1.AdminOIDCConfig{
		Issuer: p.srv.URL, ClientID: "frps", ClientSecret: "secret",
		RedirectURL: "http://127.0.0.1:30001/relay", Scopes: []string{"openid", "email"},
	})
	require.Equal(t, "/relay", s.CallbackPath())

	login := func(username string) (*OIDCOutcome, error) {
		p.username = username
		authURL, state, err := s.StartOIDC(t.Context())
		require.NoError(t, err)
		cb := p.authorize(t, authURL)
		return s.FinishOIDC(t.Context(), cb.Get("state"), state, cb.Get("code"), "10.0.0.1")
	}

	// An unknown user name waits for approval.
	out, err := login("Alice")
	require.NoError(t, err)
	require.Equal(t, "pending", out.Status)
	require.Nil(t, out.Session)
	require.Equal(t, 1, s.PendingUsers())
	out, err = login("alice")
	require.NoError(t, err)
	require.Equal(t, "pending", out.Status)

	_, err = s.SetUserStatus("admin", "alice", UserApproved, "1.1.1.1")
	require.NoError(t, err)
	out, err = login("ALICE")
	require.NoError(t, err)
	require.Equal(t, "ok", out.Status)
	require.Equal(t, "Alice", out.Username)
	_, ok := s.Authenticate(out.Session.Token)
	require.True(t, ok)

	// A user pre-registered by name signs in at once.
	_, err = s.AddUser("Alice", "bob", "ops", "1.1.1.1")
	require.NoError(t, err)
	p.subject = "sub-2"
	out, err = login("bob")
	require.NoError(t, err)
	require.Equal(t, "ok", out.Status)

	// Another identity with a known user name is refused.
	p.subject = "sub-3"
	_, err = login("bob")
	require.Error(t, err)
	p.subject = "sub-2"

	// Disabling signs the user out at once.
	_, err = s.SetUserStatus("Alice", "bob", UserDisabled, "1.1.1.1")
	require.NoError(t, err)
	_, ok = s.Authenticate(out.Session.Token)
	require.False(t, ok)
	out, err = login("bob")
	require.NoError(t, err)
	require.Equal(t, "disabled", out.Status)

	// The local account cannot sign in through OIDC, nor be disabled by itself.
	p.subject = "sub-9"
	_, err = login("admin")
	require.Error(t, err)
	_, err = s.SetUserStatus("Alice", "alice", UserDisabled, "1.1.1.1")
	require.Error(t, err)

	// A wrong state is refused.
	authURL, _, err := s.StartOIDC(t.Context())
	require.NoError(t, err)
	cb := p.authorize(t, authURL)
	_, err = s.FinishOIDC(t.Context(), cb.Get("state"), "other", cb.Get("code"), "10.0.0.1")
	require.Error(t, err)
}

func TestHTTPFlow(t *testing.T) {
	p := newFakeProvider(t)
	s, _ := newTestService(t, v1.AdminOIDCConfig{
		Issuer: p.srv.URL, ClientID: "frps", ClientSecret: "secret",
		RedirectURL: "http://example.invalid/sso/callback", Scopes: []string{"openid"}, DisplayName: "NAS",
	})
	r := mux.NewRouter()
	s.RegisterPublic(r)
	private := r.NewRoute().Subrouter()
	private.Use(s.Middleware)
	s.RegisterPrivate(private)
	srv := httptest.NewServer(r)
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	do := func(method, path, body string, csrf bool) (int, map[string]any) {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if csrf {
			req.Header.Set(csrfHeader, "1")
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		var env map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&env)
		return resp.StatusCode, env
	}

	code, env := do("GET", "/api/auth/config", "", false)
	require.Equal(t, 200, code)
	require.Equal(t, "NAS", env["data"].(map[string]any)["oidcName"])

	code, _ = do("GET", "/api/admin/users", "", false)
	require.Equal(t, 401, code)

	code, _ = do("POST", "/api/auth/login", `{"username":"admin","password":"correct-horse"}`, false)
	require.Equal(t, 403, code, "login needs the CSRF header")
	code, env = do("POST", "/api/auth/login", `{"username":"admin","password":"correct-horse"}`, true)
	require.Equal(t, 200, code)
	data := env["data"].(map[string]any)
	require.Equal(t, "enroll", data["status"])
	totp := codeAt(t, data["secret"].(string), time.Now())
	code, env = do("POST", "/api/auth/enroll", `{"enrollToken":"`+data["enrollToken"].(string)+`","code":"`+totp+`"}`, true)
	require.Equal(t, 200, code, env)
	require.Len(t, env["data"].(map[string]any)["recoveryCodes"], recoveryCount)

	code, env = do("GET", "/api/auth/me", "", false)
	require.Equal(t, 200, code)
	require.Equal(t, "admin", env["data"].(map[string]any)["username"])

	code, _ = do("POST", "/api/admin/users", `{"username":"carol"}`, false)
	require.Equal(t, 403, code, "changes need the CSRF header")
	code, _ = do("POST", "/api/admin/users", `{"username":"carol"}`, true)
	require.Equal(t, 200, code)
	code, env = do("GET", "/api/admin/users", "", false)
	require.Equal(t, 200, code)
	require.Len(t, env["data"].(map[string]any)["users"], 1)

	// The OIDC callback is served at the redirect URL's path.
	resp, err := c.Get(srv.URL + "/api/auth/oidc/login")
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	p.username = "carol"
	cb := p.authorize(t, resp.Header.Get("Location"))
	resp, err = c.Get(srv.URL + "/sso/callback?" + cb.Encode())
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/static/#/", resp.Header.Get("Location"))
	code, env = do("GET", "/api/auth/me", "", false)
	require.Equal(t, 200, code)
	require.Equal(t, "carol", env["data"].(map[string]any)["username"])

	// An unknown user is sent to the login page as pending.
	p.username = "dave"
	resp, _ = c.Get(srv.URL + "/api/auth/oidc/login")
	cb = p.authorize(t, resp.Header.Get("Location"))
	resp, _ = c.Get(srv.URL + "/sso/callback?" + cb.Encode())
	require.Contains(t, resp.Header.Get("Location"), "sso=pending")

	code, _ = do("POST", "/api/auth/logout", "", true)
	require.Equal(t, 200, code)
	code, _ = do("GET", "/api/auth/me", "", false)
	require.Equal(t, 401, code)
}
