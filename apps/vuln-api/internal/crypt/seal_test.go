package crypt

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"
)

func key(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	k := key(t)
	ct, err := Seal(k, []byte("SQL injection in /login"), []byte("1:description"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, []byte("SQL injection")) {
		t.Fatal("plaintext visible in ciphertext")
	}
	pt, err := Open(k, ct, []byte("1:description"))
	if err != nil || string(pt) != "SQL injection in /login" {
		t.Fatalf("Open = %q, %v", pt, err)
	}
}

func TestOpenWrongAAD(t *testing.T) {
	k := key(t)
	ct, _ := Seal(k, []byte("secret"), []byte("1:description"))
	if _, err := Open(k, ct, []byte("2:description")); err == nil {
		t.Fatal("ciphertext moved to another finding must not decrypt")
	}
	if _, err := Open(k, ct, []byte("1:reporter_email")); err == nil {
		t.Fatal("ciphertext moved to another field must not decrypt")
	}
}

func TestOpenTampered(t *testing.T) {
	k := key(t)
	ct, _ := Seal(k, []byte("secret"), []byte("a"))
	ct[len(ct)-1] ^= 0x01
	if _, err := Open(k, ct, []byte("a")); err == nil {
		t.Fatal("tampered ciphertext decrypted")
	}
	if _, err := Open(k, ct[:5], []byte("a")); err == nil {
		t.Fatal("truncated ciphertext decrypted")
	}
}

func TestSealNonceUnique(t *testing.T) {
	k := key(t)
	a, _ := Seal(k, []byte("same"), []byte("x"))
	b, _ := Seal(k, []byte("same"), []byte("x"))
	if bytes.Equal(a, b) {
		t.Fatal("two seals of the same input are identical (nonce reuse)")
	}
}

func TestSealRejectsBadKey(t *testing.T) {
	if _, err := Seal(make([]byte, 16), []byte("x"), nil); err == nil {
		t.Fatal("AES-128 key accepted; data keys must be 256-bit")
	}
}

func TestVerifyReport(t *testing.T) {
	ctx := context.Background()
	tr := newFakeForTest(t)
	data := []byte(`{"findings":3}`)
	sig, ver, err := tr.Sign(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := tr.PublicKey(ctx, ver)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(pub, data, sig); err != nil {
		t.Fatalf("valid report rejected: %v", err)
	}
	if err := VerifyReport(pub, []byte(`{"findings":4}`), sig); err == nil {
		t.Fatal("altered report accepted")
	}
	if err := VerifyReport(pub, data, "garbage"); err == nil {
		t.Fatal("malformed signature accepted")
	}
}
