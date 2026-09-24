package code

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	Length   = 8
	alphabet = "23456789ABCDEFGHJKLMNPQRSTVWXY"
)

// Generate returns an eight-character code using digits 2-9 and uppercase ASCII letters except I, O, U, and Z.
func Generate() (string, error) {
	value := make([]byte, Length)
	limit := big.NewInt(int64(len(alphabet)))
	for i := range value {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("generate random code: %w", err)
		}
		value[i] = alphabet[index.Int64()]
	}
	return string(value), nil
}
