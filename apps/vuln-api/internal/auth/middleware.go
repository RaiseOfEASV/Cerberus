package auth

import (
	"net/http"
)

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}

// RequireUser rejects requests without a live session (401) and puts the
// session's user in the request context.
func RequireUser(s SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(SessionCookie)
			if err != nil || c.Value == "" {
				jsonError(w, http.StatusUnauthorized, "not logged in")
				return
			}
			u, ok, err := s.Touch(r.Context(), c.Value, IdleTTL)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "session lookup failed")
				return
			}
			if !ok {
				jsonError(w, http.StatusUnauthorized, "session expired")
				return
			}
			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), u)))
		})
	}
}

// SameOrigin blocks cross-site state-changing requests (CSRF), on top of
// SameSite=Lax: the Origin header must be exactly publicBaseURL, or, when a
// browser omits Origin, Sec-Fetch-Site must be same-origin.
func SameOrigin(publicBaseURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			if o := r.Header.Get("Origin"); o != "" {
				if o != publicBaseURL {
					jsonError(w, http.StatusForbidden, "cross-origin request refused")
					return
				}
			} else if r.Header.Get("Sec-Fetch-Site") != "same-origin" {
				jsonError(w, http.StatusForbidden, "cross-origin request refused")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
