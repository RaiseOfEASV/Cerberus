package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

func init() {
	mounts = append(mounts, func(s *server, _, api *http.ServeMux) {
		api.HandleFunc("GET /api/audit", s.audit)
		api.HandleFunc("GET /api/audit/verify", s.verifyAudit)
		api.HandleFunc("GET /api/findings/{id}/history", s.history)
	})
}

// audit is AUDIT-02: PMs and auditors read the trail, oldest first.
func (s *server) audit(w http.ResponseWriter, r *http.Request) {
	if !can(w, user(r), domain.ActViewAudit) {
		return
	}
	q := r.URL.Query()
	f := store.AuditFilter{ActorSub: q.Get("actor"), Limit: 100}
	if v := q.Get("finding"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "finding must be a finding id")
			return
		}
		f.FindingID = &id
	}
	for name, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if v := q.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeErr(w, http.StatusBadRequest, name+" must be an RFC 3339 time")
				return
			}
			*dst = &t
		}
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			writeErr(w, http.StatusBadRequest, "limit must be 1-1000")
			return
		}
		f.Limit = n
	}
	entries, err := s.Store.Audit(r.Context(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if entries == nil {
		entries = []store.AuditEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// verifyAudit is AUDIT-03: recompute the hash chain.
func (s *server) verifyAudit(w http.ResponseWriter, r *http.Request) {
	if !can(w, user(r), domain.ActViewAudit) {
		return
	}
	res, err := s.Store.VerifyAudit(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// history only checks that the finding exists - it never decrypts it, so
// the trail stays readable exactly when a finding's data has been damaged.
func (s *server) history(w http.ResponseWriter, r *http.Request) {
	if !can(w, user(r), domain.ActViewAudit) {
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if exists, err := s.Store.FindingExists(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	} else if !exists {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	entries, err := s.Store.Audit(r.Context(), store.AuditFilter{FindingID: &id, Limit: 1000})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if entries == nil {
		entries = []store.AuditEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}
