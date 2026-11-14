package auth

import (
	"encoding/base32"
	"errors"
	"net/url"
	"testing"
	"time"
)

func TestTOTPRFC6238VectorsAndReplay(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for _, tc := range []struct {
		at   int64
		code string
	}{
		{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"},
	} {
		now := time.Unix(tc.at, 0)
		step, err := verifyTOTP(secret, tc.code, now, -1, 30*time.Second)
		if err != nil || step != tc.at/30 {
			t.Fatalf("RFC vector at %d failed", tc.at)
		}
		if _, err := verifyTOTP(secret, tc.code, now, step, 30*time.Second); !errors.Is(err, ErrVerificationInvalid) {
			t.Fatal("replayed time step accepted")
		}
		if _, err := verifyTOTP(secret, tc.code, now.Add(90*time.Second), -1, 30*time.Second); !errors.Is(err, ErrVerificationInvalid) {
			t.Fatal("expired code accepted")
		}
	}
	for _, code := range []string{"", "12345", "1234567", "abcdef"} {
		if _, err := verifyTOTP(secret, code, time.Now(), -1, 30*time.Second); !errors.Is(err, ErrVerificationInvalid) {
			t.Fatal("invalid code accepted")
		}
	}
	if _, err := totpCode("broken!", 1); err == nil {
		t.Fatal("invalid secret accepted")
	}
	if _, err := totpCode(secret, -1); err == nil {
		t.Fatal("negative step accepted")
	}
	if _, err := verifyTOTP("broken!", "000000", time.Now(), -1, 30*time.Second); err == nil {
		t.Fatal("invalid secret accepted")
	}
}

func TestTOTPEnrollmentMaterial(t *testing.T) {
	secret, err := newTOTPSecret()
	if err != nil || len(secret) != 32 {
		t.Fatal("secret generation failed")
	}
	uri, err := url.Parse(totpURI("Test Service", "acme admin - user+qa@example.com", secret, 30*time.Second))
	if err != nil || uri.Scheme != "otpauth" || uri.Host != "totp" || uri.Path != "/Test Service:acme admin - user+qa@example.com" {
		t.Fatal("invalid enrollment account label")
	}
	if uri.Query().Get("secret") != secret || uri.Query().Get("issuer") != "Test Service" || uri.Query().Get("algorithm") != "SHA1" || uri.Query().Get("digits") != "6" || uri.Query().Get("period") != "30" {
		t.Fatal("account label changed TOTP parameters")
	}
}

func TestConfiguredTOTPPeriod(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	// RFC 4226's counter-2 vector: with a 60-second period, Unix 120 is step 2.
	step, err := verifyTOTP(secret, "359152", time.Unix(120, 0), -1, time.Minute)
	if err != nil || step != 2 {
		t.Fatal("configured period was ignored", err)
	}
	if _, err := verifyTOTP(secret, "359152", time.Unix(120, 0), -1, 30*time.Second); !errors.Is(err, ErrVerificationInvalid) {
		t.Fatal("wrong period accepted", err)
	}
	if _, err := verifyTOTP(secret, "359152", time.Unix(120, 0), step, time.Minute); !errors.Is(err, ErrVerificationInvalid) {
		t.Fatal("custom-period replay accepted", err)
	}
	for _, invalid := range []time.Duration{0, -time.Second, 1500 * time.Millisecond} {
		if _, err := verifyTOTP(secret, "359152", time.Unix(120, 0), -1, invalid); !errors.Is(err, ErrVerificationInvalid) {
			t.Fatal("invalid period accepted", invalid)
		}
	}
	uri, err := url.Parse(totpURI("Example", "account", secret, time.Minute))
	if err != nil || uri.Query().Get("period") != "60" {
		t.Fatal("QR period differs from verification period", err)
	}
}
