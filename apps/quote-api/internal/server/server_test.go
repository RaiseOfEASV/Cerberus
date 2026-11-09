package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz_ReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	New("*").ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestQuote_ReturnsJSONWithNonEmptyQuote(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/quote", nil)
	rec := httptest.NewRecorder()
	New("*").ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Quote string `json:"quote"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body.Quote == "" {
		t.Fatal("quote field is empty")
	}
}

func TestQuote_PreflightOptionsReturnsAllowHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/quote", nil)
	req.Header.Set("Origin", "https://cerberoast.site")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	New("https://cerberoast.site").ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://cerberoast.site" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want https://cerberoast.site", got)
	}
}

func TestQuote_ActualRequestCarriesAllowOriginHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/quote", nil)
	req.Header.Set("Origin", "https://cerberoast.site")
	rec := httptest.NewRecorder()
	New("https://cerberoast.site").ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://cerberoast.site" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want https://cerberoast.site", got)
	}
}

// Prod serves quote-web on both cerberoast.site and www.cerberoast.site, so
// ALLOWED_ORIGIN is a comma-separated list and the matching origin is echoed.
func TestQuote_OriginListEchoesTheMatchingOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/quote", nil)
	req.Header.Set("Origin", "https://www.cerberoast.site")
	rec := httptest.NewRecorder()
	New("https://cerberoast.site, https://www.cerberoast.site").ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://www.cerberoast.site" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want https://www.cerberoast.site", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}
}

func TestQuote_DisallowedOriginGetsNoAllowOriginHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/quote", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	New("https://cerberoast.site,https://www.cerberoast.site").ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty for a disallowed origin", got)
	}
}

func TestQuote_PostIsRejected(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/quote", nil)
	rec := httptest.NewRecorder()
	New("*").ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
