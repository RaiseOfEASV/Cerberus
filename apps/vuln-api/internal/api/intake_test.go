package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/auth"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

func (hs *harness) raw(u auth.User, method, path, ctype string, body []byte) *httptest.ResponseRecorder {
	hs.t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.Header.Set("Content-Type", ctype)
	r.Header.Set("Origin", base)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: hs.cookies[u.Sub]})
	w := httptest.NewRecorder()
	hs.h.ServeHTTP(w, r)
	return w
}

func trivyFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../intake/testdata/trivy-image.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTrivyImportAndReimportSkipsDuplicates(t *testing.T) {
	hs := newHarness(t)
	w := hs.raw(uAnalyst, "POST", "/api/import/trivy", "application/json", trivyFixture(t))
	res := decode[store.ImportResult](t, w)
	if w.Code != 200 || res.Created != 14 || res.Skipped != 0 {
		t.Fatalf("first import: %d %+v %s", w.Code, res, w.Body)
	}
	w = hs.raw(uPM, "POST", "/api/import/trivy", "application/json", trivyFixture(t))
	res = decode[store.ImportResult](t, w)
	if w.Code != 200 || res.Created != 0 || res.Skipped != 14 {
		t.Fatalf("re-import: %d %+v", w.Code, res)
	}
	// A remediated finding no longer blocks a re-detected one.
	items, _, _ := hs.st.ListFindings(context.Background(), store.Filter{Q: "CVE-2022-28391"})
	id := items[0].ID
	for _, st := range []string{"Triaged", "In Progress", "Remediated"} {
		hs.do(uAnalyst, "POST", "/api/findings/"+itoa(id)+"/status", map[string]any{"status": st})
	}
	res = decode[store.ImportResult](t, hs.raw(uAnalyst, "POST", "/api/import/trivy", "application/json", trivyFixture(t)))
	if res.Created != 1 || res.Skipped != 13 {
		t.Fatalf("after remediation: %+v", res)
	}
	entries, _ := hs.st.Audit(context.Background(), store.AuditFilter{Limit: 1000})
	summaries := 0
	for _, e := range entries {
		if e.Action == "import.trivy" {
			summaries++
		}
	}
	if summaries != 3 {
		t.Fatalf("import.trivy audit rows: %d", summaries)
	}
	if v, _ := hs.st.VerifyAudit(context.Background()); !v.OK {
		t.Fatal("audit chain broken by imports")
	}
}

func TestTrivyImportAssetOverride(t *testing.T) {
	hs := newHarness(t)
	w := hs.raw(uAnalyst, "POST", "/api/import/trivy?asset=billing-service", "application/json", trivyFixture(t))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if _, total, _ := hs.st.ListFindings(context.Background(), store.Filter{Asset: "billing-service"}); total != 14 {
		t.Fatalf("override asset: %d", total)
	}
	if w := hs.raw(uAnalyst, "POST", "/api/import/trivy?asset=%3Cb%3E", "application/json", trivyFixture(t)); w.Code != 400 {
		t.Fatalf("bad asset override: %d", w.Code)
	}
}

func TestImportForbiddenForDeveloper(t *testing.T) {
	hs := newHarness(t)
	for _, u := range []auth.User{uDev, uAuditor} {
		if w := hs.raw(u, "POST", "/api/import/trivy", "application/json", trivyFixture(t)); w.Code != 403 {
			t.Errorf("%s trivy import: %d", u.Role, w.Code)
		}
		if w := hs.raw(u, "POST", "/api/import/csv", "text/csv", []byte("title,asset,severity\nabc,a,Low\n")); w.Code != 403 {
			t.Errorf("%s csv import: %d", u.Role, w.Code)
		}
	}
}

func TestTrivyImportTooLarge(t *testing.T) {
	hs := newHarness(t)
	big := append([]byte(`{"SchemaVersion":2,"ArtifactName":"x","Results":[],"pad":"`), bytes.Repeat([]byte("a"), 5<<20)...)
	if w := hs.raw(uAnalyst, "POST", "/api/import/trivy", "application/json", append(big, '"', '}')); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("6 MiB import: %d", w.Code)
	}
}

func TestCSVImportBadSeverity400(t *testing.T) {
	hs := newHarness(t)
	w := hs.raw(uAnalyst, "POST", "/api/import/csv", "text/csv", []byte("title,asset,severity\nGood title,a,Low\nBad title,b,Severe\n"))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "row 3") {
		t.Fatalf("bad severity: %d %s", w.Code, w.Body)
	}
	if _, total, _ := hs.st.ListFindings(context.Background(), store.Filter{}); total != 0 {
		t.Fatal("partial import stored rows despite an error")
	}
	w = hs.raw(uAnalyst, "POST", "/api/import/csv", "text/csv", []byte("title,asset,severity\nab,a,Low\n"))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "row 2") {
		t.Fatalf("short title: %d %s", w.Code, w.Body)
	}
}

func TestCSVExportRoundTrip(t *testing.T) {
	hs := newHarness(t)
	w := hs.raw(uPM, "POST", "/api/import/csv", "text/csv", []byte("title,asset,severity,cvss\n=cmd|' /C calc'!A0,shop,High,7.5\nSecond one,shop,Low,\n"))
	if w.Code != 200 || decode[store.ImportResult](t, w).Created != 2 {
		t.Fatalf("csv import: %d %s", w.Code, w.Body)
	}
	w = hs.do(uAuditor, "GET", "/api/export/csv?severity=High", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		!strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("export: %d %v", w.Code, w.Header())
	}
	body := w.Body.String()
	if strings.Count(body, "\n") != 2 || !strings.Contains(body, "'=cmd") || !strings.Contains(body, "7.5") {
		t.Fatalf("export body:\n%s", body)
	}
	if w := hs.do(uDev, "GET", "/api/export/csv", nil); w.Code != 403 {
		t.Fatalf("developer export: %d", w.Code)
	}
}
