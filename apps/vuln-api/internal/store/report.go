package store

import (
	"context"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

// Report is FR-RPT-02's export: findings discovered in [From, To),
// optionally for one asset. Items omit description and reporter e-mail.
type Report struct {
	GeneratedAt time.Time               `json:"generatedAt"`
	From        time.Time               `json:"from"`
	To          time.Time               `json:"to"`
	Asset       string                  `json:"asset,omitempty"`
	Total       int                     `json:"total"`
	BySeverity  map[domain.Severity]int `json:"bySeverity"`
	ByStatus    map[domain.Status]int   `json:"byStatus"`
	Findings    []ReportItem            `json:"findings"`
}

type ReportItem struct {
	Ref          string          `json:"ref"`
	Title        string          `json:"title"`
	Asset        string          `json:"asset"`
	Severity     domain.Severity `json:"severity"`
	Status       domain.Status   `json:"status"`
	CVSS         *float64        `json:"cvss,omitempty"`
	CVE          string          `json:"cve,omitempty"`
	AssigneeName string          `json:"assignee,omitempty"`
	DueDate      *time.Time      `json:"dueDate,omitempty"`
	DiscoveredAt time.Time       `json:"discoveredAt"`
}

func (s *Store) Report(ctx context.Context, from, to time.Time, asset string) (Report, error) {
	r := Report{GeneratedAt: s.ts(), From: from.UTC(), To: to.UTC(), Asset: asset,
		BySeverity: map[domain.Severity]int{}, ByStatus: map[domain.Status]int{}, Findings: []ReportItem{}}
	q := `SELECT ref, title, asset, severity, status, cvss, cve, assignee_name, due_date, discovered_at FROM findings
		WHERE discovered_at >= $1 AND discovered_at < $2`
	args := []any{r.From, r.To}
	if asset != "" {
		q += ` AND asset = $3`
		args = append(args, asset)
	}
	rows, err := s.pool.Query(ctx, q+` ORDER BY discovered_at, id`, args...)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var it ReportItem
		if err := rows.Scan(&it.Ref, &it.Title, &it.Asset, &it.Severity, &it.Status, &it.CVSS, &it.CVE, &it.AssigneeName, &it.DueDate, &it.DiscoveredAt); err != nil {
			return r, err
		}
		r.Findings = append(r.Findings, it)
		r.BySeverity[it.Severity]++
		r.ByStatus[it.Status]++
	}
	r.Total = len(r.Findings)
	return r, rows.Err()
}
