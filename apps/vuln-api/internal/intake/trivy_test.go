package intake

import (
	"os"
	"strings"
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

func open(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestParseTrivyImage(t *testing.T) {
	got, err := ParseTrivy(open(t, "trivy-image.json"), 2000, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 14 {
		t.Fatalf("%d findings, want 14", len(got))
	}
	f := got[0]
	if f.Title != "CVE-2022-28391 in busybox 1.34.1-r3" || f.Asset != "python:3.6-alpine" || f.Severity != domain.SevHigh ||
		f.CVE != "CVE-2022-28391" || f.CVSS == nil || *f.CVSS != 8.8 || f.Source != "trivy" || len(f.Fingerprint) != 64 {
		t.Fatalf("first finding: %+v", f)
	}
	for _, want := range []string{"busybox: remote attackers may execute arbitrary code", "Fixed in: 1.34.1-r5", "https://avd.aquasec.com/nvd/cve-2022-28391"} {
		if !strings.Contains(f.Description, want) {
			t.Errorf("description lacks %q", want)
		}
	}
	var ghsa, low int
	for _, f := range got {
		if strings.HasPrefix(f.Title, "GHSA-") {
			ghsa++
			if f.CVE != "" {
				t.Errorf("GHSA id put in the CVE field: %+v", f)
			}
		}
		if f.Severity == domain.SevLow {
			low++
		}
	}
	if ghsa != 1 || low != 2 {
		t.Fatalf("ghsa %d low %d", ghsa, low)
	}
}

func TestParseTrivySBOM(t *testing.T) {
	got, err := ParseTrivy(open(t, "trivy-sbom.json"), 2000, "python-app")
	if err != nil || len(got) != 14 {
		t.Fatalf("sbom: %d %v", len(got), err)
	}
	for _, f := range got {
		if f.Asset != "python-app" {
			t.Fatalf("asset override not applied: %q", f.Asset)
		}
	}
	img, _ := ParseTrivy(open(t, "trivy-image.json"), 2000, "python-app")
	if img[0].Fingerprint != got[0].Fingerprint {
		t.Fatal("same vuln on the same asset must fingerprint the same from image and SBOM scans")
	}
}

func TestParseTrivyEOLImage(t *testing.T) {
	got, err := ParseTrivy(open(t, "trivy-alpine-eol.json"), 2000, "")
	if err != nil || len(got) != 1 || got[0].Severity != domain.SevCritical || got[0].Asset != "alpine:3.10" {
		t.Fatalf("eol image: %+v %v", got, err)
	}
}

func TestParseTrivyLimitsAndJunk(t *testing.T) {
	if _, err := ParseTrivy(open(t, "trivy-image.json"), 5, ""); err == nil || !strings.Contains(err.Error(), "at most 5") {
		t.Fatalf("over the limit: %v", err)
	}
	for _, junk := range []string{`{}`, `{"SchemaVersion": 1}`, `[1,2]`, `not json`, `{"SchemaVersion":2,"ArtifactName":"<script>","Results":[]}`} {
		if _, err := ParseTrivy(strings.NewReader(junk), 10, ""); err == nil {
			t.Errorf("accepted %q", junk)
		}
	}
	got, err := ParseTrivy(strings.NewReader(`{"SchemaVersion":2,"ArtifactName":"clean:1","Results":[{"Target":"x"}]}`), 10, "")
	if err != nil || len(got) != 0 {
		t.Fatalf("clean scan: %v %v", got, err)
	}
}

func TestParseTrivyUnknownSeverityIsLow(t *testing.T) {
	in := `{"SchemaVersion":2,"ArtifactName":"x:1","Results":[{"Target":"t","Vulnerabilities":[
		{"VulnerabilityID":"CVE-2026-0001","PkgName":"p","InstalledVersion":"1","Severity":"UNKNOWN"},
		{"VulnerabilityID":"CVE-2026-0001","PkgName":"p","InstalledVersion":"1","Severity":"UNKNOWN"}]}]}`
	got, err := ParseTrivy(strings.NewReader(in), 10, "")
	if err != nil || len(got) != 1 || got[0].Severity != domain.SevLow || got[0].CVSS != nil {
		t.Fatalf("unknown severity / in-file duplicate: %+v %v", got, err)
	}
}
