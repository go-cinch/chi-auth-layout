package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func newSliderCaptchaTestManager(t *testing.T, enableE2ETest bool) *SliderCaptcha {
	t.Helper()
	manager, err := NewSliderCaptcha(NewMemoryPointCaptchaStore(), SliderCaptchaConfig{
		TTL: time.Minute, MinimumDuration: 100 * time.Millisecond, EnableE2ETest: func() bool { return enableE2ETest },
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
	manager := newSliderCaptchaTestManager(t, false)
	challenge, err := manager.IssueSliderChallenge(t.Context(), sliderCaptchaPurposeLogin, "readonly")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(challenge.CaptchaID, "slider-challenge:") || strings.Count(challenge.CaptchaID, ":") != 1 || strings.Contains(challenge.CaptchaID, ".") {
		t.Fatalf("slider challenge id = %q", challenge.CaptchaID)
	}
	result, err := manager.VerifySlider(t.Context(), validSliderVerification(challenge.CaptchaID, sliderCaptchaPurposeLogin, "readonly"))
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
	result, err = manager.VerifySlider(t.Context(), validSliderVerification(challenge.CaptchaID, sliderCaptchaPurposeRegister, "new-user"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ConsumeProof(t.Context(), sliderCaptchaPurposeRegister, "new-user", result.Proof); err != nil {
		t.Fatal(err)
	}
}

func TestSliderCaptchaRejectsSyntheticOrReplayedChallenges(t *testing.T) {
	manager := newSliderCaptchaTestManager(t, false)
	challenge, _ := manager.IssueSliderChallenge(t.Context(), sliderCaptchaPurposeLogin, "readonly")
	invalid := validSliderVerification(challenge.CaptchaID, sliderCaptchaPurposeLogin, "readonly")
	invalid.Duration = 1
	if _, err := manager.VerifySlider(t.Context(), invalid); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("short drag = %v", err)
	}
	if _, err := manager.VerifySlider(t.Context(), validSliderVerification(challenge.CaptchaID, sliderCaptchaPurposeLogin, "readonly")); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("challenge reuse = %v", err)
	}
	if _, err := manager.IssueSliderChallenge(t.Context(), "password_change", "readonly"); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("invalid purpose = %v", err)
	}
}

func TestSliderCaptchaE2ETestSkipsOnlyTheAnswer(t *testing.T) {
	manager := newSliderCaptchaTestManager(t, true)
	challenge, err := manager.IssueSliderChallenge(t.Context(), sliderCaptchaPurposeLogin, "readonly")
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.VerifySlider(t.Context(), SliderCaptchaVerification{
		CaptchaID: challenge.CaptchaID, Purpose: sliderCaptchaPurposeLogin, Username: "readonly",
	})
	if err != nil || result.Proof == "" {
		t.Fatalf("e2e verify = %#v, %v", result, err)
	}
	if err := manager.ConsumeProof(t.Context(), sliderCaptchaPurposeLogin, "readonly", result.Proof); err != nil {
		t.Fatal(err)
	}
	challenge, _ = manager.IssueSliderChallenge(t.Context(), sliderCaptchaPurposeLogin, "readonly")
	result, err = manager.VerifySlider(t.Context(), SliderCaptchaVerification{
		CaptchaID: challenge.CaptchaID, Purpose: sliderCaptchaPurposeLogin, Username: "readonly",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.enableE2ETest = func() bool { return false }
	if err := manager.ConsumeProof(t.Context(), sliderCaptchaPurposeLogin, "readonly", result.Proof); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("disabled e2e proof = %v", err)
	}
	manager.enableE2ETest = func() bool { return true }
	challenge, _ = manager.IssueSliderChallenge(t.Context(), sliderCaptchaPurposeLogin, "readonly")
	if _, err := manager.VerifySlider(t.Context(), SliderCaptchaVerification{
		CaptchaID: challenge.CaptchaID, Purpose: sliderCaptchaPurposeLogin, Username: "other-user",
	}); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("wrong subject bypass = %v", err)
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
