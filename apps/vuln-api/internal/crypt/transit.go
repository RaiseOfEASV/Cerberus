package crypt

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Transit is what the rest of vuln-api needs from Vault. crypttest.Fake
// implements it for tests.
type Transit interface {
	// DataKey returns a fresh 256-bit data key and its Vault-wrapped form.
	DataKey(ctx context.Context) (plaintext []byte, wrapped string, err error)
	Unwrap(ctx context.Context, wrapped string) ([]byte, error)
	// Sign returns "vault:vN:<base64 ed25519 signature>" and N.
	Sign(ctx context.Context, data []byte) (sig string, keyVersion int, err error)
	PublicKey(ctx context.Context, version int) (ed25519.PublicKey, error)
}

// Vault is the real Transit client. It authenticates with the pod's
// projected, audience-bound ServiceAccount token (Kubernetes auth method)
// and only trusts the internal CA for TLS.
type Vault struct {
	addr, role, tokenPath, dataKey, reportKey string
	hc                                        *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

func NewVault(addr, caCertPath, role, tokenPath, dataKey, reportKey string) (*Vault, error) {
	pem, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read Vault CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("no certificates in Vault CA file")
	}
	return &Vault{
		addr: strings.TrimRight(addr, "/"), role: role, tokenPath: tokenPath, dataKey: dataKey, reportKey: reportKey,
		hc: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		},
	}, nil
}

var errForbidden = errors.New("vault: permission denied")

func (v *Vault) login(ctx context.Context) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.token != "" && time.Now().Before(v.expires) {
		return v.token, nil
	}
	jwt, err := os.ReadFile(v.tokenPath)
	if err != nil {
		return "", fmt.Errorf("read service account token: %w", err)
	}
	var out struct {
		Auth struct {
			ClientToken   string `json:"client_token"`
			LeaseDuration int    `json:"lease_duration"`
		} `json:"auth"`
	}
	body := map[string]string{"role": v.role, "jwt": strings.TrimSpace(string(jwt))}
	if err := v.do(ctx, http.MethodPost, "auth/kubernetes/login", "", body, &out); err != nil {
		return "", fmt.Errorf("vault login: %w", err)
	}
	v.token = out.Auth.ClientToken
	v.expires = time.Now().Add(time.Duration(float64(out.Auth.LeaseDuration)*0.8) * time.Second)
	return v.token, nil
}

// call runs an authenticated request, re-logging in once on 403 (token
// revoked or expired early).
func (v *Vault) call(ctx context.Context, method, path string, in, out any) error {
	for attempt := 0; ; attempt++ {
		tok, err := v.login(ctx)
		if err != nil {
			return err
		}
		err = v.do(ctx, method, path, tok, in, out)
		if errors.Is(err, errForbidden) && attempt == 0 {
			v.mu.Lock()
			v.token = ""
			v.mu.Unlock()
			continue
		}
		return err
	}
}

func (v *Vault) do(ctx context.Context, method, path, token string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, v.addr+"/v1/"+path, body)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return errForbidden
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("vault %s %s: HTTP %d", method, path, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

func (v *Vault) DataKey(ctx context.Context) ([]byte, string, error) {
	var out struct {
		Data struct{ Plaintext, Ciphertext string } `json:"data"`
	}
	if err := v.call(ctx, http.MethodPost, "transit/datakey/plaintext/"+v.dataKey, map[string]int{"bits": 256}, &out); err != nil {
		return nil, "", err
	}
	pt, err := base64.StdEncoding.DecodeString(out.Data.Plaintext)
	if err != nil || len(pt) != 32 {
		return nil, "", errors.New("vault returned an invalid data key")
	}
	return pt, out.Data.Ciphertext, nil
}

func (v *Vault) Unwrap(ctx context.Context, wrapped string) ([]byte, error) {
	var out struct {
		Data struct{ Plaintext string } `json:"data"`
	}
	if err := v.call(ctx, http.MethodPost, "transit/decrypt/"+v.dataKey, map[string]string{"ciphertext": wrapped}, &out); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Data.Plaintext)
}

func (v *Vault) Sign(ctx context.Context, data []byte) (string, int, error) {
	var out struct {
		Data struct {
			Signature  string `json:"signature"`
			KeyVersion int    `json:"key_version"`
		} `json:"data"`
	}
	in := map[string]string{"input": base64.StdEncoding.EncodeToString(data)}
	if err := v.call(ctx, http.MethodPost, "transit/sign/"+v.reportKey, in, &out); err != nil {
		return "", 0, err
	}
	return out.Data.Signature, out.Data.KeyVersion, nil
}

func (v *Vault) PublicKey(ctx context.Context, version int) (ed25519.PublicKey, error) {
	var out struct {
		Data struct {
			Keys map[string]struct {
				PublicKey string `json:"public_key"`
			} `json:"keys"`
		} `json:"data"`
	}
	if err := v.call(ctx, http.MethodGet, "transit/keys/"+v.reportKey, nil, &out); err != nil {
		return nil, err
	}
	k, ok := out.Data.Keys[strconv.Itoa(version)]
	if !ok {
		return nil, fmt.Errorf("no report key version %d", version)
	}
	raw, err := base64.StdEncoding.DecodeString(k.PublicKey)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("vault returned an invalid public key")
	}
	return ed25519.PublicKey(raw), nil
}
