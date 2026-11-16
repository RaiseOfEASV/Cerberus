package api

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")

func TestAttachmentUploadAndList(t *testing.T) {
	hs := newHarness(t)
	f := hs.createFinding("Evidence")
	w := hs.upload(uAnalyst, fmt.Sprintf("/api/findings/%d/attachments", f.ID), "../../etc/shot.png", pngHeader)
	if w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	att := decode[store.Attachment](t, w)
	if att.Name != "shot.png" || att.ContentType != "image/png" || att.Size != len(pngHeader) {
		t.Fatalf("attachment meta: %+v", att)
	}
	list := decode[[]store.Attachment](t, hs.do(uAuditor, "GET", fmt.Sprintf("/api/findings/%d/attachments", f.ID), nil))
	if len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}
}

func TestAttachmentTypeSniffed(t *testing.T) {
	hs := newHarness(t)
	f := hs.createFinding("Sniff")
	w := hs.upload(uAnalyst, fmt.Sprintf("/api/findings/%d/attachments", f.ID), "innocent.png", []byte("<html><script>alert(1)</script></html>"))
	if w.Code != 400 {
		t.Fatalf("HTML disguised as png: %d %s", w.Code, w.Body)
	}
	w = hs.upload(uAnalyst, fmt.Sprintf("/api/findings/%d/attachments", f.ID), "scan.json", []byte(`{"Results": []}`))
	if w.Code != 201 || decode[store.Attachment](t, w).ContentType != "application/json" {
		t.Fatalf("json evidence: %d %s", w.Code, w.Body)
	}
}

func TestAttachmentTooLarge413(t *testing.T) {
	hs := newHarness(t)
	f := hs.createFinding("Big")
	big := bytes.Repeat([]byte("a"), MaxAttachmentBytes+1)
	if w := hs.upload(uAnalyst, fmt.Sprintf("/api/findings/%d/attachments", f.ID), "big.txt", big); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large: %d", w.Code)
	}
}

func TestAttachmentDownloadHeaders(t *testing.T) {
	hs := newHarness(t)
	f := hs.createFinding("Download")
	att := decode[store.Attachment](t, hs.upload(uAnalyst, fmt.Sprintf("/api/findings/%d/attachments", f.ID), "evidence \"quoted\".txt", []byte("plain text evidence")))
	w := hs.do(uDev, "GET", fmt.Sprintf("/api/findings/%d/attachments/%d", f.ID, att.ID), nil)
	if w.Code != 200 || w.Body.String() != "plain text evidence" {
		t.Fatalf("download: %d %q", w.Code, w.Body)
	}
	h := w.Header()
	if h.Get("Content-Type") != "text/plain" || h.Get("X-Content-Type-Options") != "nosniff" ||
		h.Get("Content-Disposition") != `attachment; filename="evidence _quoted_.txt"` {
		t.Fatalf("download headers: %v", h)
	}
	if w := hs.do(uDev, "GET", fmt.Sprintf("/api/findings/%d/attachments/%d", f.ID+1000, att.ID), nil); w.Code != 404 {
		t.Fatalf("attachment through wrong finding: %d", w.Code)
	}
}
