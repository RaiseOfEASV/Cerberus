// Package crypttest provides an in-memory stand-in for Vault Transit, for
// tests in other packages. It wraps data keys with a random AES key and
// signs with a real Ed25519 key, so callers exercise real cryptography.
package crypttest

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
)

type Fake struct {
	kek  cipher.AEAD
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
	// FailUnwrap makes Unwrap fail, to test Vault being unavailable.
	FailUnwrap bool
}

func New() *Fake {
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	b, _ := aes.NewCipher(k)
	g, _ := cipher.NewGCM(b)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	return &Fake{kek: g, pub: pub, priv: priv}
}

func (f *Fake) DataKey(context.Context) ([]byte, string, error) {
	dek := make([]byte, 32)
	_, _ = rand.Read(dek)
	nonce := make([]byte, f.kek.NonceSize())
	_, _ = rand.Read(nonce)
	return dek, "fake:v1:" + base64.StdEncoding.EncodeToString(f.kek.Seal(nonce, nonce, dek, nil)), nil
}

func (f *Fake) Unwrap(_ context.Context, wrapped string) ([]byte, error) {
	if f.FailUnwrap {
		return nil, errors.New("vault unavailable")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(wrapped, "fake:v1:"))
	if err != nil || len(raw) < f.kek.NonceSize() {
		return nil, errors.New("bad wrapped key")
	}
	return f.kek.Open(nil, raw[:f.kek.NonceSize()], raw[f.kek.NonceSize():], nil)
}

func (f *Fake) Sign(_ context.Context, data []byte) (string, int, error) {
	return "vault:v1:" + base64.StdEncoding.EncodeToString(ed25519.Sign(f.priv, data)), 1, nil
}

func (f *Fake) PublicKey(_ context.Context, version int) (ed25519.PublicKey, error) {
	if version != 1 {
		return nil, errors.New("no such key version")
	}
	return f.pub, nil
}
