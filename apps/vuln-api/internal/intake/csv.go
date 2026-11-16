package intake

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

// ExportColumns is the CSV export layout, in order. No description, no
// reporter e-mail: exports leave the encrypted fields behind (FIND-11).
var ExportColumns = []string{"ref", "title", "asset", "severity", "cvss", "cve", "status", "assignee", "due_date", "discovered_at", "source"}

var importColumns = map[string]bool{"title": true, "asset": true, "severity": true, "cvss": true, "cve": true, "description": true, "discovered_at": true}

// RowError names the 1-based CSV row (header = row 1).
type RowError struct {
	Row int
	Err error
}

func (e *RowError) Error() string { return fmt.Sprintf("row %d: %v", e.Row, e.Err) }

// ReadCSV parses an import file: a header row with title, asset, severity
// and optionally cvss, cve, description, discovered_at (RFC 3339).
func ReadCSV(r io.Reader, max int) ([]store.NewFinding, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = 0 // every row must match the header's width
	header, err := cr.Read()
	if err != nil {
		return nil, errors.New("empty or unreadable CSV")
	}
	col := map[string]int{}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(h))
		if !importColumns[h] {
			return nil, fmt.Errorf("unknown column %q", h)
		}
		col[h] = i
	}
	for _, req := range []string{"title", "asset", "severity"} {
		if _, ok := col[req]; !ok {
			return nil, fmt.Errorf("missing required column %q", req)
		}
	}
	get := func(rec []string, name string) string {
		if i, ok := col[name]; ok {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	var out []store.NewFinding
	for row := 2; ; row++ {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, &RowError{row, errors.New("wrong number of fields")}
		}
		if len(out) >= max {
			return nil, fmt.Errorf("a CSV import may contain at most %d rows", max)
		}
		sev, err := domain.ParseSeverity(get(rec, "severity"))
		if err != nil {
			return nil, &RowError{row, errors.New("severity must be Critical, High, Medium or Low")}
		}
		nf := store.NewFinding{Title: get(rec, "title"), Asset: get(rec, "asset"), Severity: sev, CVE: get(rec, "cve"),
			Description: get(rec, "description"), Source: "csv"}
		if v := get(rec, "cvss"); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, &RowError{row, errors.New("cvss must be a number")}
			}
			nf.CVSS = &f
		}
		if v := get(rec, "discovered_at"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return nil, &RowError{row, errors.New("discovered_at must be RFC 3339, e.g. 2026-11-01T00:00:00Z")}
			}
			nf.DiscoveredAt = t
		}
		out = append(out, nf)
	}
	return out, nil
}

// cell neutralizes spreadsheet formulas (CSV injection).
func cell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func WriteCSV(w io.Writer, fs []store.Finding) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(ExportColumns); err != nil {
		return err
	}
	for _, f := range fs {
		cvss, due := "", ""
		if f.CVSS != nil {
			cvss = strconv.FormatFloat(*f.CVSS, 'f', 1, 64)
		}
		if f.DueDate != nil {
			due = f.DueDate.UTC().Format(time.RFC3339)
		}
		rec := []string{f.Ref, f.Title, f.Asset, string(f.Severity), cvss, f.CVE, string(f.Status), f.AssigneeName, due,
			f.DiscoveredAt.UTC().Format(time.RFC3339), f.Source}
		for i := range rec {
			rec[i] = cell(rec[i])
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
