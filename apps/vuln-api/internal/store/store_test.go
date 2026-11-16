package store_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/crypt/crypttest"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/db/testdb"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

var (
	ctx     = context.Background()
	pm      = store.Actor{Sub: "pm-1", Name: "Pat PM", Role: domain.RolePM}
	analyst = store.Actor{Sub: "an-1", Name: "Ana Analyst", Role: domain.RoleAnalyst}
	dev     = store.Actor{Sub: "dev-1", Name: "Dee Dev", Role: domain.RoleDeveloper}
	t0      = time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
)

type env struct {
	s   *store.Store
	now *time.Time
	t   *testing.T
}

func newEnv(t *testing.T) *env {
	t.Helper()
	now := t0
	app := testdb.New(t)
	e := &env{now: &now, t: t}
	e.s = store.New(app, crypttest.New(), func() time.Time { return *e.now })
	return e
}

func (e *env) create(title, asset string, sev domain.Severity) store.Finding {
	e.t.Helper()
	f, err := e.s.CreateFinding(ctx, analyst, store.NewFinding{
		Title: title, Description: "details of " + title, Asset: asset, Severity: sev,
		Source: "internal", DiscoveredAt: t0,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return f
}

func TestCreateEncryptsDescription(t *testing.T) {
	e := newEnv(t)
	f, err := e.s.CreateFinding(ctx, analyst, store.NewFinding{
		Title: "SQLi", Description: "UNIQUE-PLAINTEXT-123", Asset: "quote-api", Severity: domain.SevHigh,
		Source: "external", ReporterEmail: "reporter@example.com", DiscoveredAt: t0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != domain.StNew || len(f.Ref) != 13 || f.Ref[:3] != "VM-" {
		t.Fatalf("new finding: %+v", f)
	}
	var desc, email []byte
	if err := e.s.Pool().QueryRow(ctx, `SELECT description_ct, reporter_email_ct FROM findings WHERE id=$1`, f.ID).Scan(&desc, &email); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(desc, []byte("UNIQUE-PLAINTEXT-123")) || bytes.Contains(email, []byte("reporter@")) {
		t.Fatal("plaintext stored in the database")
	}
	got, err := e.s.GetFinding(ctx, f.ID)
	if err != nil || got.Description != "UNIQUE-PLAINTEXT-123" || got.ReporterEmail != "reporter@example.com" {
		t.Fatalf("GetFinding = %+v, %v", got, err)
	}
}

func TestSwappedCiphertextFails(t *testing.T) {
	e := newEnv(t)
	a := e.create("A", "x", domain.SevLow)
	b := e.create("B", "x", domain.SevLow)
	owner := testdb.OwnerOf(t, e.s.Pool())
	if _, err := owner.Exec(ctx, `UPDATE findings SET description_ct = (SELECT description_ct FROM findings WHERE id=$1),
		wrapped_dek = (SELECT wrapped_dek FROM findings WHERE id=$1) WHERE id=$2`, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	got, err := e.s.GetFinding(ctx, b.ID)
	if err == nil {
		t.Fatalf("swapped ciphertext decrypted: %q", got.Description)
	}
	if !errors.Is(err, store.ErrDecrypt) {
		t.Fatalf("want ErrDecrypt, got %v", err)
	}
}

func TestFilterCombos(t *testing.T) {
	e := newEnv(t)
	c1 := e.create("Log4Shell in api", "quote-api", domain.SevCritical)
	_ = e.create("Weak TLS", "edge", domain.SevMedium)
	h1 := e.create("XSS in search", "quote-web", domain.SevHigh)
	_ = e.create("Old nginx", "quote-web", domain.SevLow)
	if _, err := e.s.ChangeStatus(ctx, analyst, c1.ID, domain.StTriaged, ""); err != nil {
		t.Fatal(err)
	}
	due := t0.Add(24 * time.Hour)
	if _, err := e.s.Assign(ctx, analyst, h1.ID, dev.Sub, dev.Name, &due); err != nil {
		t.Fatal(err)
	}
	count := func(f store.Filter) int {
		t.Helper()
		items, total, err := e.s.ListFindings(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != total && f.Limit == 0 {
			t.Fatalf("items %d != total %d", len(items), total)
		}
		for _, it := range items {
			if it.Description != "" {
				t.Fatal("list rows must not carry decrypted descriptions")
			}
		}
		return total
	}
	cases := []struct {
		name string
		f    store.Filter
		want int
	}{
		{"all", store.Filter{}, 4},
		{"status New", store.Filter{Status: []domain.Status{domain.StNew}}, 3},
		{"status New+Triaged", store.Filter{Status: []domain.Status{domain.StNew, domain.StTriaged}}, 4},
		{"severity Critical/High", store.Filter{Severity: []domain.Severity{domain.SevCritical, domain.SevHigh}}, 2},
		{"assignee", store.Filter{AssigneeSub: dev.Sub}, 1},
		{"asset exact", store.Filter{Asset: "quote-web"}, 2},
		{"q title", store.Filter{Q: "log4"}, 1},
		{"q ref", store.Filter{Q: c1.Ref}, 1},
		{"combined", store.Filter{Asset: "quote-web", Severity: []domain.Severity{domain.SevHigh}}, 1},
		{"q is not a wildcard", store.Filter{Q: "%"}, 0},
	}
	for _, c := range cases {
		if got := count(c.f); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
	*e.now = t0.Add(48 * time.Hour) // h1 (due t0+24h) and c1 (due t0+7d) - only h1 overdue
	if got := count(store.Filter{Overdue: true}); got != 1 {
		t.Errorf("overdue: got %d, want 1", got)
	}
	items, total, _ := e.s.ListFindings(ctx, store.Filter{Limit: 2})
	if len(items) != 2 || total != 4 {
		t.Errorf("pagination: %d items, total %d", len(items), total)
	}
}

func TestTriageSetsSLADue(t *testing.T) {
	e := newEnv(t)
	f := e.create("RCE", "api", domain.SevCritical)
	*e.now = t0.Add(time.Hour)
	got, err := e.s.ChangeStatus(ctx, analyst, f.ID, domain.StTriaged, "confirmed")
	if err != nil {
		t.Fatal(err)
	}
	if got.TriagedAt == nil || !got.TriagedAt.Equal(t0.Add(time.Hour)) {
		t.Fatalf("triaged_at = %v", got.TriagedAt)
	}
	if want := t0.Add(time.Hour + 7*24*time.Hour); got.DueDate == nil || !got.DueDate.Equal(want) {
		t.Fatalf("due = %v, want %v", got.DueDate, want)
	}
	// An explicit due date set before triage is kept.
	g := e.create("Low thing", "api", domain.SevLow)
	explicit := t0.Add(3 * 24 * time.Hour)
	if _, err := e.s.Assign(ctx, pm, g.ID, dev.Sub, dev.Name, &explicit); err != nil {
		t.Fatal(err)
	}
	got, _ = e.s.ChangeStatus(ctx, pm, g.ID, domain.StTriaged, "")
	if !got.DueDate.Equal(explicit) {
		t.Fatalf("explicit due date overwritten: %v", got.DueDate)
	}
	// SLA targets are read at triage time.
	if err := e.s.SetSLA(ctx, pm, domain.SLA{domain.SevCritical: 1, domain.SevHigh: 2, domain.SevMedium: 3, domain.SevLow: 4}); err != nil {
		t.Fatal(err)
	}
	h := e.create("Another", "api", domain.SevHigh)
	got, _ = e.s.ChangeStatus(ctx, pm, h.ID, domain.StTriaged, "")
	if want := t0.Add(time.Hour + 2*24*time.Hour); !got.DueDate.Equal(want) {
		t.Fatalf("due with custom SLA = %v, want %v", got.DueDate, want)
	}
}

func TestIllegalTransition(t *testing.T) {
	e := newEnv(t)
	f := e.create("x", "y", domain.SevLow)
	before := auditCount(t, e)
	if _, err := e.s.ChangeStatus(ctx, analyst, f.ID, domain.StRemediated, ""); !errors.Is(err, store.ErrIllegalTransition) {
		t.Fatalf("want ErrIllegalTransition, got %v", err)
	}
	if _, err := e.s.ChangeStatus(ctx, dev, f.ID, domain.StAccepted, ""); !errors.Is(err, store.ErrIllegalTransition) {
		t.Fatalf("developer accepting risk: want ErrIllegalTransition, got %v", err)
	}
	if after := auditCount(t, e); after != before {
		t.Fatalf("audit rows %d -> %d on a rejected change", before, after)
	}
	if _, err := e.s.ChangeStatus(ctx, analyst, 9999, domain.StTriaged, ""); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUpdateAuditsChangedFieldsOnly(t *testing.T) {
	e := newEnv(t)
	f := e.create("Old title", "api", domain.SevLow)
	newTitle, newDesc := "New title", "secret new description"
	sev := domain.SevHigh
	if _, err := e.s.UpdateFinding(ctx, analyst, f.ID, store.Patch{Title: &newTitle, Description: &newDesc, Severity: &sev}); err != nil {
		t.Fatal(err)
	}
	got, _ := e.s.GetFinding(ctx, f.ID)
	if got.Title != newTitle || got.Description != newDesc || got.Severity != sev {
		t.Fatalf("update not applied: %+v", got)
	}
	id := f.ID
	entries, err := e.s.Audit(ctx, store.AuditFilter{FindingID: &id})
	if err != nil {
		t.Fatal(err)
	}
	last := entries[len(entries)-1]
	if last.Action != "finding.update" {
		t.Fatalf("last action %q", last.Action)
	}
	d := fmt.Sprint(last.Details)
	if !bytes.Contains([]byte(d), []byte("Old title")) || !bytes.Contains([]byte(d), []byte("description")) {
		t.Fatalf("details missing changes: %v", last.Details)
	}
	if bytes.Contains([]byte(d), []byte("secret new description")) {
		t.Fatal("audit details leak the decrypted description")
	}
}

func TestLinkSymmetricAndNoSelf(t *testing.T) {
	e := newEnv(t)
	a := e.create("a", "x", domain.SevLow)
	b := e.create("b", "x", domain.SevLow)
	if err := e.s.Link(ctx, analyst, b.ID, a.ID, store.LinkDuplicate); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{a.ID, b.ID} {
		ls, err := e.s.Links(ctx, id)
		if err != nil || len(ls) != 1 || ls[0].Kind != store.LinkDuplicate {
			t.Fatalf("Links(%d) = %+v, %v", id, ls, err)
		}
	}
	if err := e.s.Link(ctx, analyst, a.ID, b.ID, store.LinkRelated); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate link: want ErrConflict, got %v", err)
	}
	if err := e.s.Link(ctx, analyst, a.ID, a.ID, store.LinkRelated); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("self link: want ErrInvalid, got %v", err)
	}
	if err := e.s.Link(ctx, analyst, a.ID, 9999, store.LinkRelated); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing target: want ErrNotFound, got %v", err)
	}
	if err := e.s.Unlink(ctx, analyst, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if ls, _ := e.s.Links(ctx, a.ID); len(ls) != 0 {
		t.Fatalf("unlink left %v", ls)
	}
}

func TestAttachmentRoundTripEncrypted(t *testing.T) {
	e := newEnv(t)
	f := e.create("a", "x", domain.SevLow)
	data := []byte("PNG-ish evidence bytes UNIQUE-ATT")
	att, err := e.s.AddAttachment(ctx, analyst, f.ID, "shot.png", "image/png", data)
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	_ = e.s.Pool().QueryRow(ctx, `SELECT data_ct FROM attachments WHERE id=$1`, att.ID).Scan(&raw)
	if bytes.Contains(raw, []byte("UNIQUE-ATT")) {
		t.Fatal("attachment stored in plaintext")
	}
	meta, got, err := e.s.Attachment(ctx, f.ID, att.ID)
	if err != nil || !bytes.Equal(got, data) || meta.ContentType != "image/png" || meta.Size != len(data) {
		t.Fatalf("Attachment = %+v %q %v", meta, got, err)
	}
	other := e.create("b", "x", domain.SevLow)
	if _, _, err := e.s.Attachment(ctx, other.ID, att.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("attachment via the wrong finding: want ErrNotFound, got %v", err)
	}
	list, _ := e.s.Attachments(ctx, f.ID)
	if len(list) != 1 || list[0].Name != "shot.png" {
		t.Fatalf("Attachments = %+v", list)
	}
}

func TestAttachmentLimit(t *testing.T) {
	e := newEnv(t)
	f := e.create("a", "x", domain.SevLow)
	for i := 0; i < store.MaxAttachments; i++ {
		if _, err := e.s.AddAttachment(ctx, analyst, f.ID, fmt.Sprintf("%d.txt", i), "text/plain", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.s.AddAttachment(ctx, analyst, f.ID, "one-too-many.txt", "text/plain", []byte("x")); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("want ErrConflict past the limit, got %v", err)
	}
}

func auditCount(t *testing.T, e *env) int {
	t.Helper()
	var n int
	if err := e.s.Pool().QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAuditChainVerifies(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 10; i++ {
		e.create(fmt.Sprintf("f%d", i), "x", domain.SevLow)
	}
	res, err := e.s.VerifyAudit(ctx)
	if err != nil || !res.OK || res.Checked != 10 || res.FirstBadID != nil {
		t.Fatalf("VerifyAudit = %+v, %v", res, err)
	}
}

func TestAuditTamperDetected(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 10; i++ {
		e.create(fmt.Sprintf("f%d", i), "x", domain.SevLow)
	}
	owner := testdb.OwnerOf(t, e.s.Pool())
	var fifth int64
	_ = e.s.Pool().QueryRow(ctx, `SELECT id FROM audit_log ORDER BY id OFFSET 4 LIMIT 1`).Scan(&fifth)
	for _, q := range []string{
		`ALTER TABLE audit_log DISABLE TRIGGER audit_log_immutable`,
		fmt.Sprintf(`UPDATE audit_log SET details = '{"title":"forged"}' WHERE id = %d`, fifth),
		`ALTER TABLE audit_log ENABLE TRIGGER audit_log_immutable`,
	} {
		if _, err := owner.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	res, err := e.s.VerifyAudit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.FirstBadID == nil || *res.FirstBadID != fifth {
		t.Fatalf("VerifyAudit = %+v, want first bad id %d", res, fifth)
	}
}

func TestConcurrentMutationsKeepChain(t *testing.T) {
	e := newEnv(t)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := e.s.CreateFinding(ctx, analyst, store.NewFinding{Title: fmt.Sprintf("c%d", i), Description: "d", Asset: "x",
				Severity: domain.SevLow, Source: "internal", DiscoveredAt: t0})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	res, err := e.s.VerifyAudit(ctx)
	if err != nil || !res.OK || res.Checked != 20 {
		t.Fatalf("VerifyAudit after concurrent writes = %+v, %v", res, err)
	}
	rows, _ := e.s.Pool().Query(ctx, `SELECT prev_hash, hash FROM audit_log ORDER BY id`)
	defer rows.Close()
	var prev []byte
	for i := 0; rows.Next(); i++ {
		var p, h []byte
		_ = rows.Scan(&p, &h)
		if i > 0 && !bytes.Equal(p, prev) {
			t.Fatalf("row %d prev_hash does not match previous hash", i)
		}
		prev = h
	}
}

func TestSLARoundTrip(t *testing.T) {
	e := newEnv(t)
	sla, err := e.s.SLA(ctx)
	if err != nil || sla[domain.SevCritical] != 7 || sla[domain.SevLow] != 180 {
		t.Fatalf("default SLA = %v, %v", sla, err)
	}
	if err := e.s.SetSLA(ctx, pm, domain.SLA{domain.SevCritical: 3}); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("partial SLA: want ErrInvalid, got %v", err)
	}
}

func TestAuditChainVerifiesAcrossAllActions(t *testing.T) {
	e := newEnv(t)
	a := e.create("a <b> & \"c\"", "x", domain.SevCritical)
	b := e.create("b", "x", domain.SevLow)
	cv := 9.8
	title := "renamed ünïcode ✓"
	if _, err := e.s.UpdateFinding(ctx, analyst, a.ID, store.Patch{CVSS: &cv, Title: &title}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.ChangeStatus(ctx, analyst, a.ID, domain.StTriaged, "note with \n newline"); err != nil {
		t.Fatal(err)
	}
	due := t0.Add(36 * time.Hour)
	if _, err := e.s.Assign(ctx, pm, b.ID, dev.Sub, dev.Name, &due); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.UpdateFinding(ctx, analyst, a.ID, store.Patch{ClearCVSS: true}); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Link(ctx, analyst, a.ID, b.ID, store.LinkRootCause); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.AddAttachment(ctx, analyst, a.ID, "x.txt", "text/plain", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := e.s.SetSLA(ctx, pm, domain.SLA{domain.SevCritical: 5, domain.SevHigh: 20, domain.SevMedium: 60, domain.SevLow: 120}); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Unlink(ctx, analyst, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	res, err := e.s.VerifyAudit(ctx)
	if err != nil || !res.OK {
		t.Fatalf("chain over mixed actions = %+v, %v", res, err)
	}
	if res.Checked != 12 {
		t.Fatalf("checked %d rows, want 12", res.Checked)
	}
}

func TestCreateDuplicateOpenFingerprintConflict(t *testing.T) {
	e := newEnv(t)
	nf := store.NewFinding{Title: "CVE-2026-1 in p 1", Asset: "x", Severity: domain.SevLow, Source: "trivy", Fingerprint: "fp-1", DiscoveredAt: t0}
	if _, err := e.s.CreateFinding(ctx, analyst, nf); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.CreateFinding(ctx, analyst, nf); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("second open finding with the same fingerprint: %v", err)
	}
	if res, _ := e.s.VerifyAudit(ctx); !res.OK || res.Checked != 1 {
		t.Fatalf("failed create left audit rows: %+v", res)
	}
}
