package store

import (
	"context"
	"math"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

type Dashboard struct {
	OpenBySeverity   map[domain.Severity]int `json:"openBySeverity"`
	ByStatus         map[domain.Status]int   `json:"byStatus"`
	Overdue          int                     `json:"overdue"`
	OpenWithDue      int                     `json:"openWithDue"`
	SLACompliancePct float64                 `json:"slaCompliancePct"`
	OverdueList      []Finding               `json:"overdueList"`
}

const openCond = `status NOT IN ('Remediated','Accepted Risk')`

// Dashboard is FR-RPT-01 (open by severity, all by status) and FR-RPT-04
// (SLA compliance: share of open findings with a due date that aren't overdue).
func (s *Store) Dashboard(ctx context.Context, now time.Time) (Dashboard, error) {
	d := Dashboard{OpenBySeverity: map[domain.Severity]int{}, ByStatus: map[domain.Status]int{}, OverdueList: []Finding{}}
	for _, sev := range domain.Severities {
		d.OpenBySeverity[sev] = 0
	}
	for _, st := range domain.Statuses {
		d.ByStatus[st] = 0
	}
	rows, err := s.pool.Query(ctx, `SELECT severity, status, count(*) FROM findings GROUP BY severity, status`)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var sev, st string
		var n int
		if err := rows.Scan(&sev, &st, &n); err != nil {
			rows.Close()
			return d, err
		}
		d.ByStatus[domain.Status(st)] += n
		if domain.IsOpen(domain.Status(st)) {
			d.OpenBySeverity[domain.Severity(sev)] += n
		}
	}
	rows.Close()
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE due_date IS NOT NULL), count(*) FILTER (WHERE due_date < $1)
		FROM findings WHERE `+openCond, now.UTC()).Scan(&d.OpenWithDue, &d.Overdue); err != nil {
		return d, err
	}
	d.SLACompliancePct = 100
	if d.OpenWithDue > 0 {
		d.SLACompliancePct = math.Round(float64(d.OpenWithDue-d.Overdue)/float64(d.OpenWithDue)*1000) / 10
	}
	lrows, err := s.pool.Query(ctx, `SELECT `+findingCols+` FROM findings WHERE `+openCond+` AND due_date < $1 ORDER BY due_date LIMIT 10`, now.UTC())
	if err != nil {
		return d, err
	}
	defer lrows.Close()
	for lrows.Next() {
		var f Finding
		if err := scanFinding(lrows, &f); err != nil {
			return d, err
		}
		d.OverdueList = append(d.OverdueList, f)
	}
	return d, lrows.Err()
}

type WeekCount struct {
	WeekStart time.Time `json:"weekStart"`
	Opened    int       `json:"opened"`
	Closed    int       `json:"closed"`
}

// mondayUTC is the start of the ISO week containing t.
func mondayUTC(t time.Time) time.Time {
	t = t.UTC()
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

// Trend is FR-RPT-05: findings opened vs closed (moved to Remediated or
// Accepted Risk, from the audit log) per ISO week, the last `weeks` weeks.
func (s *Store) Trend(ctx context.Context, weeks int, now time.Time) ([]WeekCount, error) {
	first := mondayUTC(now).AddDate(0, 0, -7*(weeks-1))
	out := make([]WeekCount, weeks)
	idx := map[time.Time]int{}
	for i := range out {
		out[i].WeekStart = first.AddDate(0, 0, 7*i)
		idx[out[i].WeekStart] = i
	}
	count := func(q string, add func(*WeekCount, int)) error {
		rows, err := s.pool.Query(ctx, q, first)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var wk time.Time
			var n int
			if err := rows.Scan(&wk, &n); err != nil {
				return err
			}
			if i, ok := idx[mondayUTC(wk)]; ok {
				add(&out[i], n)
			}
		}
		return rows.Err()
	}
	if err := count(`SELECT date_trunc('week', created_at AT TIME ZONE 'UTC'), count(*) FROM findings
		WHERE created_at >= $1 GROUP BY 1`, func(w *WeekCount, n int) { w.Opened += n }); err != nil {
		return nil, err
	}
	if err := count(`SELECT date_trunc('week', at AT TIME ZONE 'UTC'), count(*) FROM audit_log
		WHERE action = 'finding.status' AND details->>'to' IN ('Remediated','Accepted Risk') AND at >= $1 GROUP BY 1`,
		func(w *WeekCount, n int) { w.Closed += n }); err != nil {
		return nil, err
	}
	return out, nil
}
