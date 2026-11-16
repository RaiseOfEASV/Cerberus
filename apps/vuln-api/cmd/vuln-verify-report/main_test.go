package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/crypt/crypttest"
)

func files(t *testing.T, reportJSON string, tamper bool) (string, string) {
	t.Helper()
	tr := crypttest.New()
	sig, ver, _ := tr.Sign(context.Background(), []byte(reportJSON))
	pub, _ := tr.PublicKey(context.Background(), ver)
	if tamper {
		reportJSON = strings.Replace(reportJSON, "3", "4", 1)
	}
	dir := t.TempDir()
	rep := filepath.Join(dir, "report.json")
	b, _ := json.Marshal(map[string]any{"reportJson": reportJSON, "signature": sig, "keyVersion": ver})
	_ = os.WriteFile(rep, b, 0o600)
	key := filepath.Join(dir, "key.b64")
	_ = os.WriteFile(key, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o600)
	return rep, key
}

func TestVerifyOK(t *testing.T) {
	rep, key := files(t, `{"total":3}`, false)
	var out bytes.Buffer
	if code := run([]string{rep, key}, &out); code != 0 || strings.TrimSpace(out.String()) != "OK" {
		t.Fatalf("code %d out %q", code, out.String())
	}
}

func TestVerifyTampered(t *testing.T) {
	rep, key := files(t, `{"total":3}`, true)
	var out bytes.Buffer
	if code := run([]string{rep, key}, &out); code != 1 || strings.TrimSpace(out.String()) != "TAMPERED" {
		t.Fatalf("code %d out %q", code, out.String())
	}
}

func TestVerifyUsage(t *testing.T) {
	var out bytes.Buffer
	if code := run(nil, &out); code != 2 || !strings.Contains(out.String(), "usage") {
		t.Fatalf("code %d out %q", code, out.String())
	}
}
