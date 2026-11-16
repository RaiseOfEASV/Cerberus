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
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/config"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
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
		jsonError(w, http.StatusBadRequest, "login state missing or invalid - start again")
		return
	}
	if subtle.ConstantTimeCompare([]byte(st.State), []byte(r.URL.Query().Get("state"))) != 1 {
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
		slog.Warn("oidc code exchange failed", "err", err)
		jsonError(w, http.StatusBadRequest, "login failed")
		return
	}
	rawID, _ := tok.Extra("id_token").(string)
	idt, err := o.idVerifier.Verify(ctx, rawID)
	if err != nil {
		slog.Warn("id token rejected", "err", err)
		jsonError(w, http.StatusBadRequest, "login failed")
		return
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(st.Nonce)) != 1 {
		jsonError(w, http.StatusBadRequest, "login failed")
		return
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
		jsonError(w, http.StatusBadRequest, "login failed")
		return
	}
	var atc struct {
		Azp         string `json:"azp"`
		RealmAccess struct {
			Roles []string `json:"roles"`
		} `json:"realm_access"`
	}
	if err := at.Claims(&atc); err != nil || atc.Azp != o.clientID || at.Subject != idt.Subject {
		jsonError(w, http.StatusBadRequest, "login failed")
		return
	}
	role, err := domain.RoleFromClaims(atc.RealmAccess.Roles)
	if err != nil {
		jsonError(w, http.StatusForbidden, "no VulnMan role assigned")
		return
	}
	name := idClaims.Name
	if name == "" {
		name = idClaims.PreferredUsername
	}
	u := User{Sub: idt.Subject, Name: name, Email: idClaims.Email, Role: role}
	id, err := o.sessions.Create(ctx, u, AbsoluteTTL)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: id, Path: "/", MaxAge: int(AbsoluteTTL / time.Second),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
	if o.onLogin != nil {
		o.onLogin(ctx, u)
	}
	http.Redirect(w, r, st.Return, http.StatusFound)
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
