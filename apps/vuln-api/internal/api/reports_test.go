package api

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/auth"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/crypt"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

type signedReport struct {
	ReportJSON string `json:"reportJson"`
	Signature  string `json:"signature"`
	KeyVersion int    `json:"keyVersion"`
}

func TestDashboardAPI(t *testing.T) {
	hs := newHarness(t)
	hs.createFinding("Dash one")
	w := hs.do(uAuditor, "GET", "/api/dashboard", nil)
	d := decode[store.Dashboard](t, w)
	if w.Code != 200 || d.ByStatus["New"] != 1 || d.OpenBySeverity["High"] != 1 {
		t.Fatalf("dashboard: %d %+v", w.Code, d)
	}
	w = hs.do(uDev, "GET", "/api/dashboard/trend?weeks=4", nil)
	if weeks := decode[[]store.WeekCount](t, w); w.Code != 200 || len(weeks) != 4 {
		t.Fatalf("trend: %d %s", w.Code, w.Body)
	}
	for _, q := range []string{"weeks=0", "weeks=53", "weeks=x"} {
		if w := hs.do(uDev, "GET", "/api/dashboard/trend?"+q, nil); w.Code != 400 {
			t.Errorf("trend %s: %d", q, w.Code)
		}
	}
}

func TestSetSLAPMOnly(t *testing.T) {
	hs := newHarness(t)
	body := map[string]int{"Critical": 3, "High": 14, "Medium": 60, "Low": 120}
	for _, u := range []auth.User{uAnalyst, uDev, uAuditor} {
		if w := hs.do(u, "PUT", "/api/sla", body); w.Code != 403 {
			t.Errorf("%s PUT sla: %d", u.Role, w.Code)
		}
	}
	if w := hs.do(uPM, "PUT", "/api/sla", body); w.Code != 200 {
		t.Fatalf("pm PUT sla: %d %s", w.Code, w.Body)
	}
	got := decode[map[string]int](t, hs.do(uDev, "GET", "/api/sla", nil))
	if got["Critical"] != 3 || got["Low"] != 120 {
		t.Fatalf("sla after PUT: %v", got)
	}
	for _, bad := range []any{map[string]int{"Critical": 0, "High": 1, "Medium": 1, "Low": 1}, map[string]int{"Critical": 1}, map[string]int{"Critical": 1, "High": 1, "Medium": 1, "Low": 1, "Info": 1}} {
		if w := hs.do(uPM, "PUT", "/api/sla", bad); w.Code != 400 {
			t.Errorf("bad sla %v: %d", bad, w.Code)
		}
	}
}

func TestReportSignedAndVerifies(t *testing.T) {
	hs := newHarness(t)
	hs.createFinding("Reported one")
	w := hs.do(uAuditor, "GET", "/api/reports?from=2026-11-01&to=2026-12-01", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), "report-2026-11-01-2026-12-01.json") {
		t.Fatalf("report: %d %v %s", w.Code, w.Header(), w.Body)
	}
	sr := decode[signedReport](t, w)
	if !strings.Contains(sr.ReportJSON, "Reported one") || strings.Contains(sr.ReportJSON, "desc") || sr.KeyVersion != 1 {
		t.Fatalf("report body: %+v", sr)
	}
	pk := decode[map[string]any](t, hs.do(uAuditor, "GET", "/api/reports/public-key?version=1", nil))
	raw, err := base64.StdEncoding.DecodeString(pk["publicKey"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if err := crypt.VerifyReport(ed25519.PublicKey(raw), []byte(sr.ReportJSON), sr.Signature); err != nil {
		t.Fatalf("signature does not verify: %v", err)
	}
	entries, _ := hs.st.Audit(context.Background(), store.AuditFilter{})
	if entries[len(entries)-1].Action != "report.export" {
		t.Fatalf("report export not audited: %+v", entries[len(entries)-1])
	}
}

func TestReportTamperDetected(t *testing.T) {
	hs := newHarness(t)
	hs.createFinding("Tamper target")
	sr := decode[signedReport](t, hs.do(uPM, "GET", "/api/reports?from=2026-11-01&to=2026-12-01", nil))
	pk := decode[map[string]any](t, hs.do(uPM, "GET", "/api/reports/public-key?version=1", nil))
	raw, _ := base64.StdEncoding.DecodeString(pk["publicKey"].(string))
	tampered := strings.Replace(sr.ReportJSON, "High", "Low", 1)
	if err := crypt.VerifyReport(ed25519.PublicKey(raw), []byte(tampered), sr.Signature); err == nil {
		t.Fatal("tampered report verified")
	}
}

func TestReportRangeValidation(t *testing.T) {
	hs := newHarness(t)
	for _, q := range []string{"from=2026-12-01&to=2026-11-01", "from=2026-11-01", "from=yesterday&to=2026-11-01", "from=2025-01-01&to=2026-12-01&asset=%3Cb%3E", "from=2020-01-01&to=2026-12-01"} {
		if w := hs.do(uPM, "GET", "/api/reports?"+q, nil); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", q, w.Code)
		}
	}
	if w := hs.do(uDev, "GET", "/api/reports?from=2026-11-01&to=2026-12-01", nil); w.Code != 403 {
		t.Errorf("developer report: %d", w.Code)
	}
}
