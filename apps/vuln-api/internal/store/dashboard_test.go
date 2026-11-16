package store_test

import (
	"testing"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

// fixture: 5 findings at t0 (Mon 2026-11-16 10:00 UTC):
//
//	c  Critical  Triaged     due t0+7d
//	h  High      In Progress due t0+30d (triaged)
//	m  Medium    New         no due
//	l  Low       Remediated  (was triaged)
//	a  High      Accepted Risk
func dashFixture(t *testing.T) (*env, map[string]store.Finding) {
	e := newEnv(t)
	fs := map[string]store.Finding{
		"c": e.create("crit", "api", domain.SevCritical),
		"h": e.create("high", "api", domain.SevHigh),
		"m": e.create("med", "web", domain.SevMedium),
		"l": e.create("low", "web", domain.SevLow),
		"a": e.create("acc", "web", domain.SevHigh),
	}
	move := func(k string, sts ...domain.Status) {
		for _, s := range sts {
			if _, err := e.s.ChangeStatus(ctx, pm, fs[k].ID, s, ""); err != nil {
				t.Fatal(err)
			}
		}
	}
	move("c", domain.StTriaged)
	move("h", domain.StTriaged, domain.StInProgress)
	move("l", domain.StTriaged, domain.StInProgress, domain.StRemediated)
	move("a", domain.StAccepted)
	return e, fs
}

func TestDashboardCounts(t *testing.T) {
	e, _ := dashFixture(t)
	d, err := e.s.Dashboard(ctx, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	wantSev := map[domain.Severity]int{domain.SevCritical: 1, domain.SevHigh: 1, domain.SevMedium: 1, domain.SevLow: 0}
	for k, v := range wantSev {
		if d.OpenBySeverity[k] != v {
			t.Errorf("open %s = %d, want %d", k, d.OpenBySeverity[k], v)
		}
	}
	wantSt := map[domain.Status]int{domain.StNew: 1, domain.StTriaged: 1, domain.StInProgress: 1, domain.StRemediated: 1, domain.StAccepted: 1}
	for k, v := range wantSt {
		if d.ByStatus[k] != v {
			t.Errorf("status %s = %d, want %d", k, d.ByStatus[k], v)
		}
	}
	if d.Overdue != 0 || d.OpenWithDue != 2 || d.SLACompliancePct != 100 {
		t.Fatalf("SLA numbers at t0: %+v", d)
	}
}

func TestSLACompliance(t *testing.T) {
	e, fs := dashFixture(t)
	g := e.create("medium two", "api", domain.SevMedium)
	if _, err := e.s.ChangeStatus(ctx, pm, g.ID, domain.StTriaged, ""); err != nil { // due t0+90d
		t.Fatal(err)
	}
	// 3 open findings with a due date; at t0+8d only the critical (due t0+7d) is overdue.
	d, err := e.s.Dashboard(ctx, t0.Add(8*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if d.OpenWithDue != 3 || d.Overdue != 1 || d.SLACompliancePct != 66.7 {
		t.Fatalf("compliance: %+v", d)
	}
	if len(d.OverdueList) != 1 || d.OverdueList[0].ID != fs["c"].ID {
		t.Fatalf("overdue list: %+v", d.OverdueList)
	}
}

func TestTrendWeeks(t *testing.T) {
	e, _ := dashFixture(t)
	*e.now = t0.Add(7 * 24 * time.Hour) // next week: one more opened, one remediated
	n := e.create("next week", "api", domain.SevLow)
	for _, s := range []domain.Status{domain.StTriaged, domain.StInProgress, domain.StRemediated} {
		if _, err := e.s.ChangeStatus(ctx, pm, n.ID, s, ""); err != nil {
			t.Fatal(err)
		}
	}
	weeks, err := e.s.Trend(ctx, 3, t0.Add(8*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(weeks) != 3 {
		t.Fatalf("%d weeks", len(weeks))
	}
	want := []store.WeekCount{
		{WeekStart: time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC), Opened: 0, Closed: 0},
		{WeekStart: time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC), Opened: 5, Closed: 2}, // l remediated, a accepted
		{WeekStart: time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC), Opened: 1, Closed: 1},
	}
	for i := range want {
		if !weeks[i].WeekStart.Equal(want[i].WeekStart) || weeks[i].Opened != want[i].Opened || weeks[i].Closed != want[i].Closed {
			t.Errorf("week %d = %+v, want %+v", i, weeks[i], want[i])
		}
	}
}

func TestReportContents(t *testing.T) {
	e, _ := dashFixture(t)
	r, err := e.s.Report(ctx, t0.Add(-time.Hour), t0.Add(time.Hour), "web")
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 3 || r.BySeverity[domain.SevHigh] != 1 || r.ByStatus[domain.StNew] != 1 || len(r.Findings) != 3 || r.Asset != "web" {
		t.Fatalf("report: %+v", r)
	}
	r, _ = e.s.Report(ctx, t0.Add(time.Hour), t0.Add(2*time.Hour), "")
	if r.Total != 0 || len(r.Findings) != 0 {
		t.Fatalf("empty range: %+v", r)
	}
}
