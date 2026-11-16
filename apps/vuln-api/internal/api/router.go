// Package api is vuln-api's HTTP surface. Authorization is enforced here
// against domain's permission matrix, for every route, on the server; the
// web UI's hiding of controls is cosmetic.
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/auth"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/config"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/crypt"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/ratelimit"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

// Assignee is a user findings can be assigned to (analyst, developer, pm).
type Assignee = domain.Person

// Directory lists staff from the IdP. kcadmin implements it.
type Directory interface {
	ListAssignees(ctx context.Context) ([]Assignee, error)
}

// UserAdmin manages app roles in the IdP (AUTH-04). kcadmin implements it.
type UserAdmin interface {
	ListUsers(ctx context.Context) ([]domain.Person, error)
	SetRole(ctx context.Context, sub string, r domain.Role) error
}

type Deps struct {
	Store    *store.Store
	Sessions auth.SessionStore
	OIDC     *auth.OIDC // nil in tests: /auth routes not mounted
	Dir      Directory
	Users    UserAdmin
	Limiter  *ratelimit.Limiter // public report form (D12)
	Transit  crypt.Transit      // report signing (D15)
	Cfg      config.Config
	Now      func() time.Time
}

type server struct{ Deps }

// New builds the handler: public routes, /auth/*, and /api/* behind a
// session and a same-origin check.
func New(d Deps) http.Handler {
	if d.Now == nil {
		d.Now = time.Now
	}
	s := &server{d}
	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	if d.OIDC != nil {
		root.HandleFunc("GET /auth/login", d.OIDC.Login)
		root.HandleFunc("GET /auth/callback", d.OIDC.Callback)
		root.Handle("POST /auth/logout", auth.SameOrigin(d.Cfg.PublicBaseURL)(http.HandlerFunc(d.OIDC.Logout)))
	}

	api := http.NewServeMux()
	api.HandleFunc("GET /api/me", s.me)
	api.HandleFunc("GET /api/assignees", s.assignees)
	api.HandleFunc("GET /api/findings", s.listFindings)
	api.HandleFunc("POST /api/findings", s.createFinding)
	api.HandleFunc("GET /api/findings/{id}", s.getFinding)
	api.HandleFunc("PATCH /api/findings/{id}", s.patchFinding)
	api.HandleFunc("POST /api/findings/{id}/status", s.changeStatus)
	api.HandleFunc("POST /api/findings/{id}/assignment", s.assign)
	api.HandleFunc("GET /api/findings/{id}/links", s.listLinks)
	api.HandleFunc("POST /api/findings/{id}/links", s.addLink)
	api.HandleFunc("DELETE /api/findings/{id}/links/{other}", s.removeLink)
	api.HandleFunc("GET /api/findings/{id}/attachments", s.listAttachments)
	api.HandleFunc("POST /api/findings/{id}/attachments", s.addAttachment)
	api.HandleFunc("GET /api/findings/{id}/attachments/{att}", s.getAttachment)
	s.mountMore(root, api)
	root.Handle("/api/", auth.RequireUser(d.Sessions)(auth.SameOrigin(d.Cfg.PublicBaseURL)(api)))
	return securityHeaders(root)
}

// mountMore is where later route groups register (public intake, imports,
// dashboard, reports, audit, users).
func (s *server) mountMore(root, api *http.ServeMux) {
	for _, m := range mounts {
		m(s, root, api)
	}
}

var mounts []func(s *server, root, api *http.ServeMux)

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	writeJSON(w, http.StatusOK, u)
}

func (s *server) assignees(w http.ResponseWriter, r *http.Request) {
	list, err := s.Dir.ListAssignees(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if list == nil {
		list = []Assignee{}
	}
	writeJSON(w, http.StatusOK, list)
}
