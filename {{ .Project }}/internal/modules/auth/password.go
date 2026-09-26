package auth

import (
	"crypto/sha256"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const longPasswordHashPrefix = "$bcrypt-sha256$"

// HashPassword keeps bcrypt compatibility for existing passwords while
// pre-hashing values that exceed bcrypt's 72-byte input limit.
func HashPassword(password string) ([]byte, error) {
	value := []byte(password)
	if len(value) <= 72 {
		return bcrypt.GenerateFromPassword(value, bcrypt.DefaultCost)
	}
	digest := sha256.Sum256(value)
	hash, err := bcrypt.GenerateFromPassword(digest[:], bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return []byte(longPasswordHashPrefix + string(hash)), nil
}

// VerifyPassword supports both legacy bcrypt hashes and the long-password
// representation emitted by HashPassword.
func VerifyPassword(hash, password string) bool {
	if strings.HasPrefix(hash, longPasswordHashPrefix) {
		digest := sha256.Sum256([]byte(password))
		return bcrypt.CompareHashAndPassword([]byte(strings.TrimPrefix(hash, longPasswordHashPrefix)), digest[:]) == nil
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
