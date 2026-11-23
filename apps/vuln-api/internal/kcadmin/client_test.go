package kcadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

type call struct{ Method, Path, Body string }

// fakeKC models Keycloak's admin REST API for one realm.
type fakeKC struct {
	mu     sync.Mutex
	calls  []call
	tokens int
	users  []map[string]any
	maps   map[string]map[string]bool // userID -> role names
}

func newKC(t *testing.T) (*fakeKC, *Client) {
	t.Helper()
	f := &fakeKC{
		users: []map[string]any{
			{"id": "u-pm", "username": "pat", "firstName": "Pat", "lastName": "PM", "email": "pat@x"},
			{"id": "u-dev", "username": "dee", "firstName": "Dee", "lastName": "Dev"},
			{"id": "u-new", "username": "newbie"},
		},
		maps: map[string]map[string]bool{"u-pm": {"pm": true, "developer": true}, "u-dev": {"developer": true}, "u-new": {}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /realms/vulnman/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "vuln-api-admin" || r.Form.Get("client_secret") != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		f.tokens++
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "adm", "expires_in": 300})
	})
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer adm" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var b strings.Builder
			if r.Body != nil {
				var v any
				_ = json.NewDecoder(r.Body).Decode(&v)
				j, _ := json.Marshal(v)
				if v != nil {
					b.Write(j)
				}
			}
			f.mu.Lock()
			f.calls = append(f.calls, call{r.Method, r.URL.Path, b.String()})
			f.mu.Unlock()
			r.Body = http.NoBody
			h(w, r)
		}
	}
	mux.HandleFunc("GET /admin/realms/vulnman/users", authed(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.users)
	}))
	mux.HandleFunc("GET /admin/realms/vulnman/roles/{role}/users", authed(func(w http.ResponseWriter, r *http.Request) {
		var out []map[string]any
		for _, u := range f.users {
			if f.maps[u["id"].(string)][r.PathValue("role")] {
				out = append(out, u)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	mux.HandleFunc("GET /admin/realms/vulnman/roles/{role}", authed(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "id-" + r.PathValue("role"), "name": r.PathValue("role")})
	}))
	mux.HandleFunc("/admin/realms/vulnman/users/{id}/role-mappings/realm", authed(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, New(srv.URL, "vulnman", "vuln-api-admin", "s3cret", srv.Client())
}

func TestListUsersAndRoles(t *testing.T) {
	f, c := newKC(t)
	users, err := c.ListUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Sub < users[j].Sub })
	want := []domain.Person{
		{Sub: "u-dev", Username: "dee", Name: "Dee Dev", Role: domain.RoleDeveloper},
		{Sub: "u-new", Username: "newbie", Name: "newbie", Role: ""},
		{Sub: "u-pm", Username: "pat", Name: "Pat PM", Email: "pat@x", Role: domain.RolePM},
	}
	if len(users) != 3 {
		t.Fatalf("users: %+v", users)
	}
	for i := range want {
		if users[i] != want[i] {
			t.Errorf("user %d = %+v, want %+v", i, users[i], want[i])
		}
	}
	as, _ := c.ListAssignees(context.Background())
	if len(as) != 2 {
		t.Fatalf("assignees (users with a staff role that can own work): %+v", as)
	}
	if f.tokens != 1 {
		t.Fatalf("fetched %d admin tokens, want 1 (cached)", f.tokens)
	}
}

func TestSetRoleReplacesOthers(t *testing.T) {
	f, c := newKC(t)
	if err := c.SetRole(context.Background(), "u-dev", domain.RoleAnalyst); err != nil {
		t.Fatal(err)
	}
	var mutations []call
	for _, cl := range f.calls {
		if cl.Method != "GET" {
			mutations = append(mutations, cl)
		}
	}
	if len(mutations) != 2 {
		t.Fatalf("mutations: %+v", mutations)
	}
	del, add := mutations[0], mutations[1]
	if del.Method != "DELETE" || del.Path != "/admin/realms/vulnman/users/u-dev/role-mappings/realm" ||
		// guest too: making someone staff must drop the pentest role.
		del.Body != `[{"id":"id-pm","name":"pm"},{"id":"id-developer","name":"developer"},{"id":"id-auditor","name":"auditor"},{"id":"id-guest","name":"guest"}]` {
		t.Errorf("delete call: %+v", del)
	}
	if add.Method != "POST" || add.Path != del.Path || add.Body != `[{"id":"id-analyst","name":"analyst"}]` {
		t.Errorf("add call: %+v", add)
	}
}

func TestSetRoleRefusesNonAppRole(t *testing.T) {
	f, c := newKC(t)
	if err := c.SetRole(context.Background(), "u-dev", domain.Role("admin")); err == nil {
		t.Fatal("non-app role accepted")
	}
	if err := c.SetRole(context.Background(), "../../users", domain.RoleAnalyst); err == nil {
		t.Fatal("path-like user id accepted")
	}
	if len(f.calls) != 0 {
		t.Fatalf("made calls: %+v", f.calls)
	}
}
