package api

import (
	"log/slog"
	"net/http"
	"net/mail"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/ratelimit"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

func init() {
	mounts = append(mounts, func(s *server, root, _ *http.ServeMux) {
		// Outside the /api/ session chain: anyone may submit, nobody may read.
		root.HandleFunc("POST /api/public/reports", s.publicReport)
	})
}

const maxPublicBody = 32 << 10

type publicReq struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Asset        string `json:"asset"`
	ContactEmail string `json:"contactEmail"`
	Website      string `json:"website"` // honeypot: hidden in the form, bots fill it in
}

var externalActor = store.Actor{Sub: "external", Name: "External reporter", Role: domain.Role("external")}

const ackMessage = "Thank you. Your report has been received and will be triaged."

// publicReport is AUTH-03 / FIND-02: an unauthenticated vulnerability
// report, acknowledged with a reference the reporter can quote.
func (s *server) publicReport(w http.ResponseWriter, r *http.Request) {
	if s.Limiter != nil && !s.Limiter.Allow(ratelimit.ClientIP(r, s.Cfg.TrustedProxyCIDRs)) {
		w.Header().Set("Retry-After", "3600")
		writeErr(w, http.StatusTooManyRequests, "too many reports from your network, try again later")
		return
	}
	var req publicReq
	if !decodeJSONLimit(w, r, &req, maxPublicBody) {
		return
	}
	if req.Website != "" {
		slog.Info("public report honeypot triggered")
		writeJSON(w, http.StatusCreated, map[string]string{"reference": store.NewRef(), "message": ackMessage})
		return
	}
	for _, err := range []error{checkTitle(req.Title), checkDescription(req.Description), checkAsset(req.Asset)} {
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	addr, err := mail.ParseAddress(req.ContactEmail)
	if err != nil || addr.Address != req.ContactEmail || addr.Name != "" || len(req.ContactEmail) > 254 {
		writeErr(w, http.StatusBadRequest, "contactEmail must be a plain e-mail address")
		return
	}
	f, err := s.Store.CreateFinding(r.Context(), externalActor, store.NewFinding{
		Title: req.Title, Description: req.Description, Asset: req.Asset, Severity: domain.SevLow,
		Source: "external", ReporterEmail: req.ContactEmail, DiscoveredAt: s.Now(),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"reference": f.Ref, "message": ackMessage})
}
