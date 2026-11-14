package auth

import (
	"strings"
	"testing"
)

func TestTOTPEncryption(t *testing.T) {
	if _, err := verificationCipher([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
	cipher, err := verificationCipher([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	secret := "test secret"
	sealed, err := sealVerification(cipher, secret, "totp:1")
	if err != nil || strings.Contains(sealed, secret) {
		t.Fatal("encryption failed")
	}
	plain, err := openVerification(cipher, sealed, "totp:1")
	if err != nil || plain != secret {
		t.Fatal("decryption failed")
	}
	for _, value := range []string{sealed, "short!", sealed[:len(sealed)-3]} {
		if _, err := openVerification(cipher, value, "totp:2"); err == nil {
			t.Fatal("cross-user or corrupt ciphertext accepted")
		}
	}
}
