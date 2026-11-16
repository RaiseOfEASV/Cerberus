// Package intake turns scanner output and spreadsheets into findings:
// Trivy JSON (image and SBOM scans share one schema, FIND-09) and CSV
// (FIND-08). It parses and types; the API validates each finding's fields.
package intake

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

type trivyReport struct {
	SchemaVersion int    `json:"SchemaVersion"`
	ArtifactName  string `json:"ArtifactName"`
	Results       []struct {
		Target          string `json:"Target"`
		Vulnerabilities []struct {
			VulnerabilityID  string `json:"VulnerabilityID"`
			PkgName          string `json:"PkgName"`
			InstalledVersion string `json:"InstalledVersion"`
			FixedVersion     string `json:"FixedVersion"`
			Severity         string `json:"Severity"`
			Title            string `json:"Title"`
			Description      string `json:"Description"`
			PrimaryURL       string `json:"PrimaryURL"`
			CVSS             map[string]struct {
				V3Score float64 `json:"V3Score"`
			} `json:"CVSS"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
}

var (
	cveRe      = regexp.MustCompile(`^CVE-\d{4}-\d{4,}$`)
	severities = map[string]domain.Severity{"CRITICAL": domain.SevCritical, "HIGH": domain.SevHigh, "MEDIUM": domain.SevMedium, "LOW": domain.SevLow, "UNKNOWN": domain.SevLow}
)

// Fingerprint identifies "this vulnerability in this package on this asset"
// across scans, so a re-scan doesn't duplicate open findings.
func Fingerprint(asset, vulnID, pkg string) string {
	s := sha256.Sum256([]byte(asset + "|" + vulnID + "|" + pkg))
	return hex.EncodeToString(s[:])
}

// ParseTrivy reads `trivy image|sbom --format json` output. asset, when
// non-empty, overrides ArtifactName (an SBOM scan's ArtifactName is the
// SBOM file path). At most max findings; duplicates within the file collapse.
func ParseTrivy(r io.Reader, max int, asset string) ([]store.NewFinding, error) {
	var rep trivyReport
	if err := json.NewDecoder(r).Decode(&rep); err != nil {
		return nil, errors.New("not a Trivy JSON report")
	}
	if rep.SchemaVersion != 2 || rep.ArtifactName == "" {
		return nil, errors.New("not a Trivy JSON report (need SchemaVersion 2 and ArtifactName)")
	}
	if asset == "" {
		asset = rep.ArtifactName
	}
	if !domain.ValidAsset(asset) {
		return nil, fmt.Errorf("asset %q is not a valid asset name; pass ?asset=", asset)
	}
	now := time.Now().UTC()
	seen := map[string]bool{}
	var out []store.NewFinding
	for _, res := range rep.Results {
		for _, v := range res.Vulnerabilities {
			if v.VulnerabilityID == "" || v.PkgName == "" {
				continue
			}
			fp := Fingerprint(asset, v.VulnerabilityID, v.PkgName)
			if seen[fp] {
				continue
			}
			seen[fp] = true
			if len(out) >= max {
				return nil, fmt.Errorf("a report may contain at most %d findings", max)
			}
			sev, ok := severities[strings.ToUpper(v.Severity)]
			if !ok {
				sev = domain.SevLow
			}
			nf := store.NewFinding{
				Title:        truncate(strings.TrimSpace(v.VulnerabilityID+" in "+v.PkgName+" "+v.InstalledVersion), 200),
				Description:  truncate(describe(v.Title, v.Description, v.FixedVersion, v.PrimaryURL, res.Target), 20000),
				Asset:        asset,
				Severity:     sev,
				Source:       "trivy",
				Fingerprint:  fp,
				DiscoveredAt: now,
			}
			if cveRe.MatchString(v.VulnerabilityID) {
				nf.CVE = v.VulnerabilityID
			}
			if n, ok := v.CVSS["nvd"]; ok && n.V3Score > 0 && n.V3Score <= 10 {
				score := math.Round(n.V3Score*10) / 10
				nf.CVSS = &score
			}
			out = append(out, nf)
		}
	}
	return out, nil
}

func describe(title, desc, fixed, url, target string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString(title + "\n\n")
	}
	if desc != "" {
		b.WriteString(desc + "\n\n")
	}
	if fixed != "" {
		b.WriteString("Fixed in: " + fixed + "\n")
	} else {
		b.WriteString("No fixed version available yet.\n")
	}
	if target != "" {
		b.WriteString("Scan target: " + target + "\n")
	}
	if url != "" {
		b.WriteString(url + "\n")
	}
	return strings.TrimSpace(b.String())
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
