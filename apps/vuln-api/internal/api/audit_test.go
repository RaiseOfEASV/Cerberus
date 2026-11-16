package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/auth"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/db/testdb"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

func TestAuditReadableByPMAndAuditorOnly(t *testing.T) {
	hs := newHarness(t)
	f := hs.createFinding("Audited")
	for _, p := range []string{"/api/audit", "/api/audit/verify", fmt.Sprintf("/api/findings/%d/history", f.ID)} {
		for u, want := range map[auth.User]int{uAnalyst: 403, uDev: 403, uPM: 200, uAuditor: 200} {
			if w := hs.do(u, "GET", p, nil); w.Code != want {
				t.Errorf("%s GET %s: %d, want %d", u.Role, p, w.Code, want)
			}
		}
	}
}

func TestFindingHistoryOrder(t *testing.T) {
	hs := newHarness(t)
	f := hs.createFinding("History")
	other := hs.createFinding("Other")
	id := fmt.Sprint(f.ID)
	hs.now = hs.now.Add(time.Minute)
	hs.do(uAnalyst, "POST", "/api/findings/"+id+"/status", map[string]any{"status": "Triaged"})
	hs.now = hs.now.Add(time.Minute)
	hs.do(uAnalyst, "POST", "/api/findings/"+id+"/assignment", map[string]any{"assigneeSub": uDev.Sub})
	hs.do(uAnalyst, "PATCH", fmt.Sprintf("/api/findings/%d", other.ID), map[string]any{"title": "Other changed"})
	hist := decode[[]store.AuditEntry](t, hs.do(uAuditor, "GET", "/api/findings/"+id+"/history", nil))
	got := []string{}
	for _, e := range hist {
		got = append(got, e.Action)
	}
	want := []string{"finding.create", "finding.status", "finding.assign"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("history = %v, want %v", got, want)
	}
	if !hist[0].At.Before(hist[2].At) {
		t.Fatal("history not oldest first")
	}
}

func TestAuditRecordsActorAndTime(t *testing.T) {
	hs := newHarness(t)
	f := hs.createFinding("Who did what")
	hs.now = time.Date(2026, 11, 20, 8, 30, 0, 0, time.UTC)
	hs.do(uPM, "POST", fmt.Sprintf("/api/findings/%d/status", f.ID), map[string]any{"status": "Triaged", "note": "looks real"})
	entries := decode[[]store.AuditEntry](t, hs.do(uAuditor, "GET", "/api/audit?actor=pm-1", nil))
	if len(entries) != 1 {
		t.Fatalf("entries by pm: %+v", entries)
	}
	e := entries[0]
	if e.ActorSub != uPM.Sub || e.ActorRole != "pm" || e.Action != "finding.status" || e.FindingID == nil || *e.FindingID != f.ID ||
		!e.At.Equal(hs.now) || e.Details["to"] != "Triaged" || e.Details["note"] != "looks real" {
		t.Fatalf("entry: %+v", e)
	}
	byTime := decode[[]store.AuditEntry](t, hs.do(uAuditor, "GET", "/api/audit?from=2026-11-20T00:00:00Z&to=2026-11-21T00:00:00Z", nil))
	if len(byTime) != 1 {
		t.Fatalf("time filter: %d", len(byTime))
	}
	for _, q := range []string{"from=yesterday", "finding=abc", "limit=0", "limit=5000"} {
		if w := hs.do(uAuditor, "GET", "/api/audit?"+q, nil); w.Code != 400 {
			t.Errorf("%s: %d", q, w.Code)
		}
	}
}

func TestVerifyEndpointReportsTamper(t *testing.T) {
	hs := newHarness(t)
	for i := 0; i < 3; i++ {
		hs.createFinding(fmt.Sprintf("Chain %d", i))
	}
	v := decode[store.VerifyResult](t, hs.do(uAuditor, "GET", "/api/audit/verify", nil))
	if !v.OK || v.Checked != 3 {
		t.Fatalf("clean chain: %+v", v)
	}
	owner := testdb.OwnerOf(t, hs.st.Pool())
	ctx := context.Background()
	for _, q := range []string{`ALTER TABLE audit_log DISABLE TRIGGER audit_log_immutable`,
		`UPDATE audit_log SET actor_sub = 'someone-else' WHERE id = (SELECT min(id) FROM audit_log)`,
		`ALTER TABLE audit_log ENABLE TRIGGER audit_log_immutable`} {
		if _, err := owner.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	v = decode[store.VerifyResult](t, hs.do(uPM, "GET", "/api/audit/verify", nil))
	if v.OK || v.FirstBadID == nil {
		t.Fatalf("tampered chain reported OK: %+v", v)
	}
}
