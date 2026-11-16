// Package auth is VulnMan's backend-for-frontend login (D2): vuln-api runs
// the OIDC Authorization Code + PKCE flow against Keycloak as a confidential
// client and gives the browser only an opaque, HttpOnly session cookie.
// Tokens never reach JavaScript.
package auth

import (
	"context"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

const (
	// SessionCookie: __Host- prefix = Secure, Path=/, no Domain - the
	// browser refuses it from subdomains or over plain HTTP.
	SessionCookie = "__Host-vm_session"
	stateCookie   = "__Host-vm_oidc"
	AbsoluteTTL   = 8 * time.Hour
	IdleTTL       = 30 * time.Minute
)

type User struct {
	Sub   string      `json:"sub"`
	Name  string      `json:"name"`
	Email string      `json:"email"`
	Role  domain.Role `json:"role"`
}

// SessionStore is implemented by store.Sessions (Postgres, SHA-256 of the
// ID at rest).
type SessionStore interface {
	Create(ctx context.Context, u User, absTTL time.Duration) (rawID string, err error)
	Touch(ctx context.Context, rawID string, idleTTL time.Duration) (User, bool, error)
	Delete(ctx context.Context, rawID string) error
	DeleteBySub(ctx context.Context, sub string) error
}

type ctxKey struct{}

func withUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFrom returns the authenticated user RequireUser put in the context.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// WithUser is for tests in other packages that need an authenticated context.
func WithUser(ctx context.Context, u User) context.Context { return withUser(ctx, u) }
