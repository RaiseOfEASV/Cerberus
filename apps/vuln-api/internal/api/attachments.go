package api

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"unicode"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

// MaxAttachmentBytes - D13.
const MaxAttachmentBytes = 10 << 20

// allowedTypes are decided by sniffing the bytes, never by the client.
var allowedTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "application/pdf": true, "text/plain": true, "application/json": true,
}

func sniff(data []byte, name string) string {
	ct := http.DetectContentType(data)
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	// DetectContentType reports JSON as text/plain; trust .json only when
	// the bytes really are plain text.
	if ct == "text/plain" && strings.EqualFold(path.Ext(name), ".json") {
		return "application/json"
	}
	return ct
}

// cleanName keeps the base name, drops control characters and quotes, and
// caps the length - it ends up in a Content-Disposition header.
func cleanName(n string) string {
	n = path.Base(strings.ReplaceAll(n, `\`, "/"))
	n = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '"' {
			return '_'
		}
		return r
	}, n)
	if n == "." || n == "/" || n == "" {
		n = "attachment"
	}
	if len(n) > 200 {
		n = n[:200]
	}
	return n
}

func (s *server) addAttachment(w http.ResponseWriter, r *http.Request) {
	f, u, ok := s.load(w, r, domain.ActAttach)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxAttachmentBytes+1<<20)
	file, hdr, err := r.FormFile("file")
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "attachments are limited to 10 MiB")
			return
		}
		writeErr(w, http.StatusBadRequest, "expected a multipart upload with field \"file\"")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxAttachmentBytes+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read upload")
		return
	}
	if len(data) > MaxAttachmentBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "attachments are limited to 10 MiB")
		return
	}
	name := cleanName(hdr.Filename)
	ct := sniff(data, name)
	if !allowedTypes[ct] {
		writeErr(w, http.StatusBadRequest, "file type "+ct+" is not allowed (png, jpeg, pdf, plain text, json)")
		return
	}
	att, err := s.Store.AddAttachment(r.Context(), actor(u), f.ID, name, ct, data)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, att)
}

func (s *server) listAttachments(w http.ResponseWriter, r *http.Request) {
	f, _, ok := s.load(w, r, domain.ActView)
	if !ok {
		return
	}
	list, err := s.Store.Attachments(r.Context(), f.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) getAttachment(w http.ResponseWriter, r *http.Request) {
	f, _, ok := s.load(w, r, domain.ActView)
	if !ok {
		return
	}
	attID, ok := pathID(r, "att")
	if !ok {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	att, data, err := s.Store.Attachment(r.Context(), f.ID, attID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+att.Name+`"`)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = w.Write(data)
}
