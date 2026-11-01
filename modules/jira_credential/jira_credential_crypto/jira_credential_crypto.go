package jira_credential_crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

// nonceSize is the standard AES-GCM nonce length.
const nonceSize = 12

// Encrypt seals plaintext with AES-256-GCM under key, returning
// nonce‖ciphertext‖tag as one blob — this is exactly what's stored in
// tr_usercredential.jiraAPITokenEncrypted (Tech Spec "Dashboard Jira
// Manage", Chapter 9e, AC5.2). A fresh random nonce is generated per call;
// reusing a nonce with the same key would break AES-GCM's guarantees, so
// this must never be exposed as "pass your own nonce" API.
func Encrypt(plaintext []byte, key [32]byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("jira_credential_crypto: failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("jira_credential_crypto: failed to create GCM: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("jira_credential_crypto: failed to generate nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt reverses Encrypt — a wrong key or corrupted/truncated ciphertext
// returns an error (GCM's authentication tag check), never garbage
// plaintext.
func Decrypt(ciphertext []byte, key [32]byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("jira_credential_crypto: failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("jira_credential_crypto: failed to create GCM: %w", err)
	}

	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("jira_credential_crypto: ciphertext too short")
	}
	nonce, sealed := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("jira_credential_crypto: decryption failed: %w", err)
	}
	return plaintext, nil
}
