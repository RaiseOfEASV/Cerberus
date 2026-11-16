package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

func TestRequireUser(t *testing.T) {
	ss := &memSessions{byID: map[string]User{"good": {Sub: "u1", Role: domain.RolePM}}}
	h := RequireUser(ss)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFrom(r.Context())
		if !ok || u.Sub != "u1" {
			t.Errorf("user not in context: %+v", u)
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	for name, c := range map[string]*http.Cookie{"none": nil, "unknown": {Name: SessionCookie, Value: "bad"}} {
		r := httptest.NewRequest("GET", "/api/me", nil)
		if c != nil {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized || w.Header().Get("Content-Type") != "application/json" {
			t.Errorf("%s: %d %s", name, w.Code, w.Header().Get("Content-Type"))
		}
	}
	r := httptest.NewRequest("GET", "/api/me", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookie, Value: "good"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusTeapot {
		t.Fatalf("valid session: %d", w.Code)
	}
}

func TestSameOriginBlocksCrossSitePost(t *testing.T) {
	h := SameOrigin("https://vulns.test")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	cases := []struct {
		method, origin, fetchSite string
		want                      int
	}{
		{"POST", "https://vulns.test", "", http.StatusNoContent},
		{"POST", "https://evil.test", "", http.StatusForbidden},
		{"POST", "https://vulns.test.evil.test", "", http.StatusForbidden},
		{"POST", "null", "", http.StatusForbidden},
		{"POST", "", "same-origin", http.StatusNoContent},
		{"POST", "", "cross-site", http.StatusForbidden},
		{"POST", "", "", http.StatusForbidden},
		{"PATCH", "https://evil.test", "", http.StatusForbidden},
		{"DELETE", "", "same-site", http.StatusForbidden},
		{"GET", "https://evil.test", "cross-site", http.StatusNoContent}, // reads are not state changing
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "/api/findings", nil)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.fetchSite != "" {
			r.Header.Set("Sec-Fetch-Site", c.fetchSite)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s origin=%q site=%q: %d, want %d", c.method, c.origin, c.fetchSite, w.Code, c.want)
		}
	}
}
