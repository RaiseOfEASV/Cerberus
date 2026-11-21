package store_test

import (
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

var scanner = store.Actor{Sub: "system:trivy-scanner", Name: "Trivy nightly scan", Role: domain.Role("system")}

func vuln(asset, id string) store.NewFinding {
	return store.NewFinding{Title: id + " in libfoo", Asset: asset, Severity: domain.SevHigh, CVE: id,
		Source: "trivy", Fingerprint: asset + "|" + id, DiscoveredAt: t0}
}

func statusOf(t *testing.T, e *env, asset string) map[string]domain.Status {
	t.Helper()
	items, _, err := e.s.ListFindings(ctx, store.Filter{Asset: asset, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]domain.Status{}
	for _, f := range items {
		m[f.CVE] = f.Status
	}
	return m
}

func TestSyncScanCreatesSkipsAndCloses(t *testing.T) {
	e := newEnv(t)
	img := "ghcr.io/raiseofeasv/vuln-web"

	res, err := e.s.SyncScan(ctx, scanner, img, []store.NewFinding{vuln(img, "CVE-2026-1"), vuln(img, "CVE-2026-2")})
	if err != nil || res != (store.ScanResult{Created: 2}) {
		t.Fatalf("first scan: %+v %v", res, err)
	}

	// CVE-2026-1 is fixed by an upgrade, CVE-2026-3 is new.
	res, err = e.s.SyncScan(ctx, scanner, img, []store.NewFinding{vuln(img, "CVE-2026-2"), vuln(img, "CVE-2026-3")})
	if err != nil || res != (store.ScanResult{Created: 1, Skipped: 1, Closed: 1}) {
		t.Fatalf("second scan: %+v %v", res, err)
	}
	got := statusOf(t, e, img)
	want := map[string]domain.Status{"CVE-2026-1": domain.StRemediated, "CVE-2026-2": domain.StNew, "CVE-2026-3": domain.StNew}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, want %q", k, got[k], v)
		}
	}

	// A clean scan closes everything still open for that asset.
	res, err = e.s.SyncScan(ctx, scanner, img, nil)
	if err != nil || res.Closed != 2 {
		t.Fatalf("clean scan: %+v %v", res, err)
	}

	// The vulnerability comes back: a new finding, the old one stays closed.
	res, err = e.s.SyncScan(ctx, scanner, img, []store.NewFinding{vuln(img, "CVE-2026-1")})
	if err != nil || res.Created != 1 {
		t.Fatalf("regression: %+v %v", res, err)
	}
	if v, err := e.s.VerifyAudit(ctx); err != nil || !v.OK {
		t.Fatalf("audit chain after scans: %+v %v", v, err)
	}
}

func TestSyncScanLeavesOtherFindingsAlone(t *testing.T) {
	e := newEnv(t)
	img := "ghcr.io/raiseofeasv/vuln-api"
	other := "ghcr.io/raiseofeasv/vuln-web"
	if _, err := e.s.SyncScan(ctx, scanner, other, []store.NewFinding{vuln(other, "CVE-2026-9")}); err != nil {
		t.Fatal(err)
	}
	manual := e.create("Hand-written finding", img, domain.SevLow) // source internal, same asset
	accepted, err := e.s.SyncScan(ctx, scanner, img, []store.NewFinding{vuln(img, "CVE-2026-5")})
	if err != nil || accepted.Created != 1 {
		t.Fatal(accepted, err)
	}
	items, _, err := e.s.ListFindings(ctx, store.Filter{Asset: img, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var risk int64
	for _, f := range items {
		if f.CVE == "CVE-2026-5" {
			risk = f.ID
		}
	}
	if _, err := e.s.ChangeStatus(ctx, pm, risk, domain.StAccepted, "base image fix pending"); err != nil {
		t.Fatal(err)
	}

	res, err := e.s.SyncScan(ctx, scanner, img, nil)
	if err != nil || res.Closed != 0 {
		t.Fatalf("clean scan closed %d (%v); want 0: the manual and Accepted Risk findings stay", res.Closed, err)
	}
	if f, _ := e.s.GetFinding(ctx, manual.ID); f.Status != domain.StNew {
		t.Errorf("manual finding on the same asset: %q", f.Status)
	}
	if got := statusOf(t, e, other); got["CVE-2026-9"] != domain.StNew {
		t.Errorf("other asset's finding: %q", got["CVE-2026-9"])
	}
}
