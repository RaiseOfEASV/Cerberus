package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/auth"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

func actor(u auth.User) store.Actor { return store.Actor{Sub: u.Sub, Name: u.Name, Role: u.Role} }

// ownerReq is the assignee the store must re-check under its row lock: the
// developer themself (D5), or nobody for roles that may act on any finding.
func ownerReq(u auth.User) string {
	if u.Role == domain.RoleDeveloper {
		return u.Sub
	}
	return ""
}

func user(r *http.Request) auth.User { u, _ := auth.UserFrom(r.Context()); return u }

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

// can answers 403 when the role may not perform a at all.
func can(w http.ResponseWriter, u auth.User, a domain.Action) bool {
	if !domain.Can(u.Role, a) {
		writeErr(w, http.StatusForbidden, "your role may not do this")
		return false
	}
	return true
}

// load fetches the finding in the path and checks a (including D5's
// developer ownership). It writes the error response on failure.
func (s *server) load(w http.ResponseWriter, r *http.Request, a domain.Action) (store.Finding, auth.User, bool) {
	u := user(r)
	if !can(w, u, a) {
		return store.Finding{}, u, false
	}
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return store.Finding{}, u, false
	}
	f, err := s.Store.GetFinding(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return store.Finding{}, u, false
	}
	if !domain.CanOnFinding(u.Role, a, u.Sub, f.AssigneeSub) {
		writeErr(w, http.StatusForbidden, "only the assignee may do this")
		return store.Finding{}, u, false
	}
	return f, u, true
}

func parseFilter(r *http.Request) (store.Filter, error) {
	q := r.URL.Query()
	var f store.Filter
	for _, v := range q["status"] {
		st, err := domain.ParseStatus(v)
		if err != nil {
			return f, err
		}
		f.Status = append(f.Status, st)
	}
	for _, v := range q["severity"] {
		sev, err := parseSeverity(v)
		if err != nil {
			return f, err
		}
		f.Severity = append(f.Severity, sev)
	}
	f.AssigneeSub, f.Asset, f.Q = q.Get("assignee"), q.Get("asset"), q.Get("q")
	if v := q.Get("overdue"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return f, err
		}
		f.Overdue = b
	}
	for name, dst := range map[string]*int{"limit": &f.Limit, "offset": &f.Offset} {
		if v := q.Get(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return f, errBad(name + " must be a non-negative integer")
			}
			*dst = n
		}
	}
	return f, nil
}

type errBad string

func (e errBad) Error() string { return string(e) }

func (s *server) listFindings(w http.ResponseWriter, r *http.Request) {
	if !can(w, user(r), domain.ActView) {
		return
	}
	f, err := parseFilter(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	items, total, err := s.Store.ListFindings(r.Context(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if items == nil {
		items = []store.Finding{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

type createReq struct {
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Asset        string     `json:"asset"`
	Severity     string     `json:"severity"`
	CVSS         *float64   `json:"cvss"`
	CVE          string     `json:"cve"`
	DiscoveredAt *time.Time `json:"discoveredAt"`
}

func validateNew(c createReq) (domain.Severity, error) {
	for _, err := range []error{checkTitle(c.Title), checkDescription(c.Description), checkAsset(c.Asset), checkCVE(c.CVE)} {
		if err != nil {
			return "", err
		}
	}
	if c.CVSS != nil {
		if err := checkCVSS(*c.CVSS); err != nil {
			return "", err
		}
	}
	return parseSeverity(c.Severity)
}

func (s *server) createFinding(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if !can(w, u, domain.ActCreate) {
		return
	}
	var c createReq
	if !decodeJSON(w, r, &c) {
		return
	}
	sev, err := validateNew(c)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	disc := s.Now()
	if c.DiscoveredAt != nil {
		if c.DiscoveredAt.After(s.Now().Add(time.Minute)) {
			writeErr(w, http.StatusBadRequest, "discoveredAt is in the future")
			return
		}
		disc = *c.DiscoveredAt
	}
	f, err := s.Store.CreateFinding(r.Context(), actor(u), store.NewFinding{
		Title: c.Title, Description: c.Description, Asset: c.Asset, Severity: sev, CVSS: c.CVSS, CVE: c.CVE,
		Source: "internal", DiscoveredAt: disc,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (s *server) getFinding(w http.ResponseWriter, r *http.Request) {
	f, _, ok := s.load(w, r, domain.ActView)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, f)
}

type patchReq struct {
	Title       *string         `json:"title"`
	Description *string         `json:"description"`
	Asset       *string         `json:"asset"`
	Severity    *string         `json:"severity"`
	CVSS        json.RawMessage `json:"cvss"` // absent = unchanged, null = clear
	CVE         *string         `json:"cve"`
}

func (s *server) patchFinding(w http.ResponseWriter, r *http.Request) {
	f, u, ok := s.load(w, r, domain.ActEdit)
	if !ok {
		return
	}
	var p patchReq
	if !decodeJSON(w, r, &p) {
		return
	}
	var sp store.Patch
	bad := func(err error) bool {
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return true
		}
		return false
	}
	if p.Title != nil && bad(checkTitle(*p.Title)) {
		return
	}
	if p.Description != nil && bad(checkDescription(*p.Description)) {
		return
	}
	if p.Asset != nil && bad(checkAsset(*p.Asset)) {
		return
	}
	if p.CVE != nil && bad(checkCVE(*p.CVE)) {
		return
	}
	sp.Title, sp.Description, sp.Asset, sp.CVE = p.Title, p.Description, p.Asset, p.CVE
	sp.RequireAssignee = ownerReq(u)
	if p.Severity != nil {
		sev, err := parseSeverity(*p.Severity)
		if bad(err) {
			return
		}
		sp.Severity = &sev
	}
	if len(p.CVSS) > 0 {
		if string(p.CVSS) == "null" {
			sp.ClearCVSS = true
		} else {
			var v float64
			if err := json.Unmarshal(p.CVSS, &v); err != nil {
				writeErr(w, http.StatusBadRequest, "cvss must be a number or null")
				return
			}
			if bad(checkCVSS(v)) {
				return
			}
			sp.CVSS = &v
		}
	}
	out, err := s.Store.UpdateFinding(r.Context(), actor(u), f.ID, sp)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type statusReq struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func (s *server) changeStatus(w http.ResponseWriter, r *http.Request) {
	f, u, ok := s.load(w, r, domain.ActChangeStatus)
	if !ok {
		return
	}
	var req statusReq
	if !decodeJSON(w, r, &req) {
		return
	}
	to, err := domain.ParseStatus(req.Status)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "status must be one of New, Triaged, In Progress, Remediated, Accepted Risk")
		return
	}
	if len(req.Note) > 2000 {
		writeErr(w, http.StatusBadRequest, "note must be at most 2000 characters")
		return
	}
	// Legal for someone, but not for this role (e.g. a developer accepting
	// risk) is a permission problem, not a state-machine one.
	if !domain.CanTransition(u.Role, f.Status, to) && domain.CanTransition(domain.RolePM, f.Status, to) {
		writeErr(w, http.StatusForbidden, "your role may not move a finding to "+string(to))
		return
	}
	out, err := s.Store.ChangeStatus(r.Context(), actor(u), f.ID, to, req.Note, ownerReq(u))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type assignReq struct {
	AssigneeSub string     `json:"assigneeSub"`
	DueDate     *time.Time `json:"dueDate"`
}

func (s *server) assign(w http.ResponseWriter, r *http.Request) {
	f, u, ok := s.load(w, r, domain.ActAssign)
	if !ok {
		return
	}
	var req assignReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.DueDate != nil && req.DueDate.Before(s.Now()) {
		writeErr(w, http.StatusBadRequest, "dueDate must not be in the past")
		return
	}
	name := ""
	if req.AssigneeSub != "" {
		list, err := s.Dir.ListAssignees(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, a := range list {
			if a.Sub == req.AssigneeSub {
				name = a.Name
			}
		}
		if name == "" {
			writeErr(w, http.StatusBadRequest, "assignee must be an analyst, developer or PM")
			return
		}
	}
	out, err := s.Store.Assign(r.Context(), actor(u), f.ID, req.AssigneeSub, name, req.DueDate)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
