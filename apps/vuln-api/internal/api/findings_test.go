package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/auth"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

func TestUnauthenticated401(t *testing.T) {
	hs := newHarness(t)
	for _, p := range []string{"/api/me", "/api/findings", "/api/findings/1", "/api/assignees"} {
		if w := hs.do(auth.User{}, "GET", p, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s anonymous: %d", p, w.Code)
		}
	}
}

func TestMe(t *testing.T) {
	hs := newHarness(t)
	w := hs.do(uDev, "GET", "/api/me", nil)
	me := decode[map[string]any](t, w)
	if w.Code != 200 || me["sub"] != uDev.Sub || me["role"] != "developer" {
		t.Fatalf("me = %d %v", w.Code, me)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers: %v", w.Header())
	}
}

func TestCreateRequiresAnalystOrPM(t *testing.T) {
	hs := newHarness(t)
	body := map[string]any{"title": "SQLi in login", "description": "d", "asset": "quote-api", "severity": "Critical", "cvss": 9.1, "cve": "CVE-2026-12345"}
	for u, want := range map[auth.User]int{uDev: 403, uAuditor: 403, uAnalyst: 201, uPM: 201} {
		if w := hs.do(u, "POST", "/api/findings", body); w.Code != want {
			t.Errorf("%s create: %d, want %d (%s)", u.Role, w.Code, want, w.Body)
		}
	}
}

func TestCreateValidation(t *testing.T) {
	hs := newHarness(t)
	ok := map[string]any{"title": "Valid title", "description": "d", "asset": "quote-api", "severity": "High"}
	bad := []map[string]any{
		{"title": "ab"},
		{"title": strings.Repeat("x", 201)},
		{"asset": ""},
		{"asset": "<script>"},
		{"severity": "Severe"},
		{"cvss": 10.5},
		{"cvss": 7.25},
		{"cve": "CVE-26-1"},
		{"description": strings.Repeat("x", 20001)},
	}
	for _, change := range bad {
		b := map[string]any{}
		for k, v := range ok {
			b[k] = v
		}
		for k, v := range change {
			b[k] = v
		}
		if w := hs.do(uAnalyst, "POST", "/api/findings", b); w.Code != 400 {
			t.Errorf("%v: %d, want 400", change, w.Code)
		}
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	hs := newHarness(t)
	w := hs.do(uAnalyst, "POST", "/api/findings", map[string]any{"title": "abc", "asset": "a", "severity": "Low", "status": "Remediated"})
	if w.Code != 400 {
		t.Fatalf("unknown field: %d", w.Code)
	}
	if w := hs.do(uAnalyst, "POST", "/api/findings", "{not json"); w.Code != 400 {
		t.Fatalf("bad json: %d", w.Code)
	}
}

func TestDeveloperEditOthersFinding403NoAudit(t *testing.T) {
	hs := newHarness(t)
	f := hs.createFinding("Unassigned thing")
	if w := hs.do(uAnalyst, "POST", fmt.Sprintf("/api/findings/%d/assignment", f.ID), map[string]any{"assigneeSub": uDev2.Sub}); w.Code != 200 {
		t.Fatalf("assign: %d %s", w.Code, w.Body)
	}
	before := hs.auditRows()
	id := fmt.Sprint(f.ID)
	for _, req := range []struct {
		method, path string
		body         any
	}{
		{"PATCH", "/api/findings/" + id, map[string]any{"title": "hijacked"}},
		{"POST", "/api/findings/" + id + "/status", map[string]any{"status": "In Progress"}},
		{"POST", "/api/findings/" + id + "/links", map[string]any{"otherRef": f.Ref, "kind": "related"}},
	} {
		if w := hs.do(uDev, req.method, req.path, req.body); w.Code != 403 {
			t.Errorf("%s %s by a non-assignee developer: %d", req.method, req.path, w.Code)
		}
	}
	if w := hs.upload(uDev, "/api/findings/"+id+"/attachments", "x.txt", []byte("hello")); w.Code != 403 {
		t.Errorf("attach by non-assignee developer: %d", w.Code)
	}
	if after := hs.auditRows(); after != before {
		t.Fatalf("audit rows changed %d -> %d on refused requests", before, after)
	}
	// The assignee may.
	if w := hs.do(uDev2, "PATCH", "/api/findings/"+id, map[string]any{"title": "Fixed title"}); w.Code != 200 {
		t.Fatalf("assignee edit: %d %s", w.Code, w.Body)
	}
}

func TestDeveloperAcceptRisk403(t *testing.T) {
	hs := newHarness(t)
	f := hs.createFinding("Accept me")
	id := fmt.Sprint(f.ID)
	hs.do(uAnalyst, "POST", "/api/findings/"+id+"/assignment", map[string]any{"assigneeSub": uDev.Sub})
	hs.do(uAnalyst, "POST", "/api/findings/"+id+"/status", map[string]any{"status": "Triaged"})
	if w := hs.do(uDev, "POST", "/api/findings/"+id+"/status", map[string]any{"status": "Accepted Risk"}); w.Code != 403 {
		t.Fatalf("developer accept risk: %d %s", w.Code, w.Body)
	}
	if w := hs.do(uDev, "POST", "/api/findings/"+id+"/status", map[string]any{"status": "In Progress"}); w.Code != 200 {
		t.Fatalf("developer start work: %d %s", w.Code, w.Body)
	}
	if w := hs.do(uDev, "POST", "/api/findings/"+id+"/status", map[string]any{"status": "Remediated", "note": "patched in v1.2"}); w.Code != 200 {
		t.Fatalf("developer remediate: %d %s", w.Code, w.Body)
	}
}

func TestIllegalTransition409(t *testing.T) {
	hs := newHarness(t)
	f := hs.createFinding("x x x")
	w := hs.do(uAnalyst, "POST", fmt.Sprintf("/api/findings/%d/status", f.ID), map[string]any{"status": "Remediated"})
	if w.Code != 409 {
		t.Fatalf("New -> Remediated: %d %s", w.Code, w.Body)
	}
	if w := hs.do(uAnalyst, "POST", fmt.Sprintf("/api/findings/%d/status", f.ID), map[string]any{"status": "Done"}); w.Code != 400 {
		t.Fatalf("unknown status: %d", w.Code)
	}
}

func TestAuditorReadOnly(t *testing.T) {
	hs := newHarness(t)
	f := hs.createFinding("Read only")
	id := fmt.Sprint(f.ID)
	for _, req := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/findings", map[string]any{"title": "abc", "asset": "a", "severity": "Low"}},
		{"PATCH", "/api/findings/" + id, map[string]any{"title": "changed"}},
		{"POST", "/api/findings/" + id + "/status", map[string]any{"status": "Triaged"}},
		{"POST", "/api/findings/" + id + "/assignment", map[string]any{"assigneeSub": uDev.Sub}},
		{"POST", "/api/findings/" + id + "/links", map[string]any{"otherRef": f.Ref, "kind": "related"}},
		{"DELETE", "/api/findings/" + id + "/links/1", nil},
	} {
		if w := hs.do(uAuditor, req.method, req.path, req.body); w.Code != 403 {
			t.Errorf("auditor %s %s: %d", req.method, req.path, w.Code)
		}
	}
	if w := hs.upload(uAuditor, "/api/findings/"+id+"/attachments", "x.txt", []byte("x")); w.Code != 403 {
		t.Errorf("auditor upload: %d", w.Code)
	}
	if w := hs.do(uAuditor, "GET", "/api/findings/"+id, nil); w.Code != 200 {
		t.Errorf("auditor read: %d", w.Code)
	}
}

func TestFilterQueryParsing(t *testing.T) {
	hs := newHarness(t)
	hs.createFinding("One finding")
	f2 := hs.createFinding("Two finding")
	hs.do(uAnalyst, "POST", fmt.Sprintf("/api/findings/%d/status", f2.ID), map[string]any{"status": "Triaged"})
	type list struct {
		Items []store.Finding `json:"items"`
		Total int             `json:"total"`
	}
	l := decode[list](t, hs.do(uDev, "GET", "/api/findings?status=New&status=Triaged&severity=High", nil))
	if l.Total != 2 {
		t.Fatalf("multi status: %+v", l)
	}
	l = decode[list](t, hs.do(uDev, "GET", "/api/findings?status=Triaged", nil))
	if l.Total != 1 || l.Items[0].ID != f2.ID {
		t.Fatalf("status=Triaged: %+v", l)
	}
	l = decode[list](t, hs.do(uDev, "GET", "/api/findings?status=In%20Progress", nil))
	if l.Total != 0 {
		t.Fatalf("status=In Progress: %+v", l)
	}
	for _, q := range []string{"status=Bogus", "severity=Severe", "limit=abc", "offset=-1", "overdue=maybe"} {
		if w := hs.do(uDev, "GET", "/api/findings?"+q, nil); w.Code != 400 {
			t.Errorf("%s: %d, want 400", q, w.Code)
		}
	}
}

func TestAssignValidatesAssigneeAndDueDate(t *testing.T) {
	hs := newHarness(t)
	f := hs.createFinding("Assign me")
	p := fmt.Sprintf("/api/findings/%d/assignment", f.ID)
	if w := hs.do(uAnalyst, "POST", p, map[string]any{"assigneeSub": "nobody"}); w.Code != 400 {
		t.Fatalf("unknown assignee: %d", w.Code)
	}
	if w := hs.do(uAnalyst, "POST", p, map[string]any{"assigneeSub": uDev.Sub, "dueDate": hs.now.Add(-time.Hour).Format(time.RFC3339)}); w.Code != 400 {
		t.Fatalf("due date in the past: %d", w.Code)
	}
	if w := hs.do(uDev, "POST", p, map[string]any{"assigneeSub": uDev.Sub}); w.Code != 403 {
		t.Fatalf("developer self-assign: %d", w.Code)
	}
	due := hs.now.Add(72 * time.Hour).Format(time.RFC3339)
	w := hs.do(uPM, "POST", p, map[string]any{"assigneeSub": uDev.Sub, "dueDate": due})
	got := decode[store.Finding](t, w)
	if w.Code != 200 || got.AssigneeSub != uDev.Sub || got.AssigneeName != uDev.Name || got.DueDate == nil {
		t.Fatalf("assign: %d %+v", w.Code, got)
	}
}

func TestPatchClearsCVSS(t *testing.T) {
	hs := newHarness(t)
	w := hs.do(uAnalyst, "POST", "/api/findings", map[string]any{"title": "With score", "asset": "a", "severity": "Low", "cvss": 3.1})
	f := decode[store.Finding](t, w)
	w = hs.do(uAnalyst, "PATCH", fmt.Sprintf("/api/findings/%d", f.ID), `{"cvss": null}`)
	got := decode[store.Finding](t, w)
	if w.Code != 200 || got.CVSS != nil {
		t.Fatalf("clear cvss: %d %+v", w.Code, got)
	}
}

func TestXSSStoredVerbatim(t *testing.T) {
	hs := newHarness(t)
	w := hs.do(uAnalyst, "POST", "/api/findings", map[string]any{"title": `<script>alert(1)</script>`, "description": `<img src=x onerror=alert(1)>`, "asset": "web", "severity": "Low"})
	if w.Code != 201 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("create: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	f := decode[store.Finding](t, w)
	got := decode[store.Finding](t, hs.do(uAnalyst, "GET", fmt.Sprintf("/api/findings/%d", f.ID), nil))
	if got.Title != `<script>alert(1)</script>` || got.Description != `<img src=x onerror=alert(1)>` {
		t.Fatalf("not stored verbatim: %+v", got)
	}
}

func TestNotFoundIs404(t *testing.T) {
	hs := newHarness(t)
	for _, p := range []string{"/api/findings/999", "/api/findings/abc", "/api/findings/999/links", "/api/findings/999/attachments"} {
		if w := hs.do(uAnalyst, "GET", p, nil); w.Code != 404 {
			t.Errorf("GET %s: %d", p, w.Code)
		}
	}
	if w := hs.do(uAnalyst, "PATCH", "/api/findings/999", map[string]any{"title": "nope"}); w.Code != 404 {
		t.Errorf("PATCH missing: %d", w.Code)
	}
}

func TestLinksAPI(t *testing.T) {
	hs := newHarness(t)
	a, b := hs.createFinding("Link A"), hs.createFinding("Link B")
	w := hs.do(uAnalyst, "POST", fmt.Sprintf("/api/findings/%d/links", a.ID), map[string]any{"otherRef": b.Ref, "kind": "duplicate"})
	if w.Code != 201 {
		t.Fatalf("link: %d %s", w.Code, w.Body)
	}
	ls := decode[[]store.Link](t, hs.do(uDev, "GET", fmt.Sprintf("/api/findings/%d/links", b.ID), nil))
	if len(ls) != 1 || ls[0].FindingID != a.ID || ls[0].Kind != store.LinkDuplicate {
		t.Fatalf("links of b: %+v", ls)
	}
	if w := hs.do(uAnalyst, "POST", fmt.Sprintf("/api/findings/%d/links", a.ID), map[string]any{"otherRef": "VM-NOPE", "kind": "related"}); w.Code != 404 {
		t.Fatalf("unknown ref: %d", w.Code)
	}
	if w := hs.do(uAnalyst, "POST", fmt.Sprintf("/api/findings/%d/links", a.ID), map[string]any{"otherRef": b.Ref, "kind": "cousin"}); w.Code != 400 {
		t.Fatalf("bad kind: %d", w.Code)
	}
	if w := hs.do(uAnalyst, "DELETE", fmt.Sprintf("/api/findings/%d/links/%d", a.ID, b.ID), nil); w.Code != 204 {
		t.Fatalf("unlink: %d", w.Code)
	}
}

func TestAssigneesList(t *testing.T) {
	hs := newHarness(t)
	w := hs.do(uDev, "GET", "/api/assignees", nil)
	list := decode[[]Assignee](t, w)
	if w.Code != 200 || len(list) != 4 {
		t.Fatalf("assignees: %d %+v", w.Code, list)
	}
}
