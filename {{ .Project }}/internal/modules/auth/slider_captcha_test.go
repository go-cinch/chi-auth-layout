package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newSliderCaptchaTestManager(t *testing.T, runtime, answer string) *SliderCaptcha {
	t.Helper()
	manager, err := NewSliderCaptcha(NewMemoryPointCaptchaStore(), SliderCaptchaConfig{
		TTL:                time.Minute,
		MinimumDuration:    100 * time.Millisecond,
		RuntimeEnvironment: runtime,
		CanaryHeaderValue:  "iwantsit",
		E2EAnswer:          answer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func validSliderVerification(challengeID, purpose, username string) SliderCaptchaVerification {
	return SliderCaptchaVerification{
		CaptchaID: challengeID,
		Purpose: purpose,
		Username: username,
		Duration: 100,
		Distance: 200,
		Width: 200,
		Tracks: []SliderCaptchaTrack{
			{X: 0, T: 0},
			{X: 100, T: 50},
			{X: 200, T: 100},
		},
	}
}

func TestSliderCaptchaProofIsBoundAndSingleUse(t *testing.T) {
	manager := newSliderCaptchaTestManager(t, "production", "")
	challenge, err := manager.IssueSliderChallenge(t.Context(), sliderCaptchaPurposeLogin, "readonly")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(challenge.CaptchaID, "slider-challenge:") || strings.Count(challenge.CaptchaID, ":") != 1 || strings.Contains(challenge.CaptchaID, ".") {
		t.Fatalf("slider challenge id = %q", challenge.CaptchaID)
	}
	result, err := manager.VerifySlider(t.Context(), validSliderVerification(challenge.CaptchaID, sliderCaptchaPurposeLogin, "readonly"), "", "")
	if err != nil || result.Proof == "" {
		t.Fatalf("verify = %#v, %v", result, err)
	}
	if !strings.HasPrefix(result.Proof, "slider-proof:") || strings.Count(result.Proof, ":") != 1 || strings.Contains(result.Proof, ".") {
		t.Fatalf("slider proof = %q", result.Proof)
	}
	if err := manager.ConsumeProof(t.Context(), sliderCaptchaPurposeLogin, "other-user", result.Proof); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("wrong subject = %v", err)
	}
	if err := manager.ConsumeProof(t.Context(), sliderCaptchaPurposeLogin, "readonly", result.Proof); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("consumed proof reuse = %v", err)
	}

	challenge, _ = manager.IssueSliderChallenge(t.Context(), sliderCaptchaPurposeRegister, "new-user")
	result, err = manager.VerifySlider(t.Context(), validSliderVerification(challenge.CaptchaID, sliderCaptchaPurposeRegister, "new-user"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ConsumeProof(t.Context(), sliderCaptchaPurposeRegister, "new-user", result.Proof); err != nil {
		t.Fatal(err)
	}
}

func TestSliderCaptchaRejectsSyntheticOrReplayedChallenges(t *testing.T) {
	manager := newSliderCaptchaTestManager(t, "production", "")
	challenge, _ := manager.IssueSliderChallenge(t.Context(), sliderCaptchaPurposeLogin, "readonly")
	invalid := validSliderVerification(challenge.CaptchaID, sliderCaptchaPurposeLogin, "readonly")
	invalid.Duration = 1
	if _, err := manager.VerifySlider(t.Context(), invalid, "", ""); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("short drag = %v", err)
	}
	if _, err := manager.VerifySlider(t.Context(), validSliderVerification(challenge.CaptchaID, sliderCaptchaPurposeLogin, "readonly"), "", ""); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("challenge reuse = %v", err)
	}
	if _, err := manager.IssueSliderChallenge(t.Context(), "password_change", "readonly"); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("invalid purpose = %v", err)
	}
}

func TestSliderCaptchaCanaryAnswerRequiresTrustedRuntimeAndHeader(t *testing.T) {
	for _, item := range []struct {
		name, runtime, header, answer string
		wantOK                        bool
	}{
		{name: "canary", runtime: "canary", header: "iwantsit", answer: "test-secret", wantOK: true},
		{name: "stable runtime", runtime: "stable", header: "iwantsit", answer: "test-secret"},
		{name: "missing route header", runtime: "canary", answer: "test-secret"},
		{name: "wrong answer", runtime: "canary", header: "iwantsit", answer: "wrong"},
	} {
		t.Run(item.name, func(t *testing.T) {
			manager := newSliderCaptchaTestManager(t, item.runtime, "test-secret")
			challenge, _ := manager.IssueSliderChallenge(t.Context(), sliderCaptchaPurposeLogin, "readonly")
			input := SliderCaptchaVerification{CaptchaID: challenge.CaptchaID, Purpose: sliderCaptchaPurposeLogin, Username: "readonly"}
			result, err := manager.VerifySlider(t.Context(), input, item.header, item.answer)
			if item.wantOK && (err != nil || result.Proof == "") {
				t.Fatalf("canary verify = %#v, %v", result, err)
			}
			if !item.wantOK && !errors.Is(err, ErrSliderCaptchaRequired) {
				t.Fatalf("unexpected bypass = %v", err)
			}
		})
	}
}

func TestSliderCaptchaReloadsCanaryRoleFile(t *testing.T) {
	roleFile := filepath.Join(t.TempDir(), "release-track")
	if err := os.WriteFile(roleFile, []byte("canary\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewSliderCaptcha(NewMemoryPointCaptchaStore(), SliderCaptchaConfig{
		TTL: time.Minute, MinimumDuration: 100 * time.Millisecond,
		RuntimeEnvironment: "stable", RuntimeEnvironmentFile: roleFile,
		CanaryHeaderValue: "iwantsit", E2EAnswer: "test-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	verify := func() (string, error) {
		challenge, challengeErr := manager.IssueSliderChallenge(t.Context(), sliderCaptchaPurposeLogin, "readonly")
		if challengeErr != nil {
			return "", challengeErr
		}
		result, verifyErr := manager.VerifySlider(t.Context(), SliderCaptchaVerification{
			CaptchaID: challenge.CaptchaID, Purpose: sliderCaptchaPurposeLogin, Username: "readonly",
		}, "iwantsit", "test-secret")
		if verifyErr != nil {
			return "", verifyErr
		}
		return result.Proof, nil
	}
	proof, err := verify()
	if err != nil {
		t.Fatalf("canary file = %v", err)
	}
	if err := os.WriteFile(roleFile, []byte("stable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.ConsumeProof(t.Context(), sliderCaptchaPurposeLogin, "readonly", proof); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("promoted canary proof = %v", err)
	}
	if _, err := verify(); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("promoted role = %v", err)
	}
}

func TestNewSliderCaptchaValidatesConfiguration(t *testing.T) {
	valid := SliderCaptchaConfig{TTL: time.Minute, MinimumDuration: time.Second}
	for _, item := range []struct {
		store PointCaptchaStore
		cfg   SliderCaptchaConfig
	}{
		{nil, valid},
		{NewMemoryPointCaptchaStore(), SliderCaptchaConfig{MinimumDuration: time.Second}},
		{NewMemoryPointCaptchaStore(), SliderCaptchaConfig{TTL: time.Minute, MinimumDuration: time.Millisecond}},
	} {
		if _, err := NewSliderCaptcha(item.store, item.cfg); err == nil {
			t.Fatal("expected invalid slider captcha configuration")
		}
	}
}
