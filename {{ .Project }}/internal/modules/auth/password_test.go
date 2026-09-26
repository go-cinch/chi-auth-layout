package auth

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPasswordHashCompatibility(t *testing.T) {
	legacy, err := bcrypt.GenerateFromPassword([]byte("legacy-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(string(legacy), "legacy-password") || VerifyPassword(string(legacy), "wrong") {
		t.Fatal("legacy bcrypt password verification")
	}

	for _, password := range []string{"x", " password with spaces ", strings.Repeat("密码", 100)} {
		hash, err := HashPassword(password)
		if err != nil {
			t.Fatalf("hash password: %v", err)
		}
		if !VerifyPassword(string(hash), password) || VerifyPassword(string(hash), password+"x") {
			t.Fatalf("password verification failed for %d-byte password", len(password))
		}
	}
}
