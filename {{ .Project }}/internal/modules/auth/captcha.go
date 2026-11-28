package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/apperror"

{{- if .Computed.enable_redis_final }}
	"github.com/redis/go-redis/v9"
{{- end }}
)

const (
	pointCaptchaEnglishDictionaryKey  = "POINT_CAPTCHA_ENGLISH_CHARACTERS"
	pointCaptchaChineseDictionaryKey  = "POINT_CAPTCHA_CHINESE_CHARACTERS"
	pointCaptchaMinGlyphCount         = 6
	pointCaptchaMaxGlyphCount         = 8
	pointCaptchaMinTargetCount        = 2
	pointCaptchaMaxTargetCount        = 4
	pointCaptchaPurposeLogin          = "login"
	pointCaptchaPurposePasswordChange = "password_change"
)

var ErrPointCaptchaNotFound = apperror.New("AUTH_POINT_CAPTCHA_NOT_FOUND", "point captcha not found")

type CaptchaPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type PointCaptchaChallenge struct {
	CaptchaID    string `json:"captcha_id"`
	CaptchaImage string `json:"captcha_image"`
	HintText     string `json:"hint_text"`
	TargetCount  int    `json:"target_count"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	ExpiredAt    int64  `json:"expired_at"`
}

type PointCaptchaStore interface {
	Put(context.Context, string, string, time.Duration) error
	Take(context.Context, string) (string, error)
}

type PointCaptchaDictionary interface {
	DictionaryValue(context.Context, string) (json.RawMessage, error)
}

type pointCaptchaAnswer struct {
	Purpose       string         `json:"purpose"`
	SubjectDigest string         `json:"subject_digest"`
	Points        []CaptchaPoint `json:"points"`
}

type PointCaptcha struct {
	store      PointCaptchaStore
	dictionary PointCaptchaDictionary
	threshold  int64
	ttl        time.Duration
	width      int
	height     int
	tolerance  int
	random     io.Reader
	skipAnswer bool
}

func NewPointCaptcha(store PointCaptchaStore, dictionary PointCaptchaDictionary, threshold int64, ttl time.Duration, width, height, tolerance int, skipAnswer bool) (*PointCaptcha, error) {
	if store == nil {
		return nil, errors.New("point captcha store is required")
	}
	if dictionary == nil {
		return nil, errors.New("point captcha dictionary is required")
	}
	if threshold < 1 {
		return nil, errors.New("point captcha threshold must be positive")
	}
	if ttl <= 0 || ttl > 10*time.Minute {
		return nil, errors.New("point captcha ttl must be between 1ns and 10m")
	}
	if width < 240 || width > 800 || height < 140 || height > 500 {
		return nil, errors.New("point captcha dimensions are out of range")
	}
	if tolerance < 8 || tolerance > 40 {
		return nil, errors.New("point captcha tolerance must be between 8 and 40 pixels")
	}
	return &PointCaptcha{
		store: store, dictionary: dictionary, threshold: threshold, ttl: ttl, width: width, height: height,
		tolerance: tolerance, random: rand.Reader, skipAnswer: skipAnswer,
	}, nil
}

func (c *PointCaptcha) Required(wrong int64) bool {
	return wrong >= c.threshold
}

func (c *PointCaptcha) NewChallenge(ctx context.Context, username string) (*PointCaptchaChallenge, error) {
	return c.challenge(ctx, pointCaptchaPurposeLogin, usernameDigest(strings.TrimSpace(username)))
}

func (c *PointCaptcha) RefreshChallenge(ctx context.Context, captchaID string) (*PointCaptchaChallenge, error) {
	return c.refreshChallenge(ctx, captchaID, pointCaptchaPurposeLogin)
}

func (c *PointCaptcha) NewPasswordChangeChallenge(ctx context.Context, subject string) (*PointCaptchaChallenge, error) {
	return c.challenge(ctx, pointCaptchaPurposePasswordChange, usernameDigest(strings.TrimSpace(subject)))
}

func (c *PointCaptcha) RefreshPasswordChangeChallenge(ctx context.Context, captchaID string) (*PointCaptchaChallenge, error) {
	return c.refreshChallenge(ctx, captchaID, pointCaptchaPurposePasswordChange)
}

func (c *PointCaptcha) refreshChallenge(ctx context.Context, captchaID, purpose string) (*PointCaptchaChallenge, error) {
	captchaID = strings.TrimSpace(captchaID)
	if !validPointCaptchaID(captchaID, purpose) {
		return nil, ErrPointCaptchaNotFound
	}
	value, err := c.store.Take(ctx, captchaID)
	if err != nil {
		return nil, err
	}
	answer, err := decodePointCaptchaAnswer(value)
	if err != nil || answer.Purpose != purpose {
		return nil, ErrPointCaptchaNotFound
	}
	return c.challenge(ctx, purpose, answer.SubjectDigest)
}

func (c *PointCaptcha) Verify(ctx context.Context, username, captchaID string, points []CaptchaPoint) (bool, error) {
	return c.verify(ctx, pointCaptchaPurposeLogin, usernameDigest(strings.TrimSpace(username)), captchaID, points)
}

func (c *PointCaptcha) VerifyPasswordChange(ctx context.Context, subject, captchaID string, points []CaptchaPoint) (bool, error) {
	return c.verify(ctx, pointCaptchaPurposePasswordChange, usernameDigest(strings.TrimSpace(subject)), captchaID, points)
}

func (c *PointCaptcha) Check(ctx context.Context, username, captchaID string, points []CaptchaPoint) (bool, error) {
	return c.check(ctx, pointCaptchaPurposeLogin, usernameDigest(strings.TrimSpace(username)), captchaID, points)
}

func (c *PointCaptcha) CheckPasswordChange(ctx context.Context, subject, captchaID string, points []CaptchaPoint) (bool, error) {
	return c.check(ctx, pointCaptchaPurposePasswordChange, usernameDigest(strings.TrimSpace(subject)), captchaID, points)
}

func (c *PointCaptcha) verify(ctx context.Context, purpose, subjectDigest, captchaID string, points []CaptchaPoint) (bool, error) {
	captchaID = strings.TrimSpace(captchaID)
	if captchaID == "" || len(captchaID) > 128 || !validPointCaptchaID(captchaID, purpose) || (!c.skipAnswer && !validPointCaptchaTargetCount(len(points))) {
		return false, nil
	}
	value, err := c.store.Take(ctx, captchaID)
	if errors.Is(err, ErrPointCaptchaNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("consume point captcha: %w", err)
	}
	answer, err := decodePointCaptchaAnswer(value)
	if err != nil || !pointCaptchaSubjectMatches(answer, purpose, subjectDigest) || (!c.skipAnswer && !pointCaptchaAnswerMatches(answer, purpose, subjectDigest, points, c.tolerance)) {
		return false, nil
	}
	return true, nil
}

func (c *PointCaptcha) check(ctx context.Context, purpose, subjectDigest, captchaID string, points []CaptchaPoint) (bool, error) {
	captchaID = strings.TrimSpace(captchaID)
	if captchaID == "" || len(captchaID) > 128 || !validPointCaptchaID(captchaID, purpose) || (!c.skipAnswer && !validPointCaptchaTargetCount(len(points))) {
		return false, nil
	}
	value, err := c.store.Take(ctx, captchaID)
	if errors.Is(err, ErrPointCaptchaNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("consume point captcha for check: %w", err)
	}
	answer, err := decodePointCaptchaAnswer(value)
	if err != nil || !pointCaptchaSubjectMatches(answer, purpose, subjectDigest) || (!c.skipAnswer && !pointCaptchaAnswerMatches(answer, purpose, subjectDigest, points, c.tolerance)) {
		return false, nil
	}
	if err := c.store.Put(ctx, captchaID, value, c.ttl); err != nil {
		return false, fmt.Errorf("restore checked point captcha: %w", err)
	}
	return true, nil
}

func pointCaptchaAnswerMatches(answer pointCaptchaAnswer, purpose, subjectDigest string, points []CaptchaPoint, tolerance int) bool {
	if !pointCaptchaSubjectMatches(answer, purpose, subjectDigest) || len(points) != len(answer.Points) {
		return false
	}
	toleranceSquared := tolerance * tolerance
	for index, expected := range answer.Points {
		dx := points[index].X - expected.X
		dy := points[index].Y - expected.Y
		if dx*dx+dy*dy > toleranceSquared {
			return false
		}
	}
	return true
}

func pointCaptchaSubjectMatches(answer pointCaptchaAnswer, purpose, subjectDigest string) bool {
	return answer.Purpose == purpose && subtle.ConstantTimeCompare([]byte(answer.SubjectDigest), []byte(subjectDigest)) == 1
}

func (c *PointCaptcha) challenge(ctx context.Context, purpose, subjectDigest string) (*PointCaptchaChallenge, error) {
	idBytes := make([]byte, 32)
	if _, err := io.ReadFull(c.random, idBytes); err != nil {
		return nil, fmt.Errorf("generate point captcha id: %w", err)
	}
	id := pointCaptchaIDPrefix(purpose) + base64.RawURLEncoding.EncodeToString(idBytes)
	imageData, hint, points, err := c.render(ctx)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(pointCaptchaAnswer{Purpose: purpose, SubjectDigest: subjectDigest, Points: points})
	if err != nil {
		return nil, fmt.Errorf("encode point captcha answer: %w", err)
	}
	if err := c.store.Put(ctx, id, string(encoded), c.ttl); err != nil {
		return nil, fmt.Errorf("store point captcha: %w", err)
	}
	return &PointCaptchaChallenge{
		CaptchaID: id, CaptchaImage: "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageData),
		HintText: hint, TargetCount: len(points), Width: c.width, Height: c.height,
		ExpiredAt: time.Now().Add(c.ttl).UnixMilli(),
	}, nil
}

func pointCaptchaIDPrefix(purpose string) string {
	if purpose == pointCaptchaPurposePasswordChange {
		return "password-change:"
	}
	return purpose + ":"
}

func validPointCaptchaID(value, purpose string) bool {
	return strings.HasPrefix(value, pointCaptchaIDPrefix(purpose))
}

func decodePointCaptchaAnswer(value string) (pointCaptchaAnswer, error) {
	var answer pointCaptchaAnswer
	if err := json.Unmarshal([]byte(value), &answer); err != nil ||
		(answer.Purpose != pointCaptchaPurposeLogin && answer.Purpose != pointCaptchaPurposePasswordChange) ||
		answer.SubjectDigest == "" || !validPointCaptchaTargetCount(len(answer.Points)) {
		return pointCaptchaAnswer{}, ErrPointCaptchaNotFound
	}
	return answer, nil
}

func validPointCaptchaTargetCount(count int) bool {
	return count >= pointCaptchaMinTargetCount && count <= pointCaptchaMaxTargetCount
}

func usernameDigest(username string) string {
	digest := sha256.Sum256([]byte(username))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

type memoryPointCaptcha struct {
	value     string
	expiredAt time.Time
}

type memoryPointCaptchaStore struct {
	mu     sync.Mutex
	values map[string]memoryPointCaptcha
}

func NewMemoryPointCaptchaStore() PointCaptchaStore {
	return &memoryPointCaptchaStore{values: make(map[string]memoryPointCaptcha)}
}

func (s *memoryPointCaptchaStore) Put(_ context.Context, id, value string, ttl time.Duration) error {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for currentID, captcha := range s.values {
		if !captcha.expiredAt.After(now) {
			delete(s.values, currentID)
		}
	}
	s.values[id] = memoryPointCaptcha{value: value, expiredAt: now.Add(ttl)}
	return nil
}

func (s *memoryPointCaptchaStore) Take(_ context.Context, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	captcha, ok := s.values[id]
	delete(s.values, id)
	if !ok || !captcha.expiredAt.After(time.Now()) {
		return "", ErrPointCaptchaNotFound
	}
	return captcha.value, nil
}

{{ if .Computed.enable_redis_final -}}
type redisPointCaptchaStore struct {
	client redisChallengeClient
}

func NewRedisPointCaptchaStore(client redisChallengeClient) PointCaptchaStore {
	return &redisPointCaptchaStore{client: client}
}

func (s *redisPointCaptchaStore) Put(ctx context.Context, id, value string, ttl time.Duration) error {
	return s.client.Set(ctx, "point-captcha:"+id, value, ttl).Err()
}

func (s *redisPointCaptchaStore) Take(ctx context.Context, id string) (string, error) {
	value, err := s.client.GetDel(ctx, "point-captcha:"+id).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrPointCaptchaNotFound
	}
	return value, err
}
{{ end -}}
