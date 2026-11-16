package api

import (
	"fmt"
	"math"
	"regexp"
	"unicode/utf8"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

var (
	cveRe = regexp.MustCompile(`^CVE-\d{4}-\d{4,}$`)
)

func checkTitle(s string) error {
	if n := utf8.RuneCountInString(s); n < 3 || n > 200 {
		return fmt.Errorf("title must be 3-200 characters")
	}
	return nil
}

func checkDescription(s string) error {
	if utf8.RuneCountInString(s) > 20000 {
		return fmt.Errorf("description must be at most 20000 characters")
	}
	return nil
}

func checkAsset(s string) error {
	if !domain.ValidAsset(s) {
		return fmt.Errorf("asset must be 1-200 characters of letters, digits, space and . : / @ - _")
	}
	return nil
}

func checkCVSS(v float64) error {
	if v < 0 || v > 10 || math.Abs(v*10-math.Round(v*10)) > 1e-9 {
		return fmt.Errorf("cvss must be 0.0-10.0 with one decimal")
	}
	return nil
}

func checkCVE(s string) error {
	if s != "" && !cveRe.MatchString(s) {
		return fmt.Errorf("cve must look like CVE-2026-12345")
	}
	return nil
}

func parseSeverity(s string) (domain.Severity, error) {
	v, err := domain.ParseSeverity(s)
	if err != nil {
		return "", fmt.Errorf("severity must be Critical, High, Medium or Low")
	}
	return v, nil
}
