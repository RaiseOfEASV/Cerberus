package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/config"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/ratelimit"
)

type OIDC struct {
	oauth      oauth2.Config
	idVerifier *oidc.IDTokenVerifier
	atVerifier *oidc.IDTokenVerifier
	clientID   string
	publicBase string
	endSession string
	sessions   SessionStore
	onLogin    func(context.Context, User)
	userLimit  *ratelimit.Limiter
	ipLimit    *ratelimit.Limiter
	trusted    []netip.Prefix
}

// NewOIDC discovers the provider. Discovery uses cfg.OIDCDiscoveryURL (the
// in-cluster Keycloak URL) while tokens must still carry the public
// cfg.OIDCIssuer - go-oidc's InsecureIssuerURLContext keeps that check.
func NewOIDC(ctx context.Context, cfg config.Config, sessions SessionStore, onLogin func(context.Context, User)) (*OIDC, error) {
	dctx := ctx
	if cfg.OIDCDiscoveryURL != cfg.OIDCIssuer {
		dctx = oidc.InsecureIssuerURLContext(ctx, cfg.OIDCIssuer)
	}
	p, err := oidc.NewProvider(dctx, cfg.OIDCDiscoveryURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	var extra struct {
		EndSession string `json:"end_session_endpoint"`
	}
	_ = p.Claims(&extra)
	return &OIDC{
		oauth: oauth2.Config{
			ClientID: cfg.OIDCClientID, ClientSecret: cfg.OIDCClientSecret,
			Endpoint: p.Endpoint(), RedirectURL: cfg.PublicBaseURL + "/auth/callback",
			Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
		},
		idVerifier: p.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID}),
		// Keycloak access tokens have aud "account"; azp is checked instead.
		atVerifier: p.Verifier(&oidc.Config{SkipClientIDCheck: true}),
		clientID:   cfg.OIDCClientID,
		publicBase: cfg.PublicBaseURL,
		endSession: extra.EndSession,
		sessions:   sessions,
		onLogin:    onLogin,
		// Per hour: 10 attempts per username, 30 per client address.
		userLimit: ratelimit.New(10, time.Now),
		ipLimit:   ratelimit.New(30, time.Now),
		trusted:   cfg.TrustedProxyCIDRs,
	}, nil
}

type loginState struct {
	State, Nonce, Verifier, Return string
}

func random() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// safeReturn only allows same-site absolute paths ("/x"), never "//host",
// "/\host" or a scheme.
func safeReturn(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, `/\`) {
		return "/"
	}
	if u, err := url.Parse(p); err != nil || u.Host != "" || u.Scheme != "" {
		return "/"
	}
	return p
}

// Login: GET /auth/login?return=/path - redirects to Keycloak.
func (o *OIDC) Login(w http.ResponseWriter, r *http.Request) {
	st := loginState{State: random(), Nonce: random(), Verifier: oauth2.GenerateVerifier(), Return: safeReturn(r.URL.Query().Get("return"))}
	b, _ := json.Marshal(st)
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: base64.RawURLEncoding.EncodeToString(b), Path: "/",
		MaxAge: 600, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, o.oauth.AuthCodeURL(st.State, oidc.Nonce(st.Nonce), oauth2.S256ChallengeOption(st.Verifier)), http.StatusFound)
}

func (o *OIDC) readState(r *http.Request) (loginState, error) {
	c, err := r.Cookie(stateCookie)
	if err != nil {
		return loginState{}, errors.New("missing login state")
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return loginState{}, errors.New("bad login state")
	}
	var st loginState
	if err := json.Unmarshal(raw, &st); err != nil || st.State == "" {
		return loginState{}, errors.New("bad login state")
	}
	return st, nil
}

// Callback: GET /auth/callback - checks state, exchanges the code with the
// PKCE verifier, verifies the ID token (issuer, audience, nonce) and the
// access token, maps realm roles to one app role, and starts a session.
func (o *OIDC) Callback(w http.ResponseWriter, r *http.Request) {
	clear := &http.Cookie{Name: stateCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode}
	http.SetCookie(w, clear)
	st, err := o.readState(r)
	if err != nil {
		securityEvent(slog.LevelWarn, "auth.login_failed", "method", "oidc", "ip", o.clientIP(r), "reason", "missing_state")
		jsonError(w, http.StatusBadRequest, "login state missing or invalid - start again")
		return
	}
	if subtle.ConstantTimeCompare([]byte(st.State), []byte(r.URL.Query().Get("state"))) != 1 {
		securityEvent(slog.LevelWarn, "auth.login_failed", "method", "oidc", "ip", o.clientIP(r), "reason", "state_mismatch")
		jsonError(w, http.StatusBadRequest, "login state mismatch - start again")
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		jsonError(w, http.StatusBadRequest, "login was not completed")
		return
	}
	ctx := r.Context()
	tok, err := o.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		securityEvent(slog.LevelWarn, "auth.login_failed", "method", "oidc", "ip", o.clientIP(r), "reason", "code_exchange", "err", err)
		jsonError(w, http.StatusBadRequest, "login failed")
		return
	}
	u, code, msg := o.verifyTokens(ctx, tok, st.Nonce)
	if code != 0 {
		securityEvent(slog.LevelWarn, "auth.login_failed", "method", "oidc", "ip", o.clientIP(r), "reason", msg)
		jsonError(w, code, msg)
		return
	}
	id, err := o.sessions.Create(ctx, u, AbsoluteTTL)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: id, Path: "/", MaxAge: int(AbsoluteTTL / time.Second),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
	securityEvent(slog.LevelInfo, "auth.login_succeeded", "method", "oidc", "sub", u.Sub, "ip", o.clientIP(r))
	if o.onLogin != nil {
		o.onLogin(ctx, u)
	}
	http.Redirect(w, r, st.Return, http.StatusFound)
}

// verifyTokens checks a token response from Keycloak - ID token (issuer,
// audience and, for the redirect flow, nonce), access token (azp, same
// subject) - and maps realm roles to one app role. Shared by the redirect
// callback and the password login, so both trust exactly the same things.
// code is 0 on success, otherwise the HTTP status to answer with.
func (o *OIDC) verifyTokens(ctx context.Context, tok *oauth2.Token, wantNonce string) (User, int, string) {
	rawID, _ := tok.Extra("id_token").(string)
	idt, err := o.idVerifier.Verify(ctx, rawID)
	if err != nil {
		slog.Warn("id token rejected", "err", err)
		return User{}, http.StatusBadRequest, "login failed"
	}
	if wantNonce != "" && subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(wantNonce)) != 1 {
		return User{}, http.StatusBadRequest, "login failed"
	}
	var idClaims struct {
		Name              string `json:"name"`
		Email             string `json:"email"`
		PreferredUsername string `json:"preferred_username"`
	}
	_ = idt.Claims(&idClaims)
	at, err := o.atVerifier.Verify(ctx, tok.AccessToken)
	if err != nil {
		slog.Warn("access token rejected", "err", err)
		return User{}, http.StatusBadRequest, "login failed"
	}
	var atc struct {
		Azp         string `json:"azp"`
		RealmAccess struct {
			Roles []string `json:"roles"`
		} `json:"realm_access"`
	}
	if err := at.Claims(&atc); err != nil || atc.Azp != o.clientID || at.Subject != idt.Subject {
		return User{}, http.StatusBadRequest, "login failed"
	}
	role, err := domain.RoleFromClaims(atc.RealmAccess.Roles)
	if err != nil {
		return User{}, http.StatusForbidden, "no VulnMan role assigned"
	}
	name := idClaims.Name
	if name == "" {
		name = idClaims.PreferredUsername
	}
	return User{Sub: idt.Subject, Name: name, Email: idClaims.Email, Role: role}, 0, ""
}

// PasswordLogin: POST /auth/password {"username","password"} - VulnMan's own
// login form. vuln-api checks the credentials with Keycloak's password grant
// as a confidential client (the browser never talks to Keycloak and never
// sees a token), then starts the same server-side session as the redirect
// login. Attempts are limited per username and per client address, on top
// of Keycloak's brute-force lockout.
func (o *OIDC) PasswordLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		jsonError(w, http.StatusBadRequest, "expected a JSON body with username and password")
		return
	}
	if in.Username == "" || len(in.Username) > 256 || in.Password == "" || len(in.Password) > 1024 {
		jsonError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	username, ip := strings.ToLower(in.Username), o.clientIP(r)
	if !o.userLimit.Allow(username) || !o.ipLimit.Allow(ip) {
		securityEvent(slog.LevelWarn, "auth.login_rate_limited", "username", username, "ip", ip)
		w.Header().Set("Retry-After", "3600")
		jsonError(w, http.StatusTooManyRequests, "too many login attempts - try again later")
		return
	}
	ctx := r.Context()
	tok, err := o.oauth.PasswordCredentialsToken(ctx, in.Username, in.Password)
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && (re.Response.StatusCode == http.StatusUnauthorized || re.Response.StatusCode == http.StatusBadRequest) {
			securityEvent(slog.LevelWarn, "auth.login_failed", "method", "password", "username", username, "ip", ip, "reason", "bad_credentials")
			jsonError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		slog.Error("password grant failed", "err", err)
		jsonError(w, http.StatusBadGateway, "sign-in is unavailable right now")
		return
	}
	u, code, msg := o.verifyTokens(ctx, tok, "")
	if code != 0 {
		securityEvent(slog.LevelWarn, "auth.login_failed", "method", "password", "username", username, "ip", ip, "reason", msg)
		jsonError(w, code, msg)
		return
	}
	id, err := o.sessions.Create(ctx, u, AbsoluteTTL)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: id, Path: "/", MaxAge: int(AbsoluteTTL / time.Second),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
	securityEvent(slog.LevelInfo, "auth.login_succeeded", "method", "password", "sub", u.Sub, "ip", ip)
	if o.onLogin != nil {
		o.onLogin(ctx, u)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(u)
}

// Logout: POST /auth/logout - ends the local session and redirects to
// Keycloak's end-session endpoint so the IdP session ends too.
func (o *OIDC) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
		_ = o.sessions.Delete(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	dest := "/"
	if o.endSession != "" {
		q := url.Values{"client_id": {o.clientID}, "post_logout_redirect_uri": {o.publicBase + "/"}}
		dest = o.endSession + "?" + q.Encode()
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// hashID is how session IDs are stored (store package uses it too).
func HashID(raw string) []byte {
	s := sha256.Sum256([]byte(raw))
	return s[:]
}
