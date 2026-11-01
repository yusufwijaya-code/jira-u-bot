package jira_credential_crypto

import (
	"bytes"
	"testing"
)

func testKey() [32]byte {
	var k [32]byte
	copy(k[:], []byte("0123456789abcdef0123456789abcdef"))
	return k
}

func TestEncryptDecrypt_RoundTrips(t *testing.T) {
	key := testKey()
	plaintext := []byte("ATATT3xFfGF0-super-secret-jira-api-token")

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}
	if bytes.Contains(ciphertext, plaintext) {
		t.Fatal("ciphertext contains the plaintext token verbatim — encryption did nothing")
	}

	got, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt returned error: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestDecrypt_FailsWithWrongKey(t *testing.T) {
	key := testKey()
	ciphertext, err := Encrypt([]byte("secret-token"), key)
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	var wrongKey [32]byte
	copy(wrongKey[:], []byte("different-key-different-key-dif"))

	if _, err := Decrypt(ciphertext, wrongKey); err == nil {
		t.Fatal("expected Decrypt to fail with the wrong key, got nil error — must never return corrupted plaintext silently")
	}
}

func TestEncrypt_ProducesDifferentCiphertextEachCall(t *testing.T) {
	key := testKey()
	plaintext := []byte("same-token")

	c1, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}
	c2, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	if bytes.Equal(c1, c2) {
		t.Error("two Encrypt calls with the same plaintext produced identical ciphertext — nonce is not being randomized")
	}
}

func TestDecrypt_FailsOnTruncatedCiphertext(t *testing.T) {
	key := testKey()
	if _, err := Decrypt([]byte("short"), key); err == nil {
		t.Fatal("expected Decrypt to reject ciphertext shorter than the nonce, got nil error")
	}
}
