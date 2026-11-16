package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/auth"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

func report(title string) map[string]any {
	return map[string]any{"title": title, "description": "Found an IDOR on /api/orders/{id}", "asset": "shop", "contactEmail": "researcher@example.org"}
}

// public posts without any session or Origin, from a given peer address.
func (hs *harness) public(remote, xff string, body any) *httptest.ResponseRecorder {
	hs.t.Helper()
	var rd *bytes.Reader
	if s, ok := body.(string); ok {
		rd = bytes.NewReader([]byte(s))
	} else {
		b, _ := jsonMarshal(body)
		rd = bytes.NewReader(b)
	}
	r := httptest.NewRequest("POST", "/api/public/reports", rd)
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = remote
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	hs.h.ServeHTTP(w, r)
	return w
}

func TestPublicCreatesNewExternalFinding(t *testing.T) {
	hs := newHarness(t)
	w := hs.public("10.42.0.5:1000", "203.0.113.10", report("IDOR in orders"))
	if w.Code != http.StatusCreated {
		t.Fatalf("public report: %d %s", w.Code, w.Body)
	}
	ack := decode[map[string]string](t, w)
	if !strings.HasPrefix(ack["reference"], "VM-") || !strings.Contains(ack["message"], "will be triaged") {
		t.Fatalf("ack: %v", ack)
	}
	items, total, _ := hs.st.ListFindings(context.Background(), store.Filter{})
	if total != 1 || items[0].Source != "external" || items[0].Status != domain.StNew || items[0].Severity != domain.SevLow || items[0].Ref != ack["reference"] {
		t.Fatalf("stored finding: %+v", items)
	}
	f, _ := hs.st.GetFinding(context.Background(), items[0].ID)
	if f.ReporterEmail != "researcher@example.org" {
		t.Fatalf("reporter e-mail: %q", f.ReporterEmail)
	}
	var raw []byte
	_ = hs.st.Pool().QueryRow(context.Background(), `SELECT reporter_email_ct FROM findings`).Scan(&raw)
	if bytes.Contains(raw, []byte("researcher@")) {
		t.Fatal("reporter e-mail stored in plaintext")
	}
	entries, _ := hs.st.Audit(context.Background(), store.AuditFilter{})
	if len(entries) != 1 || entries[0].ActorSub != "external" || entries[0].ActorRole != "external" {
		t.Fatalf("audit: %+v", entries)
	}
}

func TestPublicValidation(t *testing.T) {
	hs := newHarness(t)
	// One peer per case: invalid attempts count toward the rate limit too.
	for i, change := range []map[string]any{
		{"contactEmail": ""}, {"contactEmail": "not-an-email"}, {"contactEmail": "Bob <bob@example.org>"},
		{"title": "x"}, {"asset": "<b>"},
	} {
		b := report("Valid title here")
		for k, v := range change {
			b[k] = v
		}
		if w := hs.public(fmt.Sprintf("192.0.2.%d:1", 10+i), "", b); w.Code != 400 {
			t.Errorf("%v: %d, want 400", change, w.Code)
		}
	}
	if w := hs.public("192.0.2.1:1", "", `{"title":"abc","asset":"a","contactEmail":"a@b.co","severity":"Critical"}`); w.Code != 400 {
		t.Errorf("extra field (severity) accepted: %d", w.Code)
	}
}

func TestPublicSixthIn429(t *testing.T) {
	hs := newHarness(t)
	for i := 0; i < 5; i++ {
		if w := hs.public("10.42.0.5:1000", "203.0.113.20", report("Report number "+string(rune('A'+i)))); w.Code != 201 {
			t.Fatalf("report %d: %d", i+1, w.Code)
		}
	}
	w := hs.public("10.42.0.5:1000", "203.0.113.20", report("Sixth report"))
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("6th report: %d %v", w.Code, w.Header())
	}
	if w := hs.public("10.42.0.5:1000", "203.0.113.21", report("Other reporter")); w.Code != 201 {
		t.Fatalf("another client limited too: %d", w.Code)
	}
}

func TestSpoofedXFFIgnoredFromUntrustedPeer(t *testing.T) {
	hs := newHarness(t)
	// A client directly reaching the pod rotates X-Forwarded-For to dodge the limit.
	for i := 0; i < 5; i++ {
		hs.public("192.0.2.66:999", "198.51.100."+string(rune('1'+i)), report("Spoof attempt "+string(rune('A'+i))))
	}
	if w := hs.public("192.0.2.66:999", "198.51.100.99", report("Spoof attempt Z")); w.Code != http.StatusTooManyRequests {
		t.Fatalf("spoofed XFF bypassed the limit: %d", w.Code)
	}
}

func TestHoneypotAcceptedButDropped(t *testing.T) {
	hs := newHarness(t)
	b := report("Bot report")
	b["website"] = "http://spam.example"
	w := hs.public("192.0.2.2:1", "", b)
	if w.Code != 201 || !strings.HasPrefix(decode[map[string]string](t, w)["reference"], "VM-") {
		t.Fatalf("honeypot response: %d %s", w.Code, w.Body)
	}
	if _, total, _ := hs.st.ListFindings(context.Background(), store.Filter{}); total != 0 {
		t.Fatalf("honeypot submission stored (%d findings)", total)
	}
}

func TestPublicBodyTooLarge413(t *testing.T) {
	hs := newHarness(t)
	b := report("Huge")
	b["description"] = strings.Repeat("A", 33<<10)
	if w := hs.public("192.0.2.3:1", "", b); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("33 KiB body: %d", w.Code)
	}
	b["description"] = strings.Repeat("A", 1<<20)
	if w := hs.public("192.0.2.3:1", "", b); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("1 MiB body: %d", w.Code)
	}
}

func TestPublicCannotRead(t *testing.T) {
	hs := newHarness(t)
	hs.public("192.0.2.4:1", "", report("Secret report"))
	for _, p := range []string{"/api/findings", "/api/public/reports", "/api/findings/1"} {
		if w := hs.do(auth.User{}, "GET", p, nil); w.Code != http.StatusUnauthorized && w.Code != http.StatusMethodNotAllowed {
			t.Errorf("anonymous GET %s: %d", p, w.Code)
		}
	}
}
