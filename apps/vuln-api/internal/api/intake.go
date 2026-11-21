package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/intake"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

func init() {
	mounts = append(mounts, func(s *server, _, api *http.ServeMux) {
		api.HandleFunc("POST /api/import/trivy", s.importTrivy)
		api.HandleFunc("POST /api/import/csv", s.importCSV)
		api.HandleFunc("GET /api/export/csv", s.exportCSV)
	})
}

const (
	maxTrivyBytes    = 5 << 20
	maxTrivyFindings = 2000
	maxCSVBytes      = 1 << 20
	maxCSVRows       = 2000
	maxExportRows    = 10000
)

// readBody reads at most limit bytes, answering 413 above it.
func readBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("upload limited to %d MiB", limit>>20))
		} else {
			writeErr(w, http.StatusBadRequest, "could not read upload")
		}
		return nil, false
	}
	return b, true
}

func (s *server) importTrivy(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if !can(w, u, domain.ActImport) {
		return
	}
	body, ok := readBody(w, r, maxTrivyBytes)
	if !ok {
		return
	}
	nfs, err := intake.ParseTrivy(bytes.NewReader(body), maxTrivyFindings, r.URL.Query().Get("asset"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.runImport(w, r, nfs, "trivy")
}

func (s *server) importCSV(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if !can(w, u, domain.ActImport) {
		return
	}
	body, ok := readBody(w, r, maxCSVBytes)
	if !ok {
		return
	}
	nfs, err := intake.ReadCSV(bytes.NewReader(body), maxCSVRows)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.runImport(w, r, nfs, "csv")
}

// runImport validates every row like a manual create, then stores all or nothing.
func (s *server) runImport(w http.ResponseWriter, r *http.Request, nfs []store.NewFinding, source string) {
	for i := range nfs {
		nf := &nfs[i]
		if nf.DiscoveredAt.IsZero() {
			nf.DiscoveredAt = s.Now()
		}
		if nf.DiscoveredAt.After(s.Now().Add(time.Minute)) {
			writeErr(w, http.StatusBadRequest, (&intake.RowError{Row: i + 2, Err: errBad("discovered_at is in the future")}).Error())
			return
		}
		c := createReq{Title: nf.Title, Description: nf.Description, Asset: nf.Asset, Severity: string(nf.Severity), CVSS: nf.CVSS, CVE: nf.CVE}
		if _, err := validateNew(c); err != nil {
			if source == "csv" {
				writeErr(w, http.StatusBadRequest, (&intake.RowError{Row: i + 2, Err: err}).Error())
			} else {
				writeErr(w, http.StatusBadRequest, fmt.Sprintf("finding %d (%s): %v", i+1, nf.Title, err))
			}
			return
		}
	}
	res, err := s.Store.Import(r.Context(), actor(user(r)), nfs, source)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *server) exportCSV(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if !can(w, u, domain.ActExport) {
		return
	}
	f, err := parseFilter(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var all []store.Finding
	truncated := false
	f.Limit, f.Offset = 200, 0
	for {
		items, total, err := s.Store.ListFindings(r.Context(), f)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		all = append(all, items...)
		f.Offset += len(items)
		if len(items) == 0 || f.Offset >= total {
			break
		}
		if f.Offset >= maxExportRows {
			truncated = true
			break
		}
	}
	if err := s.Store.RecordEvent(r.Context(), actor(u), "export.csv", map[string]any{"rows": len(all), "truncated": truncated, "query": r.URL.RawQuery}); err != nil {
		s.fail(w, r, err)
		return
	}
	// Exports stop at maxExportRows; say so rather than silently cutting off.
	w.Header().Set("X-Export-Truncated", strconv.FormatBool(truncated))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="findings.csv"`)
	_ = intake.WriteCSV(w, all)
}
