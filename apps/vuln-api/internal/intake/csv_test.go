package intake

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

func TestCSVFormulaEscaped(t *testing.T) {
	var buf bytes.Buffer
	cv := 9.8
	due := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	err := WriteCSV(&buf, []store.Finding{
		{Ref: "VM-1", Title: `=HYPERLINK("http://evil","x")`, Asset: "+cmd", Severity: domain.SevHigh, CVSS: &cv, CVE: "CVE-2026-1",
			Status: domain.StTriaged, AssigneeName: "@admin", DueDate: &due, DiscoveredAt: due, Source: "internal"},
		{Ref: "VM-2", Title: "-2+3", Asset: "\tTab", Severity: domain.SevLow, Status: domain.StNew, DiscoveredAt: due, Source: "csv"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if lines[0] != "ref,title,asset,severity,cvss,cve,status,assignee,due_date,discovered_at,source" {
		t.Fatalf("header %q", lines[0])
	}
	for _, want := range []string{`"'=HYPERLINK(""http://evil"",""x"")"`, "'+cmd", "'@admin", "'-2+3", "'\tTab", "9.8", "2026-12-01T00:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "description") {
		t.Fatal("export must not contain descriptions")
	}
}

func TestReadCSV(t *testing.T) {
	in := "title,asset,severity,cvss,cve,description,discovered_at\n" +
		"Weak cipher suites,edge-lb,Medium,5.3,,TLS 1.0 enabled,2026-11-01T00:00:00Z\n" +
		"Open S3 bucket,storage,High,,,,\n"
	got, err := ReadCSV(strings.NewReader(in), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Asset != "edge-lb" || got[0].Severity != domain.SevMedium || *got[0].CVSS != 5.3 ||
		got[0].Description != "TLS 1.0 enabled" || got[0].Source != "csv" || !got[0].DiscoveredAt.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("row 1: %+v", got[0])
	}
	if got[1].CVSS != nil || !got[1].DiscoveredAt.IsZero() {
		t.Fatalf("row 2 optional fields: %+v", got[1])
	}
}

func TestReadCSVErrorsNameTheRow(t *testing.T) {
	cases := map[string]string{
		"title,asset,severity\nA ok title,a,Low\nB title,b,Severe\n":    "row 3",
		"title,asset,severity\nX title,a,High,extra\n":                  "row 2",
		"title,asset\nNo severity column,a\n":                           "severity",
		"title,asset,severity,owner\nA title,a,Low,bob\n":               "unknown column",
		"title,asset,severity,cvss\nA title,a,Low,high\n":               "row 2",
		"title,asset,severity,discovered_at\nA title,a,Low,yesterday\n": "row 2",
	}
	for in, want := range cases {
		if _, err := ReadCSV(strings.NewReader(in), 100); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want mention of %q", in, err, want)
		}
	}
	many := "title,asset,severity\n" + strings.Repeat("Some title,a,Low\n", 11)
	if _, err := ReadCSV(strings.NewReader(many), 10); err == nil || !strings.Contains(err.Error(), "at most 10") {
		t.Errorf("row limit: %v", err)
	}
}
