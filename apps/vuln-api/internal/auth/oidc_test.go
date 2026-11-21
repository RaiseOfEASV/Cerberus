package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/config"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

// fakeIdP is a minimal OpenID provider: discovery, JWKS, and a token
// endpoint that enforces PKCE S256 like Keycloak does.
type fakeIdP struct {
	srv      *httptest.Server
	key      *rsa.PrivateKey
	mu       sync.Mutex
	codes    map[string]authReq
	roles    []string
	badNonce bool
}

type authReq struct{ nonce, challenge, redirect string }

func newIdP(t *testing.T) *fakeIdP {
	t.Helper()
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	f := &fakeIdP{key: k, codes: map[string]authReq{}, roles: []string{"analyst", "offline_access"}}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/auth", "token_endpoint": f.srv.URL + "/token",
			"jwks_uri": f.srv.URL + "/jwks", "end_session_endpoint": f.srv.URL + "/logout",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &k.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") == "password" {
			if r.Form.Get("username") != "ana" || r.Form.Get("password") != "correct horse" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Invalid user credentials"}`))
				return
			}
			now := time.Now()
			std := jwt.Claims{Issuer: f.srv.URL, Subject: "user-123", Expiry: jwt.NewNumericDate(now.Add(5 * time.Minute)), IssuedAt: jwt.NewNumericDate(now)}
			id := f.sign(t, std, jwt.Audience{"vuln-api"}, map[string]any{"name": "Ana Analyst", "email": "ana@example.com"})
			access := f.sign(t, std, jwt.Audience{"account"}, map[string]any{"azp": "vuln-api", "realm_access": map[string]any{"roles": f.roles}})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": access, "id_token": id, "token_type": "Bearer", "expires_in": 300})
			return
		}
		f.mu.Lock()
		req, ok := f.codes[r.Form.Get("code")]
		delete(f.codes, r.Form.Get("code"))
		f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != req.challenge || r.Form.Get("redirect_uri") != req.redirect {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		nonce := req.nonce
		if f.badNonce {
			nonce = "something-else"
		}
		now := time.Now()
		std := jwt.Claims{Issuer: f.srv.URL, Subject: "user-123", Expiry: jwt.NewNumericDate(now.Add(5 * time.Minute)), IssuedAt: jwt.NewNumericDate(now)}
		id := f.sign(t, std, jwt.Audience{"vuln-api"}, map[string]any{"nonce": nonce, "name": "Ana Analyst", "email": "ana@example.com"})
		access := f.sign(t, std, jwt.Audience{"account"}, map[string]any{"azp": "vuln-api", "realm_access": map[string]any{"roles": f.roles}})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": access, "id_token": id, "token_type": "Bearer", "expires_in": 300})
	})
	return f
}

func (f *fakeIdP) sign(t *testing.T, std jwt.Claims, aud jwt.Audience, extra map[string]any) string {
	t.Helper()
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
	if err != nil {
		t.Fatal(err)
	}
	std.Audience = aud
	s, err := jwt.Signed(sig).Claims(std).Claims(extra).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// authorize plays the browser + IdP login: takes the redirect from /auth/login
// and returns the callback URL Keycloak would send the browser to.
func (f *fakeIdP) authorize(t *testing.T, loginRedirect string) string {
	t.Helper()
	u, err := url.Parse(loginRedirect)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	code := "code-" + q.Get("state")
	f.mu.Lock()
	f.codes[code] = authReq{nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri")}
	f.mu.Unlock()
	return q.Get("redirect_uri") + "?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(q.Get("state"))
}

type memSessions struct {
	mu   sync.Mutex
	byID map[string]User
}

func (m *memSessions) Create(_ context.Context, u User, _ time.Duration) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := "sess-" + u.Sub + "-" + time.Now().Format(time.RFC3339Nano)
	m.byID[id] = u
	return id, nil
}
func (m *memSessions) Touch(_ context.Context, id string, _ time.Duration) (User, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byID[id]
	return u, ok, nil
}
func (m *memSessions) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byID, id)
	return nil
}
func (m *memSessions) DeleteBySub(_ context.Context, sub string) error { return nil }

func setup(t *testing.T) (*OIDC, *fakeIdP, *memSessions, *[]User) {
	t.Helper()
	idp := newIdP(t)
	ss := &memSessions{byID: map[string]User{}}
	var logins []User
	cfg := config.Config{PublicBaseURL: "https://vulns.test", OIDCIssuer: idp.srv.URL, OIDCDiscoveryURL: idp.srv.URL,
		OIDCClientID: "vuln-api", OIDCClientSecret: "secret"}
	o, err := NewOIDC(context.Background(), cfg, ss, func(_ context.Context, u User) { logins = append(logins, u) })
	if err != nil {
		t.Fatal(err)
	}
	return o, idp, ss, &logins
}

// login runs /auth/login and returns the redirect and the oidc state cookie.
func login(t *testing.T, o *OIDC, ret string) (string, *http.Cookie) {
	t.Helper()
	w := httptest.NewRecorder()
	o.Login(w, httptest.NewRequest("GET", "/auth/login?return="+url.QueryEscape(ret), nil))
	if w.Code != http.StatusFound {
		t.Fatalf("login status %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == stateCookie {
			return w.Header().Get("Location"), c
		}
	}
	t.Fatal("no state cookie")
	return "", nil
}

func callback(o *OIDC, callbackURL string, c *http.Cookie) *httptest.ResponseRecorder {
	u, _ := url.Parse(callbackURL)
	r := httptest.NewRequest("GET", "/auth/callback?"+u.RawQuery, nil)
	if c != nil {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	o.Callback(w, r)
	return w
}

func TestLoginSetsStateAndPKCE(t *testing.T) {
	o, _, _, _ := setup(t)
	loc, c := login(t, o, "/findings")
	u, _ := url.Parse(loc)
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("state") == "" || q.Get("nonce") == "" {
		t.Fatalf("authorization request missing PKCE/state/nonce: %s", loc)
	}
	if q.Get("redirect_uri") != "https://vulns.test/auth/callback" || q.Get("client_id") != "vuln-api" {
		t.Fatalf("redirect_uri/client_id wrong: %s", loc)
	}
	if !strings.Contains(q.Get("scope"), "openid") {
		t.Fatalf("scope %q", q.Get("scope"))
	}
	if !c.HttpOnly || !c.Secure || c.Path != "/" || c.SameSite != http.SameSiteLaxMode || c.MaxAge <= 0 || c.MaxAge > 600 {
		t.Fatalf("state cookie attributes: %+v", c)
	}
	if strings.Contains(c.Value, q.Get("code_challenge")) {
		t.Fatal("cookie should hold the verifier, not just echo the challenge")
	}
}

func TestCallbackCreatesSession(t *testing.T) {
	o, idp, ss, logins := setup(t)
	loc, c := login(t, o, "/findings/7")
	w := callback(o, idp.authorize(t, loc), c)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/findings/7" {
		t.Fatalf("callback = %d %s %s", w.Code, w.Header().Get("Location"), w.Body)
	}
	var sess *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == SessionCookie {
			sess = ck
		}
	}
	if sess == nil || !sess.HttpOnly || !sess.Secure || sess.Path != "/" || sess.SameSite != http.SameSiteLaxMode || sess.Domain != "" {
		t.Fatalf("session cookie: %+v", sess)
	}
	u, ok, _ := ss.Touch(context.Background(), sess.Value, time.Minute)
	if !ok || u.Sub != "user-123" || u.Role != domain.RoleAnalyst || u.Name != "Ana Analyst" || u.Email != "ana@example.com" {
		t.Fatalf("session user = %+v %v", u, ok)
	}
	if len(*logins) != 1 {
		t.Fatalf("onLogin called %d times", len(*logins))
	}
}

func TestCallbackRejectsBadState(t *testing.T) {
	o, idp, ss, _ := setup(t)
	loc, c := login(t, o, "/")
	cb := idp.authorize(t, loc)
	cb = strings.Replace(cb, "state=", "state=tampered", 1)
	if w := callback(o, cb, c); w.Code != http.StatusBadRequest {
		t.Fatalf("bad state: %d", w.Code)
	}
	if w := callback(o, idp.authorize(t, loc), nil); w.Code != http.StatusBadRequest {
		t.Fatalf("missing state cookie: %d", w.Code)
	}
	if len(ss.byID) != 0 {
		t.Fatal("session created despite bad state")
	}
}

func TestCallbackRejectsWrongNonce(t *testing.T) {
	o, idp, ss, _ := setup(t)
	idp.badNonce = true
	loc, c := login(t, o, "/")
	if w := callback(o, idp.authorize(t, loc), c); w.Code != http.StatusBadRequest {
		t.Fatalf("wrong nonce: %d %s", w.Code, w.Body)
	}
	if len(ss.byID) != 0 {
		t.Fatal("session created despite wrong nonce")
	}
}

func TestCallbackNoRoleForbidden(t *testing.T) {
	o, idp, ss, _ := setup(t)
	idp.roles = []string{"offline_access"}
	loc, c := login(t, o, "/")
	w := callback(o, idp.authorize(t, loc), c)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "no VulnMan role assigned") {
		t.Fatalf("no role: %d %s", w.Code, w.Body)
	}
	if len(ss.byID) != 0 {
		t.Fatal("session created for a user without a role")
	}
}

func TestOpenRedirectBlocked(t *testing.T) {
	o, idp, _, _ := setup(t)
	for _, evil := range []string{"//evil.com", "https://evil.com", "/\\evil.com", "javascript:alert(1)", ""} {
		loc, c := login(t, o, evil)
		w := callback(o, idp.authorize(t, loc), c)
		if got := w.Header().Get("Location"); got != "/" {
			t.Errorf("return=%q redirected to %q", evil, got)
		}
	}
}

func TestLogout(t *testing.T) {
	o, idp, ss, _ := setup(t)
	loc, c := login(t, o, "/")
	w := callback(o, idp.authorize(t, loc), c)
	var sess *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == SessionCookie {
			sess = ck
		}
	}
	r := httptest.NewRequest("POST", "/auth/logout", nil)
	r.AddCookie(sess)
	w = httptest.NewRecorder()
	o.Logout(w, r)
	if w.Code != http.StatusSeeOther || !strings.HasPrefix(w.Header().Get("Location"), idp.srv.URL+"/logout?") {
		t.Fatalf("logout = %d %s", w.Code, w.Header().Get("Location"))
	}
	if _, ok, _ := ss.Touch(context.Background(), sess.Value, time.Minute); ok {
		t.Fatal("session still valid after logout")
	}
}

func passwordLogin(o *OIDC, remote, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/auth/password", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = remote
	w := httptest.NewRecorder()
	o.PasswordLogin(w, r)
	return w
}

func TestPasswordLoginCreatesSession(t *testing.T) {
	o, _, ss, logins := setup(t)
	w := passwordLogin(o, "192.0.2.1:1", `{"username":"ana","password":"correct horse"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"role":"analyst"`) {
		t.Fatalf("password login: %d %s", w.Code, w.Body)
	}
	var sess *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == SessionCookie {
			sess = c
		}
	}
	if sess == nil || !sess.HttpOnly || !sess.Secure || sess.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie: %+v", sess)
	}
	if u, ok, _ := ss.Touch(context.Background(), sess.Value, time.Minute); !ok || u.Sub != "user-123" || u.Role != domain.RoleAnalyst {
		t.Fatalf("session user: %+v %v", u, ok)
	}
	if len(*logins) != 1 {
		t.Fatal("login not recorded")
	}
	if strings.Contains(w.Body.String(), "token") {
		t.Fatal("tokens must never reach the browser")
	}
}

func TestPasswordLoginWrongPassword(t *testing.T) {
	o, _, ss, _ := setup(t)
	for _, body := range []string{`{"username":"ana","password":"wrong"}`, `{"username":"nobody","password":"correct horse"}`} {
		w := passwordLogin(o, "192.0.2.2:1", body)
		if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "invalid username or password") {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if len(ss.byID) != 0 {
		t.Fatal("session created for bad credentials")
	}
}

func TestPasswordLoginNoRole(t *testing.T) {
	o, idp, ss, _ := setup(t)
	idp.roles = []string{"offline_access"}
	if w := passwordLogin(o, "192.0.2.3:1", `{"username":"ana","password":"correct horse"}`); w.Code != http.StatusForbidden {
		t.Fatalf("no role: %d %s", w.Code, w.Body)
	}
	if len(ss.byID) != 0 {
		t.Fatal("session created for a user without a role")
	}
}

func TestPasswordLoginRateLimited(t *testing.T) {
	o, _, _, _ := setup(t)
	for i := 0; i < 10; i++ {
		passwordLogin(o, "192.0.2.4:1", `{"username":"ana","password":"guess"}`)
	}
	w := passwordLogin(o, "192.0.2.5:1", `{"username":"ana","password":"correct horse"}`)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("11th attempt for one username (from another address): %d", w.Code)
	}
	if w := passwordLogin(o, "192.0.2.6:1", `{"username":"other","password":"x"}`); w.Code == http.StatusTooManyRequests {
		t.Fatal("limit leaked to another username")
	}
}

func TestPasswordLoginRejectsBadBodies(t *testing.T) {
	o, _, _, _ := setup(t)
	for _, body := range []string{`not json`, `{"username":"ana"}`, `{"username":"","password":"x"}`, `{"username":"ana","password":"x","extra":1}`, `{"username":"` + strings.Repeat("a", 5000) + `","password":"x"}`} {
		if w := passwordLogin(o, "192.0.2.7:1", body); w.Code != http.StatusBadRequest && w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("%.40s: %d", body, w.Code)
		}
	}
}
