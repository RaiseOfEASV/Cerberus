package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

const scanToken = "0123456789abcdef0123456789abcdef-scanner"

func trivyReport(ids ...string) string {
	var v []string
	for _, id := range ids {
		v = append(v, `{"VulnerabilityID":"`+id+`","PkgName":"libfoo","InstalledVersion":"1.0","FixedVersion":"1.1","Severity":"HIGH"}`)
	}
	return `{"SchemaVersion":2,"ArtifactName":"ghcr.io/raiseofeasv/vuln-web@sha256:abc","Results":[{"Target":"img (alpine 3.22)","Vulnerabilities":[` + strings.Join(v, ",") + `]}]}`
}

func (hs *harness) scan(token, asset, body string, hdr ...string) *httptest.ResponseRecorder {
	hs.t.Helper()
	r := httptest.NewRequest("POST", "/internal/scans/trivy?asset="+asset, bytes.NewBufferString(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	hs.h.ServeHTTP(w, r)
	return w
}

func withScanner(t *testing.T) *harness {
	hs := newHarness(t)
	hs.deps.Cfg.ScannerToken = scanToken
	hs.rebuild()
	return hs
}

func TestScanEndpointSyncsFindings(t *testing.T) {
	hs := withScanner(t)
	const asset = "ghcr.io/raiseofeasv/vuln-web"
	w := hs.scan(scanToken, asset, trivyReport("CVE-2026-1001", "CVE-2026-1002"))
	if w.Code != http.StatusOK {
		t.Fatalf("first scan: %d %s", w.Code, w.Body)
	}
	if got := decode[store.ScanResult](t, w); got != (store.ScanResult{Created: 2}) {
		t.Fatalf("first scan: %+v", got)
	}
	w = hs.scan(scanToken, asset, trivyReport("CVE-2026-1002"))
	if got := decode[store.ScanResult](t, w); got != (store.ScanResult{Skipped: 1, Closed: 1}) {
		t.Fatalf("second scan: %+v", got)
	}

	items, _, err := hs.st.ListFindings(t.Context(), store.Filter{Asset: asset, Limit: 10})
	if err != nil || len(items) != 2 {
		t.Fatalf("findings: %v %v", items, err)
	}
	for _, f := range items {
		if f.Source != "trivy" || f.Asset != asset {
			t.Errorf("finding %s: source %q asset %q", f.Ref, f.Source, f.Asset)
		}
		want := domain.StNew
		if f.CVE == "CVE-2026-1001" {
			want = domain.StRemediated
		}
		if f.Status != want {
			t.Errorf("%s: %q, want %q", f.CVE, f.Status, want)
		}
	}
	entries, err := hs.st.Audit(t.Context(), store.AuditFilter{ActorSub: "system:trivy-scanner"})
	if err != nil || len(entries) == 0 {
		t.Fatalf("scanner audit rows: %d %v", len(entries), err)
	}
}

func TestScanEndpointRefusals(t *testing.T) {
	hs := withScanner(t)
	const asset = "ghcr.io/raiseofeasv/vuln-web"
	cases := []struct {
		name string
		w    *httptest.ResponseRecorder
		want int
	}{
		{"no token", hs.scan("", asset, trivyReport("CVE-2026-1001")), http.StatusUnauthorized},
		{"wrong token", hs.scan(scanToken+"x", asset, trivyReport("CVE-2026-1001")), http.StatusUnauthorized},
		{"through the proxy", hs.scan(scanToken, asset, trivyReport("CVE-2026-1001"), "X-Forwarded-For", "203.0.113.9"), http.StatusNotFound},
		{"no asset", hs.scan(scanToken, "", trivyReport("CVE-2026-1001")), http.StatusBadRequest},
		{"not a report", hs.scan(scanToken, asset, `{"hello":1}`), http.StatusBadRequest},
		{"a user session is not enough", hs.do(uPM, "POST", "/internal/scans/trivy?asset="+asset, trivyReport("CVE-2026-1001")), http.StatusUnauthorized},
	}
	for _, c := range cases {
		if c.w.Code != c.want {
			t.Errorf("%s: %d, want %d (%s)", c.name, c.w.Code, c.want, c.w.Body)
		}
	}
	if _, total, _ := hs.st.ListFindings(t.Context(), store.Filter{Limit: 10}); total != 0 {
		t.Fatalf("refused requests created %d findings", total)
	}
}

func TestScanEndpointOffWithoutToken(t *testing.T) {
	hs := newHarness(t) // no SCANNER_TOKEN configured
	if w := hs.scan(scanToken, "x", trivyReport("CVE-2026-1001")); w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("endpoint without SCANNER_TOKEN: %d", w.Code)
	}
}
