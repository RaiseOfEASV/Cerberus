package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func logInto(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestAccessLogRecordsRequest(t *testing.T) {
	buf := logInto(t)
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	h := accessLog(trusted)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hello"))
	}))
	r := httptest.NewRequest("POST", "/api/findings?q=secret", nil)
	r.RemoteAddr = "10.1.2.3:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	r.Header.Set("User-Agent", "sqlmap/1.8")
	h.ServeHTTP(httptest.NewRecorder(), r)

	var e map[string]any
	if err := json.Unmarshal(buf.Bytes(), &e); err != nil {
		t.Fatalf("access log not one JSON line: %q", buf)
	}
	want := map[string]any{"event": "http.request", "method": "POST", "path": "/api/findings",
		"status": float64(418), "bytes": float64(5), "ip": "203.0.113.7", "user_agent": "sqlmap/1.8"}
	for k, v := range want {
		if e[k] != v {
			t.Errorf("%s = %v, want %v", k, e[k], v)
		}
	}
	if _, ok := e["duration_ms"]; !ok {
		t.Error("no duration_ms")
	}
	if strings.Contains(buf.String(), "secret") {
		t.Error("query string must not be logged")
	}
}

func TestAccessLogDefaultsStatusTo200(t *testing.T) {
	buf := logInto(t)
	h := accessLog(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("x")) }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/me", nil))
	if !strings.Contains(buf.String(), `"status":200`) {
		t.Fatalf("got %s", buf)
	}
}

func TestAccessLogSkipsHealthz(t *testing.T) {
	buf := logInto(t)
	h := accessLog(nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/healthz", nil))
	if buf.Len() != 0 {
		t.Fatalf("probe was logged: %s", buf)
	}
}
