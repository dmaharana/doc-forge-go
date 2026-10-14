package crypto

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := []byte("12345678901234567890123456789012") // 32 bytes
	plaintext := "sk-ant-api03-secret-token-key-value-12345"

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if ciphertext == plaintext {
		t.Fatalf("ciphertext equals plaintext")
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("decrypted text mismatch: got %s, want %s", decrypted, plaintext)
	}
}

func TestInvalidKeyLength(t *testing.T) {
	shortKey := []byte("too-short")
	_, err := Encrypt("secret", shortKey)
	if err != ErrInvalidKey {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}
}
