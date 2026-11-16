package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/auth"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/config"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/crypt/crypttest"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/db/testdb"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/ratelimit"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/store"
)

var jsonMarshal = json.Marshal

const base = "https://vulns.test"

var (
	uAnalyst = auth.User{Sub: "an-1", Name: "Ana Analyst", Role: domain.RoleAnalyst}
	uDev     = auth.User{Sub: "dev-1", Name: "Dee Dev", Role: domain.RoleDeveloper}
	uDev2    = auth.User{Sub: "dev-2", Name: "Dan Dev", Role: domain.RoleDeveloper}
	uPM      = auth.User{Sub: "pm-1", Name: "Pat PM", Role: domain.RolePM}
	uAuditor = auth.User{Sub: "au-1", Name: "Aud Auditor", Role: domain.RoleAuditor}
)

type fakeDir struct{ users []Assignee }

func (f *fakeDir) ListAssignees(context.Context) ([]Assignee, error) { return f.users, nil }

type harness struct {
	t       *testing.T
	h       http.Handler
	st      *store.Store
	now     time.Time
	cookies map[string]string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	hs := &harness{t: t, now: time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC), cookies: map[string]string{}}
	hs.st = store.New(testdb.New(t), crypttest.New(), func() time.Time { return hs.now })
	dir := &fakeDir{users: []Assignee{
		{Sub: uAnalyst.Sub, Name: uAnalyst.Name, Role: domain.RoleAnalyst},
		{Sub: uDev.Sub, Name: uDev.Name, Role: domain.RoleDeveloper},
		{Sub: uDev2.Sub, Name: uDev2.Name, Role: domain.RoleDeveloper},
		{Sub: uPM.Sub, Name: uPM.Name, Role: domain.RolePM},
	}}
	cfg := config.Config{PublicBaseURL: base, TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("10.42.0.0/16")}}
	hs.h = New(Deps{Store: hs.st, Sessions: hs.st.Sessions(), Dir: dir, Cfg: cfg,
		Limiter: ratelimit.New(5, func() time.Time { return hs.now }), Now: func() time.Time { return hs.now }})
	for _, u := range []auth.User{uAnalyst, uDev, uDev2, uPM, uAuditor} {
		raw, err := hs.st.Sessions().Create(context.Background(), u, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		hs.cookies[u.Sub] = raw
	}
	return hs
}

// do sends a same-origin request as u (zero User = anonymous).
func (hs *harness) do(u auth.User, method, path string, body any) *httptest.ResponseRecorder {
	hs.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = bytes.NewBufferString(b)
	default:
		j, _ := json.Marshal(b)
		rd = bytes.NewReader(j)
	}
	r := httptest.NewRequest(method, path, rd)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", base)
	if u.Sub != "" {
		r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: hs.cookies[u.Sub]})
	}
	w := httptest.NewRecorder()
	hs.h.ServeHTTP(w, r)
	return w
}

func (hs *harness) upload(u auth.User, path, filename string, data []byte) *httptest.ResponseRecorder {
	hs.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write(data)
	_ = mw.Close()
	r := httptest.NewRequest("POST", path, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Origin", base)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: hs.cookies[u.Sub]})
	w := httptest.NewRecorder()
	hs.h.ServeHTTP(w, r)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return v
}

func (hs *harness) createFinding(title string) store.Finding {
	hs.t.Helper()
	w := hs.do(uAnalyst, "POST", "/api/findings", map[string]any{
		"title": title, "description": "desc", "asset": "quote-api", "severity": "High",
	})
	if w.Code != http.StatusCreated {
		hs.t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	return decode[store.Finding](hs.t, w)
}

func (hs *harness) auditRows() int {
	hs.t.Helper()
	var n int
	_ = hs.st.Pool().QueryRow(context.Background(), `SELECT count(*) FROM audit_log`).Scan(&n)
	return n
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
