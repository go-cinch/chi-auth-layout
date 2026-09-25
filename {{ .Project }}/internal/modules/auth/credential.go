package auth

import (
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
{{- if .Computed.enable_redis_final }}
	"github.com/redis/go-redis/v9"
{{- end }}
)

const (
	credentialKeyAlgorithm     = jose.RSA_OAEP_256
	credentialContentAlgorithm = jose.A256GCM
	loginCredentialType        = "login+jwe"
	registerCredentialType     = "register+jwe"
	passwordCredentialType     = "password+jwe"
 passwordResetCredentialType = "password-reset+jwe"
	maxEncryptedCredential     = 8 << 10
	maxCredentialPlaintext     = 1 << 10
)

var (
	ErrInvalidCredential = apperror.New("AUTH_INVALID_CREDENTIAL", "invalid login credential")
	ErrInvalidRegistrationCredential = apperror.New("AUTH_INVALID_REGISTRATION_CREDENTIAL", "invalid registration credential")
	ErrInvalidPasswordCredential = apperror.New("AUTH_INVALID_PASSWORD_CREDENTIAL", "invalid password change credential")
	ErrChallengeNotFound = apperror.New("AUTH_CHALLENGE_NOT_FOUND", "credential challenge not found")
)

type PublicJWK struct {
	KeyType   string `json:"kty"`
	Use       string `json:"use"`
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

type CredentialChallenge struct {
	ChallengeID string    `json:"challenge_id"`
	KeyID       string    `json:"key_id"`
	PublicKey   PublicJWK `json:"public_key"`
	ExpiredAt   int64     `json:"expired_at"`
}

type EncryptedLoginInput struct {
	ChallengeID string `json:"challenge_id" example:"u7rUnVYJdWzvX41WeyuR1J6mM8Xr0jYjNQQHf9U5gEo"`
	Credential  string `json:"credential" example:"eyJhbGciOiJSU0EtT0FFUC0yNTYiLCJlbmMiOiJBMjU2R0NNIn0..."`
}

type EncryptedRegisterInput struct {
	ChallengeID string `json:"challenge_id" example:"u7rUnVYJdWzvX41WeyuR1J6mM8Xr0jYjNQQHf9U5gEo"`
	Credential  string `json:"credential" example:"eyJhbGciOiJSU0EtT0FFUC0yNTYiLCJlbmMiOiJBMjU2R0NNIn0..."`
}

type EncryptedPasswordResetInput struct {
 ChallengeID string `json:"challenge_id"`
 Credential string `json:"credential"`
}

type EncryptedPasswordChangeInput struct {
	ChallengeID string `json:"challenge_id" example:"u7rUnVYJdWzvX41WeyuR1J6mM8Xr0jYjNQQHf9U5gEo"`
	Credential  string `json:"credential" example:"eyJhbGciOiJSU0EtT0FFUC0yNTYiLCJlbmMiOiJBMjU2R0NNIn0..."`
}

type loginCredential struct {
	ChallengeID   string         `json:"challenge_id"`
	Username      string         `json:"username"`
	Password      string         `json:"password"`
	RememberMe    bool           `json:"remember_me"`
	CaptchaID     string         `json:"captcha_id,omitempty"`
	CaptchaPoints []CaptchaPoint `json:"captcha_points,omitempty"`
	SliderProof   string         `json:"slider_proof,omitempty"`
}

type registerCredential struct {
	ChallengeID string `json:"challenge_id"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	SliderProof string `json:"slider_proof,omitempty"`
}

type registrationPasswordCredential struct {
	ChallengeID string `json:"challenge_id"`
	Password    string `json:"password"`
}

type passwordChangeCredential struct {
	ChallengeID   string         `json:"challenge_id"`
	OldPassword   string         `json:"old_password"`
	NewPassword   string         `json:"new_password"`
	CaptchaID     string         `json:"captcha_id,omitempty"`
	CaptchaPoints []CaptchaPoint `json:"captcha_points,omitempty"`
}

type ChallengeStore interface {
	Put(context.Context, string, string, time.Duration) error
	Take(context.Context, string) (string, error)
}

type Credentials struct {
	keyID        string
	privateKey   *rsa.PrivateKey
	publicKey    PublicJWK
	ttl          time.Duration
	challenges   ChallengeStore
}

func NewCredentials(keyID, privateKeyFile string, ttl time.Duration, challenges ChallengeStore) (*Credentials, error) {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return nil, errors.New("challenge encryption key id is required")
	}
	if ttl <= 0 || ttl > 10*time.Minute {
		return nil, errors.New("credential challenge ttl must be between 1ns and 10m")
	}
	if challenges == nil {
		return nil, errors.New("credential challenge store is required")
	}
	privateKeyFile = strings.TrimSpace(privateKeyFile)
	var (
		privateKey *rsa.PrivateKey
		err        error
	)
	if privateKeyFile == "" {
		privateKey, err = rsa.GenerateKey(rand.Reader, 3072)
		if err == nil {
			slog.Warn("generated ephemeral challenge encryption key; configure auth.challengeEncryption.privateKeyFile for multi-instance deployments")
		}
	} else {
		privateKey, err = readRSAPrivateKey(privateKeyFile)
	}
	if err != nil {
		return nil, fmt.Errorf("load challenge encryption private key: %w", err)
	}
	return newCredentials(keyID, ttl, challenges, privateKey)
}

func newCredentials(keyID string, ttl time.Duration, challenges ChallengeStore, privateKey *rsa.PrivateKey) (*Credentials, error) {
	if privateKey == nil || privateKey.N == nil || privateKey.N.BitLen() < 2048 {
		return nil, errors.New("challenge encryption RSA key must be at least 2048 bits")
	}
	if err := privateKey.Validate(); err != nil {
		return nil, fmt.Errorf("validate challenge encryption RSA key: %w", err)
	}
	exponent := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
	return &Credentials{
		keyID:        keyID,
		privateKey:   privateKey,
		ttl:          ttl,
		challenges:   challenges,
		publicKey: PublicJWK{
			KeyType:   "RSA",
			Use:       "enc",
			Algorithm: string(credentialKeyAlgorithm),
			KeyID:     keyID,
			Modulus:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
			Exponent:  base64.RawURLEncoding.EncodeToString(exponent),
		},
	}, nil
}

func (c *Credentials) Challenge(ctx context.Context) (*CredentialChallenge, error) {
	return c.challenge(ctx, loginCredentialType, 0)
}

func (c *Credentials) RegistrationChallenge(ctx context.Context) (*CredentialChallenge, error) {
	return c.challenge(ctx, registerCredentialType, 0)
}

func (c *Credentials) PasswordChangeChallenge(ctx context.Context, userID int64) (*CredentialChallenge, error) {
	if userID <= 0 {
		return nil, ErrInvalidPasswordCredential
	}
	return c.challenge(ctx, passwordCredentialType, userID)
}

func (c *Credentials) challenge(ctx context.Context, credentialType string, userID int64) (*CredentialChallenge, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return nil, fmt.Errorf("generate credential challenge: %w", err)
	}
	id := base64.RawURLEncoding.EncodeToString(random)
	if err := c.challenges.Put(ctx, id, c.challengeValue(credentialType, userID), c.ttl); err != nil {
		return nil, fmt.Errorf("store credential challenge: %w", err)
	}
	return &CredentialChallenge{
		ChallengeID: id,
		KeyID:       c.keyID,
		PublicKey:   c.publicKey,
		ExpiredAt:   time.Now().Add(c.ttl).UnixMilli(),
	}, nil
}

func (c *Credentials) Open(ctx context.Context, input EncryptedLoginInput) (LoginInput, error) {
	challengeID := strings.TrimSpace(input.ChallengeID)
	plaintext, err := c.decrypt(challengeID, input.Credential, loginCredentialType, ErrInvalidCredential)
	if err != nil {
		return LoginInput{}, ErrInvalidCredential
	}
	payload, err := decodeLoginCredential(plaintext)
	if err != nil || subtle.ConstantTimeCompare([]byte(payload.ChallengeID), []byte(challengeID)) != 1 {
		return LoginInput{}, ErrInvalidCredential
	}
	if err := c.consumeChallenge(ctx, challengeID, loginCredentialType, 0, ErrInvalidCredential); err != nil {
		return LoginInput{}, err
	}
	return LoginInput{
		Username: payload.Username, Password: payload.Password, RememberMe: payload.RememberMe,
		CaptchaID: payload.CaptchaID, CaptchaPoints: payload.CaptchaPoints, SliderProof: payload.SliderProof,
	}, nil
}

func (c *Credentials) OpenRegistration(ctx context.Context, input EncryptedRegisterInput) (RegisterInput, error) {
	challengeID := strings.TrimSpace(input.ChallengeID)
	plaintext, err := c.decrypt(challengeID, input.Credential, registerCredentialType, ErrInvalidRegistrationCredential)
	if err != nil {
		return RegisterInput{}, err
	}
	payload, err := decodeRegisterCredential(plaintext)
	if err != nil || subtle.ConstantTimeCompare([]byte(payload.ChallengeID), []byte(challengeID)) != 1 {
		return RegisterInput{}, ErrInvalidRegistrationCredential
	}
	if err := c.consumeChallenge(ctx, challengeID, registerCredentialType, 0, ErrInvalidRegistrationCredential); err != nil {
		return RegisterInput{}, err
	}
	return RegisterInput{Username: payload.Username, Password: payload.Password, SliderProof: payload.SliderProof}, nil
}

func (c *Credentials) OpenRegistrationPassword(ctx context.Context, input EncryptedRegisterInput) (string, error) {
	challengeID := strings.TrimSpace(input.ChallengeID)
	plaintext, err := c.decrypt(challengeID, input.Credential, registerCredentialType, ErrInvalidRegistrationCredential)
	if err != nil {
		return "", err
	}
	payload, err := decodeRegistrationPasswordCredential(plaintext)
	if err != nil || subtle.ConstantTimeCompare([]byte(payload.ChallengeID), []byte(challengeID)) != 1 {
		return "", ErrInvalidRegistrationCredential
	}
	if err := c.consumeChallenge(ctx, challengeID, registerCredentialType, 0, ErrInvalidRegistrationCredential); err != nil {
		return "", err
	}
	return payload.Password, nil
}

func (c *Credentials) OpenPasswordChange(ctx context.Context, userID int64, input EncryptedPasswordChangeInput) (PasswordChangeInput, error) {
	if userID <= 0 {
		return PasswordChangeInput{}, ErrInvalidPasswordCredential
	}
	challengeID := strings.TrimSpace(input.ChallengeID)
	plaintext, err := c.decrypt(challengeID, input.Credential, passwordCredentialType, ErrInvalidPasswordCredential)
	if err != nil {
		return PasswordChangeInput{}, err
	}
	payload, err := decodePasswordChangeCredential(plaintext)
	if err != nil || subtle.ConstantTimeCompare([]byte(payload.ChallengeID), []byte(challengeID)) != 1 {
		return PasswordChangeInput{}, ErrInvalidPasswordCredential
	}
	if err := c.consumeChallenge(ctx, challengeID, passwordCredentialType, userID, ErrInvalidPasswordCredential); err != nil {
		return PasswordChangeInput{}, err
	}
	return PasswordChangeInput{
		OldPassword: payload.OldPassword, NewPassword: payload.NewPassword,
		CaptchaID: payload.CaptchaID, CaptchaPoints: payload.CaptchaPoints,
	}, nil
}

func (c *Credentials) decrypt(challengeID, credential, credentialType string, invalid error) ([]byte, error) {
	credential = strings.TrimSpace(credential)
	if challengeID == "" || len(challengeID) > 128 || credential == "" || len(credential) > maxEncryptedCredential {
		return nil, invalid
	}
	message, err := jose.ParseEncryptedCompact(
		credential,
		[]jose.KeyAlgorithm{credentialKeyAlgorithm},
		[]jose.ContentEncryption{credentialContentAlgorithm},
	)
	if err != nil || message.Header.KeyID != c.keyID || message.Header.Algorithm != string(credentialKeyAlgorithm) {
		return nil, invalid
	}
	typ, ok := message.Header.ExtraHeaders[jose.HeaderType].(string)
	if !ok || typ != credentialType {
		return nil, invalid
	}
	plaintext, err := message.Decrypt(c.privateKey)
	if err != nil || len(plaintext) == 0 || len(plaintext) > maxCredentialPlaintext {
		return nil, invalid
	}
	return plaintext, nil
}

func (c *Credentials) consumeChallenge(ctx context.Context, challengeID, credentialType string, userID int64, invalid error) error {
	value, err := c.challenges.Take(ctx, challengeID)
	if errors.Is(err, ErrChallengeNotFound) {
		return invalid
	}
	if err != nil {
		return fmt.Errorf("consume credential challenge: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(value), []byte(c.challengeValue(credentialType, userID))) != 1 {
		return invalid
	}
	return nil
}

func (c *Credentials) challengeValue(credentialType string, userID int64) string {
	return credentialType + "\x00" + c.keyID + "\x00" + fmt.Sprint(userID)
}

func decodeLoginCredential(plaintext []byte) (loginCredential, error) {
	var payload loginCredential
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return loginCredential{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return loginCredential{}, errors.New("login credential must contain one JSON object")
	}
	if payload.ChallengeID == "" || payload.Username == "" || payload.Password == "" {
		return loginCredential{}, errors.New("login credential fields are required")
	}
	if len(payload.CaptchaID) > 128 || len(payload.CaptchaPoints) > pointCaptchaMaxTargetCount {
		return loginCredential{}, errors.New("login captcha fields are invalid")
	}
	for _, point := range payload.CaptchaPoints {
		if point.X < 0 || point.X > 4096 || point.Y < 0 || point.Y > 4096 {
			return loginCredential{}, errors.New("login captcha point is invalid")
		}
	}
	return payload, nil
}

func decodeRegisterCredential(plaintext []byte) (registerCredential, error) {
	var payload registerCredential
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return registerCredential{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return registerCredential{}, errors.New("registration credential must contain one JSON object")
	}
	if payload.ChallengeID == "" || payload.Username == "" || payload.Password == "" {
		return registerCredential{}, errors.New("registration credential fields are required")
	}
	return payload, nil
}

func decodeRegistrationPasswordCredential(plaintext []byte) (registrationPasswordCredential, error) {
	var payload registrationPasswordCredential
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return registrationPasswordCredential{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return registrationPasswordCredential{}, errors.New("registration password credential must contain one JSON object")
	}
	if payload.ChallengeID == "" || payload.Password == "" {
		return registrationPasswordCredential{}, errors.New("registration password credential fields are required")
	}
	return payload, nil
}

func decodePasswordChangeCredential(plaintext []byte) (passwordChangeCredential, error) {
	var payload passwordChangeCredential
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return passwordChangeCredential{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return passwordChangeCredential{}, errors.New("password change credential must contain one JSON object")
	}
	if payload.ChallengeID == "" || payload.OldPassword == "" || payload.NewPassword == "" {
		return passwordChangeCredential{}, errors.New("password change credential fields are required")
	}
	if len(payload.CaptchaID) > 128 || len(payload.CaptchaPoints) > pointCaptchaMaxTargetCount {
		return passwordChangeCredential{}, errors.New("password change captcha fields are invalid")
	}
	for _, point := range payload.CaptchaPoints {
		if point.X < 0 || point.X > 4096 || point.Y < 0 || point.Y > 4096 {
			return passwordChangeCredential{}, errors.New("password change captcha point is invalid")
		}
	}
	return payload, nil
}

func readRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(content)
	if block == nil {
		return nil, errors.New("private key is not PEM encoded")
	}
	if value, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		privateKey, ok := value.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not RSA")
		}
		return privateKey, nil
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("private key must use PKCS#8 or PKCS#1 encoding")
	}
	return privateKey, nil
}

type memoryChallenge struct {
	keyID     string
	expiredAt time.Time
}

type memoryChallengeStore struct {
	mu     sync.Mutex
	values map[string]memoryChallenge
}

func NewMemoryChallengeStore() ChallengeStore {
	return &memoryChallengeStore{values: make(map[string]memoryChallenge)}
}

func (s *memoryChallengeStore) Put(_ context.Context, id, keyID string, ttl time.Duration) error {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for currentID, challenge := range s.values {
		if !challenge.expiredAt.After(now) {
			delete(s.values, currentID)
		}
	}
	s.values[id] = memoryChallenge{keyID: keyID, expiredAt: now.Add(ttl)}
	return nil
}

func (s *memoryChallengeStore) Take(_ context.Context, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	challenge, ok := s.values[id]
	delete(s.values, id)
	if !ok || !challenge.expiredAt.After(time.Now()) {
		return "", ErrChallengeNotFound
	}
	return challenge.keyID, nil
}

{{ if .Computed.enable_redis_final -}}
type redisChallengeClient interface {
	Set(context.Context, string, interface{}, time.Duration) *redis.StatusCmd
	GetDel(context.Context, string) *redis.StringCmd
}

type redisChallengeStore struct {
	client redisChallengeClient
}

func NewRedisChallengeStore(client redisChallengeClient) ChallengeStore {
	return &redisChallengeStore{client: client}
}

func (s *redisChallengeStore) Put(ctx context.Context, id, keyID string, ttl time.Duration) error {
	return s.client.Set(ctx, "credential-challenge:"+id, keyID, ttl).Err()
}

func (s *redisChallengeStore) Take(ctx context.Context, id string) (string, error) {
	keyID, err := s.client.GetDel(ctx, "credential-challenge:"+id).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrChallengeNotFound
	}
	return keyID, err
}
{{ end -}}

func (c *Credentials) PasswordResetChallenge(ctx context.Context, userID int64) (*CredentialChallenge, error) {
    if userID <= 0 { return nil, ErrInvalidPasswordCredential }
    return c.challenge(ctx, passwordResetCredentialType, userID)
}

func (c *Credentials) OpenPasswordReset(ctx context.Context, userID int64, input EncryptedPasswordResetInput) (PasswordResetInput, error) {
    if userID <= 0 { return PasswordResetInput{}, ErrInvalidPasswordCredential }
    challengeID := strings.TrimSpace(input.ChallengeID)
    plaintext, err := c.decrypt(challengeID, input.Credential, passwordResetCredentialType, ErrInvalidPasswordCredential)
    if err != nil { return PasswordResetInput{}, err }
    var payload struct {
        ChallengeID string `json:"challenge_id"`
        NewPassword string `json:"new_password"`
    }
    decoder := json.NewDecoder(bytes.NewReader(plaintext))
    decoder.DisallowUnknownFields()
    if decoder.Decode(&payload) != nil || decoder.Decode(new(any)) != io.EOF || payload.NewPassword == "" || subtle.ConstantTimeCompare([]byte(payload.ChallengeID), []byte(challengeID)) != 1 {
        return PasswordResetInput{}, ErrInvalidPasswordCredential
    }
    if err := c.consumeChallenge(ctx, challengeID, passwordResetCredentialType, userID, ErrInvalidPasswordCredential); err != nil {
        return PasswordResetInput{}, err
    }
    return PasswordResetInput{NewPassword: payload.NewPassword}, nil
}
