package api

import (
	"net/http"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

func (s *server) listLinks(w http.ResponseWriter, r *http.Request) {
	f, _, ok := s.load(w, r, domain.ActView)
	if !ok {
		return
	}
	ls, err := s.Store.Links(r.Context(), f.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ls)
}

type linkReq struct {
	OtherRef string `json:"otherRef"`
	Kind     string `json:"kind"`
}

func (s *server) addLink(w http.ResponseWriter, r *http.Request) {
	f, u, ok := s.load(w, r, domain.ActLink)
	if !ok {
		return
	}
	var req linkReq
	if !decodeJSON(w, r, &req) {
		return
	}
	kind, err := store.ParseLinkKind(req.Kind)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "kind must be duplicate, related or same_root_cause")
		return
	}
	other, err := s.Store.FindingIDByRef(r.Context(), req.OtherRef)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.Store.Link(r.Context(), actor(u), f.ID, other, kind); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"findingId": other, "kind": kind})
}

func (s *server) removeLink(w http.ResponseWriter, r *http.Request) {
	f, u, ok := s.load(w, r, domain.ActLink)
	if !ok {
		return
	}
	other, ok := pathID(r, "other")
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if err := s.Store.Unlink(r.Context(), actor(u), f.ID, other); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
