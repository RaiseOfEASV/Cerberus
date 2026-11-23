package api

import (
	"fmt"
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/auth"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

var uGuest = auth.User{Sub: "gu-1", Name: "Pen Tester", Role: domain.RoleGuest}

// The pentest account: a real session, but every data endpoint is 403.
func TestGuestSeesNothing(t *testing.T) {
	hs := newHarness(t)
	hs.login(uGuest)
	f := hs.createFinding("Secret finding")
	id := fmt.Sprint(f.ID)
	for _, req := range []struct{ method, path string }{
		{"GET", "/api/findings"},
		{"GET", "/api/findings/" + id},
		{"GET", "/api/findings/" + id + "/links"},
		{"GET", "/api/findings/" + id + "/attachments"},
		{"GET", "/api/findings/" + id + "/history"},
		{"GET", "/api/dashboard"},
		{"GET", "/api/dashboard/trend"},
		{"GET", "/api/sla"},
		{"GET", "/api/assignees"},
		{"GET", "/api/audit"},
		{"GET", "/api/audit/verify"},
		{"GET", "/api/export/csv"},
		{"GET", "/api/reports"},
		{"GET", "/api/reports/public-key"},
		{"GET", "/api/users"},
		{"POST", "/api/findings"},
	} {
		if w := hs.do(uGuest, req.method, req.path, nil); w.Code != 403 {
			t.Errorf("guest %s %s: %d %s", req.method, req.path, w.Code, w.Body)
		}
	}
	w := hs.do(uGuest, "GET", "/api/me", nil)
	if w.Code != 200 {
		t.Fatalf("guest /api/me: %d", w.Code)
	}
	if me := decode[auth.User](t, w); me.Role != domain.RoleGuest {
		t.Fatalf("me: %+v", me)
	}
}

// /api/assignees lists staff names and roles; it had no permission check.
func TestAssigneesNeedView(t *testing.T) {
	hs := newHarness(t)
	hs.login(uGuest)
	if w := hs.do(uGuest, "GET", "/api/assignees", nil); w.Code != 403 {
		t.Fatalf("guest assignees: %d", w.Code)
	}
	for _, u := range []auth.User{uAnalyst, uDev, uPM, uAuditor} {
		if w := hs.do(u, "GET", "/api/assignees", nil); w.Code != 200 {
			t.Errorf("%s assignees: %d", u.Role, w.Code)
		}
	}
}
