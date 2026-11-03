package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// TOTP uses RFC 6238's SHA-1 / six-digit profile with a configured time step.
// SHA-1 is used as HMAC here, not as a collision-resistant digest.
func totpCode(secret string, step int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil || len(key) < 20 || step < 0 {
		return "", ErrMFAInvalid
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 15
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000), nil
}

func verifyTOTP(secret, code string, now time.Time, lastStep int64, period time.Duration) (int64, error) {
	if period <= 0 || period%time.Second != 0 {
		return 0, ErrMFAInvalid
	}
	if len(code) != 6 {
		return 0, ErrMFAInvalid
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return 0, ErrMFAInvalid
		}
	}
	current := now.Unix() / int64(period/time.Second)
	for _, step := range []int64{current, current - 1, current + 1} {
		if step <= lastStep {
			continue
		}
		expected, err := totpCode(secret, step)
		if err != nil {
			return 0, err
		}
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return step, nil
		}
	}
	return 0, ErrMFAInvalid
}

func newTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

func totpURI(issuer, accountName, secret string, period time.Duration) string {
	u := url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + issuer + ":" + accountName}
	q := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {strconv.FormatInt(int64(period/time.Second), 10)}}
	u.RawQuery = q.Encode()
	return u.String()
}
