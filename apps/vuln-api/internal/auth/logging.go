package auth

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/ratelimit"
)

// securityEvent writes one structured log line with a stable "event" key
// (auth.login_failed, auth.login_rate_limited, auth.login_succeeded) so the
// log pipeline can alert on it. Never pass a password or token here.
func securityEvent(level slog.Level, event string, attrs ...any) {
	slog.Log(context.Background(), level, "security event", append([]any{"event", event}, attrs...)...)
}

func (o *OIDC) clientIP(r *http.Request) string { return ratelimit.ClientIP(r, o.trusted) }
