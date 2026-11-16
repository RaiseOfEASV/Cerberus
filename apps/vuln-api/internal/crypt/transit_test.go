package crypt

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeVault speaks just enough of Vault's HTTP API, in its real JSON shapes.
type fakeVault struct {
	logins     atomic.Int32
	expireNext atomic.Bool // next authenticated call answers 403 once
	dek        []byte
	pub        ed25519.PublicKey
	priv       ed25519.PrivateKey
}

func (f *fakeVault) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Vault-Token") != "tok" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if f.expireNext.CompareAndSwap(true, false) {
				w.WriteHeader(http.StatusForbidden)
				write(w, map[string]any{"errors": []string{"permission denied"}})
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("POST /v1/auth/kubernetes/login", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["role"] != "vuln-api-dev" || in["jwt"] != "sa-jwt" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.logins.Add(1)
		write(w, map[string]any{"auth": map[string]any{"client_token": "tok", "lease_duration": 900}})
	})
	mux.HandleFunc("POST /v1/transit/datakey/plaintext/vulnman-dev-data", authed(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["bits"] != float64(256) {
			t.Errorf("datakey bits = %v", in["bits"])
		}
		write(w, map[string]any{"data": map[string]any{
			"plaintext":  base64.StdEncoding.EncodeToString(f.dek),
			"ciphertext": "vault:v1:wrapped",
		}})
	}))
	mux.HandleFunc("POST /v1/transit/decrypt/vulnman-dev-data", authed(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["ciphertext"] != "vault:v1:wrapped" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		write(w, map[string]any{"data": map[string]any{"plaintext": base64.StdEncoding.EncodeToString(f.dek)}})
	}))
	mux.HandleFunc("POST /v1/transit/sign/vulnman-dev-report", authed(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		data, _ := base64.StdEncoding.DecodeString(in["input"])
		sig := ed25519.Sign(f.priv, data)
		write(w, map[string]any{"data": map[string]any{
			"signature":   "vault:v1:" + base64.StdEncoding.EncodeToString(sig),
			"key_version": 1,
		}})
	}))
	mux.HandleFunc("GET /v1/transit/keys/vulnman-dev-report", authed(func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]any{"data": map[string]any{"keys": map[string]any{
			"1": map[string]any{"public_key": base64.StdEncoding.EncodeToString(f.pub)},
		}}})
	}))
	return mux
}

func newVaultForTest(t *testing.T) (*Vault, *fakeVault) {
	t.Helper()
	f := &fakeVault{dek: make([]byte, 32)}
	_, _ = rand.Read(f.dek)
	f.pub, f.priv, _ = ed25519.GenerateKey(rand.Reader)
	srv := httptest.NewTLSServer(f.handler(t))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.crt")
	_ = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600)
	tok := filepath.Join(dir, "token")
	_ = os.WriteFile(tok, []byte("sa-jwt\n"), 0o600)
	v, err := NewVault(srv.URL, ca, "vuln-api-dev", tok, "vulnman-dev-data", "vulnman-dev-report")
	if err != nil {
		t.Fatal(err)
	}
	return v, f
}

func TestVaultClientAgainstFakeServer(t *testing.T) {
	ctx := context.Background()
	v, f := newVaultForTest(t)
	pt, wrapped, err := v.DataKey(ctx)
	if err != nil || wrapped != "vault:v1:wrapped" || string(pt) != string(f.dek) {
		t.Fatalf("DataKey = %x %q %v", pt, wrapped, err)
	}
	un, err := v.Unwrap(ctx, wrapped)
	if err != nil || string(un) != string(f.dek) {
		t.Fatalf("Unwrap = %x %v", un, err)
	}
	sig, ver, err := v.Sign(ctx, []byte("report"))
	if err != nil || ver != 1 || !strings.HasPrefix(sig, "vault:v1:") {
		t.Fatalf("Sign = %q %d %v", sig, ver, err)
	}
	pub, err := v.PublicKey(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(pub, []byte("report"), sig); err != nil {
		t.Fatalf("signature from Vault did not verify: %v", err)
	}
	if n := f.logins.Load(); n != 1 {
		t.Fatalf("logged in %d times, want 1 (token must be cached)", n)
	}
}

func TestVaultReloginAfter403(t *testing.T) {
	ctx := context.Background()
	v, f := newVaultForTest(t)
	if _, _, err := v.DataKey(ctx); err != nil {
		t.Fatal(err)
	}
	f.expireNext.Store(true)
	if _, err := v.Unwrap(ctx, "vault:v1:wrapped"); err != nil {
		t.Fatalf("call after token expiry failed: %v", err)
	}
	if n := f.logins.Load(); n != 2 {
		t.Fatalf("logins = %d, want 2 (re-login after 403)", n)
	}
}

func TestVaultRejectsUntrustedTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	dir := t.TempDir()
	// Every httptest TLS server shares one built-in cert, so trust a freshly
	// generated, unrelated self-signed CA instead.
	ca := filepath.Join(dir, "ca.crt")
	_ = os.WriteFile(ca, unrelatedCA(t), 0o600)
	tok := filepath.Join(dir, "token")
	_ = os.WriteFile(tok, []byte("x"), 0o600)
	v, err := NewVault(srv.URL, ca, "r", tok, "d", "s")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := v.DataKey(context.Background()); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("want TLS verification error, got %v", err)
	}
}

func unrelatedCA(t *testing.T) []byte {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "unrelated"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
