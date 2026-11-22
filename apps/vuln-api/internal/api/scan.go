package api

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/intake"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

// The nightly in-cluster scanner (apps/vuln-scanner) posts one Trivy report
// per running image here. Not a user endpoint: it lives outside /api (the
// public IngressRoutes only forward /api and /auth), needs the shared
// SCANNER_TOKEN, and refuses anything that came through a proxy.
func init() {
	mounts = append(mounts, func(s *server, root, _ *http.ServeMux) {
		if s.Cfg.ScannerToken != "" {
			root.HandleFunc("POST /internal/scans/trivy", s.scanTrivy)
		}
	})
}

// The scanner's reports are bigger than a user's upload: whole third-party
// images (Wazuh, Keycloak...) with hundreds of findings. The scanner already
// strips each report to the fields ParseTrivy reads, and the endpoint is
// token-only, so it gets more room than /api/import/trivy.
const (
	maxScanBytes    = 16 << 20
	maxScanFindings = 5000
)

var scannerActor = store.Actor{Sub: "system:trivy-scanner", Name: "Trivy nightly scan", Role: domain.Role("system")}

func (s *server) scannerAuthorized(r *http.Request) bool {
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || got == "" {
		return false
	}
	// Compare digests so the comparison time doesn't depend on the length.
	a, b := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(s.Cfg.ScannerToken))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

func (s *server) scanTrivy(w http.ResponseWriter, r *http.Request) {
	// Traefik always sets X-Forwarded-For; the scanner calls the Service directly.
	if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Forwarded-Host") != "" {
		http.NotFound(w, r)
		return
	}
	if !s.scannerAuthorized(r) {
		writeErr(w, http.StatusUnauthorized, "scanner token required")
		return
	}
	asset := r.URL.Query().Get("asset")
	if !domain.ValidAsset(asset) {
		writeErr(w, http.StatusBadRequest, "asset is required")
		return
	}
	body, ok := readBody(w, r, maxScanBytes)
	if !ok {
		return
	}
	nfs, err := intake.ParseTrivy(bytes.NewReader(body), maxScanFindings, asset)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	for i := range nfs {
		nf := &nfs[i]
		nf.DiscoveredAt = s.Now()
		c := createReq{Title: nf.Title, Description: nf.Description, Asset: nf.Asset, Severity: string(nf.Severity), CVSS: nf.CVSS, CVE: nf.CVE}
		if _, err := validateNew(c); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("finding %d (%s): %v", i+1, nf.Title, err))
			return
		}
	}
	res, err := s.Store.SyncScan(r.Context(), scannerActor, asset, nfs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
