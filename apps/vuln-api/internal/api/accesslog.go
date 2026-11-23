package api

import (
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/ratelimit"
)

// accessLog writes one JSON line per request (event "http.request") with the
// real client address behind the trusted proxies. The query string is left
// out on purpose: filters and search terms can carry sensitive data.
// Kubernetes probes on /healthz are skipped.
func accessLog(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			slog.Info("http request",
				"event", "http.request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"ip", ratelimit.ClientIP(r, trusted),
				"user_agent", r.UserAgent(),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wrote = true
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
