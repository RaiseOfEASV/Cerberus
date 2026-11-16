// Package crypt is VulnMan's cryptography: envelope encryption of finding
// fields with per-finding data keys from Vault Transit (FIND-11), and
// Ed25519 report signatures (FR-RPT-03). Keys never leave Vault except the
// per-finding data key, which only lives in memory while a request uses it.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Seal encrypts plaintext with AES-256-GCM under dek. The output is
// nonce ‖ ciphertext ‖ tag. aad binds the ciphertext to its place (D9:
// "<finding id>:<field>"), so it can't be moved to another row or column.
func Seal(dek, plaintext, aad []byte) ([]byte, error) {
	g, err := gcm(dek)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return g.Seal(nonce, nonce, plaintext, aad), nil
}

// Open reverses Seal; any change to the ciphertext, the key or the aad fails.
func Open(dek, sealed, aad []byte) ([]byte, error) {
	g, err := gcm(dek)
	if err != nil {
		return nil, err
	}
	if len(sealed) < g.NonceSize()+g.Overhead() {
		return nil, errors.New("ciphertext too short")
	}
	pt, err := g.Open(nil, sealed[:g.NonceSize()], sealed[g.NonceSize():], aad)
	if err != nil {
		return nil, errors.New("decryption failed")
	}
	return pt, nil
}

func gcm(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("data key must be 256 bits, got %d", len(key)*8)
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// VerifyReport checks a Vault-format signature ("vault:vN:<base64>") over
// data with an Ed25519 public key - usable offline by anyone with the key.
func VerifyReport(pub ed25519.PublicKey, data []byte, sig string) error {
	parts := strings.SplitN(sig, ":", 3)
	if len(parts) != 3 || parts[0] != "vault" || !strings.HasPrefix(parts[1], "v") {
		return errors.New("malformed signature")
	}
	raw, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return errors.New("malformed signature")
	}
	if !ed25519.Verify(pub, data, raw) {
		return errors.New("signature does not match")
	}
	return nil
}
