package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

func init() {
	mounts = append(mounts, func(s *server, _, api *http.ServeMux) {
		api.HandleFunc("GET /api/dashboard", s.dashboard)
		api.HandleFunc("GET /api/dashboard/trend", s.trend)
		api.HandleFunc("GET /api/sla", s.getSLA)
		api.HandleFunc("PUT /api/sla", s.putSLA)
		api.HandleFunc("GET /api/reports", s.report)
		api.HandleFunc("GET /api/reports/public-key", s.publicKey)
	})
}

func (s *server) dashboard(w http.ResponseWriter, r *http.Request) {
	if !can(w, user(r), domain.ActView) {
		return
	}
	d, err := s.Store.Dashboard(r.Context(), s.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *server) trend(w http.ResponseWriter, r *http.Request) {
	if !can(w, user(r), domain.ActView) {
		return
	}
	weeks := 12
	if v := r.URL.Query().Get("weeks"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 52 {
			writeErr(w, http.StatusBadRequest, "weeks must be 1-52")
			return
		}
		weeks = n
	}
	t, err := s.Store.Trend(r.Context(), weeks, s.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *server) getSLA(w http.ResponseWriter, r *http.Request) {
	if !can(w, user(r), domain.ActView) {
		return
	}
	sla, err := s.Store.SLA(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sla)
}

func (s *server) putSLA(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if !can(w, u, domain.ActSetSLA) {
		return
	}
	var in map[string]int
	if !decodeJSON(w, r, &in) {
		return
	}
	sla := domain.SLA{}
	for k, v := range in {
		sev, err := domain.ParseSeverity(k)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "unknown severity "+strconv.Quote(k))
			return
		}
		sla[sev] = v
	}
	if err := s.Store.SetSLA(r.Context(), actor(u), sla); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sla)
}

// maxReportRange keeps exports bounded.
const maxReportRange = 2 * 366 * 24 * time.Hour

// report is FR-RPT-02/03: a JSON report signed with Vault Transit (Ed25519).
// The report travels as an exact string so a verifier checks the very
// bytes that were signed.
func (s *server) report(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if !can(w, u, domain.ActExport) {
		return
	}
	q := r.URL.Query()
	from, err1 := time.Parse("2006-01-02", q.Get("from"))
	to, err2 := time.Parse("2006-01-02", q.Get("to"))
	if err1 != nil || err2 != nil || !to.After(from) || to.Sub(from) > maxReportRange {
		writeErr(w, http.StatusBadRequest, "from and to must be dates (YYYY-MM-DD), to after from, at most two years apart")
		return
	}
	asset := q.Get("asset")
	if asset != "" && !domain.ValidAsset(asset) {
		writeErr(w, http.StatusBadRequest, "invalid asset")
		return
	}
	rep, err := s.Store.Report(r.Context(), from, to, asset)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	body, err := json.Marshal(rep)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sig, ver, err := s.Transit.Sign(r.Context(), body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.RecordEvent(r.Context(), actor(u), "report.export", map[string]any{
		"from": q.Get("from"), "to": q.Get("to"), "asset": asset, "findings": rep.Total, "keyVersion": ver,
	}); err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="report-`+q.Get("from")+`-`+q.Get("to")+`.json"`)
	writeJSON(w, http.StatusOK, map[string]any{"reportJson": string(body), "signature": sig, "keyVersion": ver})
}

func (s *server) publicKey(w http.ResponseWriter, r *http.Request) {
	if !can(w, user(r), domain.ActView) {
		return
	}
	ver, err := strconv.Atoi(r.URL.Query().Get("version"))
	if err != nil || ver < 1 {
		writeErr(w, http.StatusBadRequest, "version must be a positive integer")
		return
	}
	pub, err := s.Transit.PublicKey(r.Context(), ver)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no such key version")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": ver, "algorithm": "ed25519", "publicKey": base64.StdEncoding.EncodeToString(pub)})
}
