// Package server wires quote-api's HTTP routes and CORS handling.
package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/RaiseOfEASV/Cerberus/apps/quote-api/internal/quotes"
)

// New returns quote-api's handler. allowedOrigin is either "*" (any origin,
// dev/staging) or a comma-separated list of exact origins (prod serves the
// frontend on both cerberoast.site and www.cerberoast.site).
func New(allowedOrigin string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /quote", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"quote": quotes.Random(quotes.All)})
	})
	return withCORS(parseOrigins(allowedOrigin), mux)
}

func parseOrigins(s string) []string {
	var out []string
	for _, o := range strings.Split(s, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// allowOrigin returns the Access-Control-Allow-Origin value for a request's
// Origin header, or "" if that origin is not allowed.
func allowOrigin(allowed []string, origin string) string {
	for _, a := range allowed {
		if a == "*" {
			return "*"
		}
		if a == origin {
			return origin
		}
	}
	return ""
}

func withCORS(allowed []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if v := allowOrigin(allowed, r.Header.Get("Origin")); v != "" {
			w.Header().Set("Access-Control-Allow-Origin", v)
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
