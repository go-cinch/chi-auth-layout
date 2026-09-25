package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/apperror"
)

const (
	sliderCaptchaPurposeLogin    = "login"
	sliderCaptchaPurposeRegister = "register"
	sliderCaptchaCanaryRuntime   = "canary"
	maxSliderCaptchaTracks       = 128
)

var ErrSliderCaptchaRequired = apperror.New("AUTH_SLIDER_CAPTCHA_REQUIRED", "slider verification is required")

type SliderCaptchaConfig struct {
	TTL                time.Duration
	MinimumDuration    time.Duration
	RuntimeEnvironment string
	RuntimeEnvironmentFile string
	CanaryHeaderValue  string
	E2EAnswer          string
}

type SliderCaptchaChallenge struct {
	CaptchaID string `json:"captcha_id"`
	ExpiredAt int64  `json:"expired_at"`
}

type SliderCaptchaTrack struct {
	X int `json:"x"`
	T int `json:"t"`
}

type SliderCaptchaVerification struct {
	CaptchaID string               `json:"captcha_id"`
	Username  string               `json:"username"`
	Purpose   string               `json:"purpose"`
	Duration  int                  `json:"duration_ms"`
	Distance  int                  `json:"distance"`
	Width     int                  `json:"width"`
	Tracks    []SliderCaptchaTrack `json:"tracks"`
}

type SliderCaptchaVerificationResult struct {
	Proof string `json:"proof"`
}

type sliderCaptchaRecord struct {
	Kind          string `json:"kind"`
	Purpose       string `json:"purpose"`
	SubjectDigest string `json:"subject_digest"`
	CanaryOnly    bool   `json:"canary_only,omitempty"`
}

type SliderCaptcha struct {
	store              PointCaptchaStore
	ttl                time.Duration
	minimumDuration    time.Duration
	runtimeEnvironment string
	runtimeEnvironmentFile string
	canaryHeaderValue  string
	e2eAnswer          string
}

func NewSliderCaptcha(store PointCaptchaStore, cfg SliderCaptchaConfig) (*SliderCaptcha, error) {
	if store == nil {
		return nil, errors.New("slider captcha store is required")
	}
	if cfg.TTL <= 0 || cfg.TTL > 10*time.Minute {
		return nil, errors.New("slider captcha ttl must be between 1ns and 10m")
	}
	if cfg.MinimumDuration < 100*time.Millisecond || cfg.MinimumDuration > 5*time.Second {
		return nil, errors.New("slider captcha minimum duration must be between 100ms and 5s")
	}
	return &SliderCaptcha{
		store:              store,
		ttl:                cfg.TTL,
		minimumDuration:    cfg.MinimumDuration,
		runtimeEnvironment: strings.TrimSpace(cfg.RuntimeEnvironment),
		runtimeEnvironmentFile: strings.TrimSpace(cfg.RuntimeEnvironmentFile),
		canaryHeaderValue:  strings.TrimSpace(cfg.CanaryHeaderValue),
		e2eAnswer:          cfg.E2EAnswer,
	}, nil
}

func (c *SliderCaptcha) IssueSliderChallenge(ctx context.Context, purpose, username string) (*SliderCaptchaChallenge, error) {
	purpose, username, ok := normalizeSliderCaptchaSubject(purpose, username)
	if !ok {
		return nil, ErrSliderCaptchaRequired
	}
	id, err := newSliderCaptchaID("slider-challenge")
	if err != nil {
		return nil, err
	}
	record, err := json.Marshal(sliderCaptchaRecord{Kind: "challenge", Purpose: purpose, SubjectDigest: usernameDigest(username)})
	if err != nil {
		return nil, fmt.Errorf("encode slider captcha challenge: %w", err)
	}
	if err := c.store.Put(ctx, id, string(record), c.ttl); err != nil {
		return nil, fmt.Errorf("store slider captcha challenge: %w", err)
	}
	return &SliderCaptchaChallenge{CaptchaID: id, ExpiredAt: time.Now().Add(c.ttl).UnixMilli()}, nil
}

func (c *SliderCaptcha) VerifySlider(ctx context.Context, input SliderCaptchaVerification, canaryHeader, e2eAnswer string) (*SliderCaptchaVerificationResult, error) {
	purpose, username, ok := normalizeSliderCaptchaSubject(input.Purpose, input.Username)
	if !ok || !validSliderCaptchaID(input.CaptchaID, "slider-challenge") {
		return nil, ErrSliderCaptchaRequired
	}
	value, err := c.store.Take(ctx, input.CaptchaID)
	if errors.Is(err, ErrPointCaptchaNotFound) {
		return nil, ErrSliderCaptchaRequired
	}
	if err != nil {
		return nil, fmt.Errorf("consume slider captcha challenge: %w", err)
	}
	var record sliderCaptchaRecord
	if json.Unmarshal([]byte(value), &record) != nil || record.Kind != "challenge" || record.Purpose != purpose ||
		subtle.ConstantTimeCompare([]byte(record.SubjectDigest), []byte(usernameDigest(username))) != 1 {
		return nil, ErrSliderCaptchaRequired
	}
	canaryOnly := c.validCanaryAnswer(canaryHeader, e2eAnswer)
	if !canaryOnly && !c.validDrag(input) {
		return nil, ErrSliderCaptchaRequired
	}
	proof, err := newSliderCaptchaID("slider-proof")
	if err != nil {
		return nil, err
	}
	record = sliderCaptchaRecord{Kind: "proof", Purpose: purpose, SubjectDigest: usernameDigest(username), CanaryOnly: canaryOnly}
	encoded, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode slider captcha proof: %w", err)
	}
	if err := c.store.Put(ctx, proof, string(encoded), c.ttl); err != nil {
		return nil, fmt.Errorf("store slider captcha proof: %w", err)
	}
	return &SliderCaptchaVerificationResult{Proof: proof}, nil
}

func (c *SliderCaptcha) ConsumeProof(ctx context.Context, purpose, username, proof string) error {
	purpose, username, ok := normalizeSliderCaptchaSubject(purpose, username)
	if !ok || !validSliderCaptchaID(proof, "slider-proof") {
		return ErrSliderCaptchaRequired
	}
	value, err := c.store.Take(ctx, proof)
	if errors.Is(err, ErrPointCaptchaNotFound) {
		return ErrSliderCaptchaRequired
	}
	if err != nil {
		return fmt.Errorf("consume slider captcha proof: %w", err)
	}
	var record sliderCaptchaRecord
	if json.Unmarshal([]byte(value), &record) != nil || record.Kind != "proof" || record.Purpose != purpose ||
		subtle.ConstantTimeCompare([]byte(record.SubjectDigest), []byte(usernameDigest(username))) != 1 {
		return ErrSliderCaptchaRequired
	}
	if record.CanaryOnly && c.currentRuntimeEnvironment() != sliderCaptchaCanaryRuntime {
		return ErrSliderCaptchaRequired
	}
	return nil
}

func (c *SliderCaptcha) validCanaryAnswer(header, answer string) bool {
	return c.currentRuntimeEnvironment() == sliderCaptchaCanaryRuntime && c.canaryHeaderValue != "" && c.e2eAnswer != "" &&
		subtle.ConstantTimeCompare([]byte(c.canaryHeaderValue), []byte(header)) == 1 &&
		subtle.ConstantTimeCompare([]byte(c.e2eAnswer), []byte(answer)) == 1
}

func (c *SliderCaptcha) currentRuntimeEnvironment() string {
	runtimeEnvironment := c.runtimeEnvironment
	if c.runtimeEnvironmentFile != "" {
		value, err := os.ReadFile(c.runtimeEnvironmentFile)
		if err != nil {
			return ""
		}
		runtimeEnvironment = strings.TrimSpace(string(value))
	}
	return runtimeEnvironment
}

func (c *SliderCaptcha) validDrag(input SliderCaptchaVerification) bool {
	minimumMilliseconds := int(c.minimumDuration / time.Millisecond)
	if input.Duration < minimumMilliseconds || input.Duration > int(c.ttl/time.Millisecond) || input.Width < 160 || input.Width > 2000 ||
		input.Distance*100 < input.Width*90 || input.Distance > input.Width || len(input.Tracks) < 3 || len(input.Tracks) > maxSliderCaptchaTracks {
		return false
	}
	previous := input.Tracks[0]
	if previous.T < 0 || previous.T > 100 || previous.X < 0 || previous.X*100 > input.Width*15 {
		return false
	}
	changed := 0
	for _, current := range input.Tracks[1:] {
		if current.T < previous.T || current.T > input.Duration+250 || current.X < 0 || current.X > input.Width {
			return false
		}
		if current.X != previous.X {
			changed++
		}
		previous = current
	}
	return changed >= 2 && previous.X*100 >= input.Width*90 && previous.T >= minimumMilliseconds
}

func normalizeSliderCaptchaSubject(purpose, username string) (string, string, bool) {
	purpose = strings.TrimSpace(purpose)
	username = strings.TrimSpace(username)
	return purpose, username, (purpose == sliderCaptchaPurposeLogin || purpose == sliderCaptchaPurposeRegister) && username != "" && len(username) <= 128
}

func newSliderCaptchaID(prefix string) (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate slider captcha id: %w", err)
	}
	return prefix + ":" + base64.RawURLEncoding.EncodeToString(value), nil
}

func validSliderCaptchaID(value, prefix string) bool {
	// Dot-separated IDs remain readable for the short rolling-deployment window.
	return strings.HasPrefix(value, prefix+":") || strings.HasPrefix(value, prefix+".")
}
