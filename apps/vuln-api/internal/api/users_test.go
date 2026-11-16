package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/auth"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

type fakeUsers struct {
	people map[string]domain.Person
	calls  []string
}

func (f *fakeUsers) ListUsers(context.Context) ([]domain.Person, error) {
	var out []domain.Person
	for _, p := range f.people {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeUsers) SetRole(_ context.Context, sub string, r domain.Role) error {
	p := f.people[sub]
	p.Role = r
	f.people[sub] = p
	f.calls = append(f.calls, sub+"="+string(r))
	return nil
}

func withUsers(hs *harness) *fakeUsers {
	fu := &fakeUsers{people: map[string]domain.Person{
		uDev.Sub: {Sub: uDev.Sub, Name: uDev.Name, Role: domain.RoleDeveloper},
		uPM.Sub:  {Sub: uPM.Sub, Name: uPM.Name, Role: domain.RolePM},
	}}
	hs.users = fu
	hs.rebuild()
	return fu
}

func TestOnlyPMManagesRoles(t *testing.T) {
	hs := newHarness(t)
	fu := withUsers(hs)
	for _, u := range []auth.User{uAnalyst, uDev, uAuditor} {
		if w := hs.do(u, "GET", "/api/users", nil); w.Code != 403 {
			t.Errorf("%s list users: %d", u.Role, w.Code)
		}
		if w := hs.do(u, "PUT", "/api/users/"+uDev.Sub+"/role", map[string]string{"role": "pm"}); w.Code != 403 {
			t.Errorf("%s set role: %d", u.Role, w.Code)
		}
	}
	if len(fu.calls) != 0 {
		t.Fatalf("role changed by a non-PM: %v", fu.calls)
	}
	if list := decode[[]domain.Person](t, hs.do(uPM, "GET", "/api/users", nil)); len(list) != 2 {
		t.Fatalf("pm list: %+v", list)
	}
}

func TestPMCannotChangeOwnRole(t *testing.T) {
	hs := newHarness(t)
	fu := withUsers(hs)
	if w := hs.do(uPM, "PUT", "/api/users/"+uPM.Sub+"/role", map[string]string{"role": "auditor"}); w.Code != 400 {
		t.Fatalf("own role change: %d", w.Code)
	}
	if len(fu.calls) != 0 {
		t.Fatal("own role changed")
	}
}

func TestUnknownRole400(t *testing.T) {
	hs := newHarness(t)
	withUsers(hs)
	for _, role := range []string{"admin", "", "PM"} {
		if w := hs.do(uPM, "PUT", "/api/users/"+uDev.Sub+"/role", map[string]string{"role": role}); w.Code != 400 {
			t.Errorf("role %q: %d", role, w.Code)
		}
	}
	if w := hs.do(uPM, "PUT", "/api/users/nobody/role", map[string]string{"role": "analyst"}); w.Code != 404 {
		t.Errorf("unknown user: %d", w.Code)
	}
}

func TestSetRoleRevokesSessions(t *testing.T) {
	hs := newHarness(t)
	fu := withUsers(hs)
	if w := hs.do(uDev, "GET", "/api/me", nil); w.Code != 200 {
		t.Fatalf("dev session before: %d", w.Code)
	}
	w := hs.do(uPM, "PUT", "/api/users/"+uDev.Sub+"/role", map[string]string{"role": "analyst"})
	if w.Code != 200 || len(fu.calls) != 1 || fu.calls[0] != uDev.Sub+"=analyst" {
		t.Fatalf("set role: %d %s %v", w.Code, w.Body, fu.calls)
	}
	if w := hs.do(uDev, "GET", "/api/me", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("old session still works after a role change: %d", w.Code)
	}
	entries, _ := hs.st.Audit(context.Background(), store.AuditFilter{})
	last := entries[len(entries)-1]
	if last.Action != "user.role" || last.Details["target"] != uDev.Sub || last.Details["old"] != "developer" || last.Details["new"] != "analyst" {
		t.Fatalf("audit: %+v", last)
	}
}
