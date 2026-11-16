package api

import (
	"net/http"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

func init() {
	mounts = append(mounts, func(s *server, _, api *http.ServeMux) {
		api.HandleFunc("GET /api/users", s.listUsers)
		api.HandleFunc("PUT /api/users/{sub}/role", s.setRole)
	})
}

func (s *server) listUsers(w http.ResponseWriter, r *http.Request) {
	if !can(w, user(r), domain.ActManageRoles) {
		return
	}
	list, err := s.Users.ListUsers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if list == nil {
		list = []domain.Person{}
	}
	writeJSON(w, http.StatusOK, list)
}

// setRole is AUTH-04. The target's sessions end immediately, so the new
// role (or the loss of one) applies on their next request.
func (s *server) setRole(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if !can(w, u, domain.ActManageRoles) {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	role, err := domain.ParseRole(req.Role)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "role must be analyst, developer, pm or auditor")
		return
	}
	sub := r.PathValue("sub")
	if sub == u.Sub {
		writeErr(w, http.StatusBadRequest, "you cannot change your own role")
		return
	}
	users, err := s.Users.ListUsers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var old *domain.Person
	for i := range users {
		if users[i].Sub == sub {
			old = &users[i]
		}
	}
	if old == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if err := s.Users.SetRole(r.Context(), sub, role); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Sessions.DeleteBySub(r.Context(), sub); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.RecordEvent(r.Context(), actor(u), "user.role", map[string]any{
		"target": sub, "name": old.Name, "old": old.Role, "new": role,
	}); err != nil {
		s.fail(w, r, err)
		return
	}
	old.Role = role
	writeJSON(w, http.StatusOK, old)
}
