package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func newSliderCaptchaTestManager(t *testing.T, skipDuration bool) *SliderCaptcha {
	t.Helper()
	manager, err := NewSliderCaptcha(NewMemoryPointCaptchaStore(), SliderCaptchaConfig{
		TTL: time.Minute, MinimumDuration: 100 * time.Millisecond, SkipDuration: skipDuration,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func validSliderVerification(challengeID, purpose, username string) SliderCaptchaVerification {
	return SliderCaptchaVerification{
		CaptchaID: challengeID,
		Purpose:   purpose,
		Username:  username,
		Duration:  100,
		Distance:  200,
		Width:     200,
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

func TestSliderCaptchaSkipDuration(t *testing.T) {
	manager := newSliderCaptchaTestManager(t, true)
	issue := func(t *testing.T) SliderCaptchaVerification {
		t.Helper()
		challenge, err := manager.IssueSliderChallenge(t.Context(), sliderCaptchaPurposeLogin, "readonly")
		if err != nil {
			t.Fatal(err)
		}
		input := validSliderVerification(challenge.CaptchaID, sliderCaptchaPurposeLogin, "readonly")
		input.Duration = 2
		input.Tracks[1].T = 1
		input.Tracks[2].T = 2
		return input
	}
	for _, duration := range []int{2, 61000} {
		input := issue(t)
		input.Duration = duration
		result, err := manager.VerifySlider(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		if err := manager.ConsumeProof(t.Context(), input.Purpose, input.Username, result.Proof); err != nil {
			t.Fatal(err)
		}
	}

	for name, invalidate := range map[string]func(*SliderCaptchaVerification){
		"subject":                 func(v *SliderCaptchaVerification) { v.Username = "other-user" },
		"width":                   func(v *SliderCaptchaVerification) { v.Width = 0 },
		"distance":                func(v *SliderCaptchaVerification) { v.Distance = 1 },
		"missing tracks":          func(v *SliderCaptchaVerification) { v.Tracks = nil },
		"end position":            func(v *SliderCaptchaVerification) { v.Tracks[2].X = 50 },
		"reversed timestamps":     func(v *SliderCaptchaVerification) { v.Tracks[2].T = 0 },
		"inconsistent timestamps": func(v *SliderCaptchaVerification) { v.Tracks[2].T = 1000 },
		"negative duration":       func(v *SliderCaptchaVerification) { v.Duration = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			input := issue(t)
			invalidate(&input)
			if _, err := manager.VerifySlider(t.Context(), input); !errors.Is(err, ErrSliderCaptchaRequired) {
				t.Fatalf("invalid verification accepted or unexpected error: %v", err)
			}
		})
	}

	input := issue(t)
	result, err := manager.VerifySlider(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	strictManager, err := NewSliderCaptcha(manager.store, SliderCaptchaConfig{TTL: time.Minute, MinimumDuration: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if err := strictManager.ConsumeProof(t.Context(), input.Purpose, input.Username, result.Proof); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatalf("strict manager consumed a test proof: %v", err)
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
