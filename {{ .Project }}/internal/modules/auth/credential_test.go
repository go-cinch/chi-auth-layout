package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
{{- if .Computed.enable_redis_final }}
	"context"
	"strconv"
	"github.com/redis/go-redis/v9"
{{- end }}
)

var (
	testLoginKeyOnce sync.Once
	testLoginKey     *rsa.PrivateKey
	testLoginKeyErr  error
)

func loginTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	testLoginKeyOnce.Do(func() {
		testLoginKey, testLoginKeyErr = rsa.GenerateKey(rand.Reader, 2048)
	})
	if testLoginKeyErr != nil {
		t.Fatal(testLoginKeyErr)
	}
	return testLoginKey
}

func newCredentialTestManager(t *testing.T, ttl time.Duration) *Credentials {
	t.Helper()
	credentials, err := newCredentials("test-login-key", ttl, NewMemoryChallengeStore(), loginTestKey(t))
	if err != nil {
		t.Fatal(err)
	}
	return credentials
}

func encryptLoginCredential(t *testing.T, credentials *Credentials, payload loginCredential, withType bool) string {
	t.Helper()
	options := &jose.EncrypterOptions{}
	if withType {
		options.WithType(jose.ContentType(loginCredentialType))
	}
	encrypter, err := jose.NewEncrypter(
		credentialContentAlgorithm,
		jose.Recipient{Algorithm: credentialKeyAlgorithm, Key: &credentials.privateKey.PublicKey, KeyID: credentials.keyID},
		options,
	)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	object, err := encrypter.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	value, err := object.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func encryptedLoginInput(t *testing.T, credentials *Credentials, username, password string, remembered ...bool) EncryptedLoginInput {
	return encryptedLoginInputWithProof(t, credentials, username, password, "", remembered...)
}

func encryptedLoginInputWithProof(t *testing.T, credentials *Credentials, username, password, sliderProof string, remembered ...bool) EncryptedLoginInput {
	t.Helper()
	challenge, err := credentials.Challenge(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rememberMe := len(remembered) == 1 && remembered[0]
	credential := encryptLoginCredential(t, credentials, loginCredential{
		ChallengeID: challenge.ChallengeID,
		Username:    username,
		Password:    password,
		RememberMe:  rememberMe,
		SliderProof: sliderProof,
	}, true)
	return EncryptedLoginInput{ChallengeID: challenge.ChallengeID, Credential: credential}
}

func encryptRegisterCredential(t *testing.T, credentials *Credentials, payload any, withType bool) string {
	t.Helper()
	options := &jose.EncrypterOptions{}
	if withType {
		options.WithType(jose.ContentType(registerCredentialType))
	}
	encrypter, err := jose.NewEncrypter(
		credentialContentAlgorithm,
		jose.Recipient{Algorithm: credentialKeyAlgorithm, Key: &credentials.privateKey.PublicKey, KeyID: credentials.keyID},
		options,
	)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	object, err := encrypter.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	value, err := object.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func encryptedRegisterInput(t *testing.T, credentials *Credentials, username, password string) EncryptedRegisterInput {
	return encryptedRegisterInputWithProof(t, credentials, username, password, "")
}

func encryptedRegisterInputWithProof(t *testing.T, credentials *Credentials, username, password, sliderProof string) EncryptedRegisterInput {
	t.Helper()
	challenge, err := credentials.RegistrationChallenge(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	credential := encryptRegisterCredential(t, credentials, registerCredential{
		ChallengeID: challenge.ChallengeID,
		Username:    username,
		Password:    password,
		SliderProof: sliderProof,
	}, true)
	return EncryptedRegisterInput{ChallengeID: challenge.ChallengeID, Credential: credential}
}

func encryptedRegistrationPasswordInput(t *testing.T, credentials *Credentials, password string) EncryptedRegisterInput {
	t.Helper()
	challenge, err := credentials.RegistrationChallenge(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	credential := encryptRegisterCredential(t, credentials, registrationPasswordCredential{
		ChallengeID: challenge.ChallengeID,
		Password:    password,
	}, true)
	return EncryptedRegisterInput{ChallengeID: challenge.ChallengeID, Credential: credential}
}

func encryptedPasswordChangeInput(t *testing.T, credentials *Credentials, userID int64, oldPassword, newPassword string, captcha ...PasswordChangeInput) EncryptedPasswordChangeInput {
	t.Helper()
	challenge, err := credentials.PasswordChangeChallenge(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	options := (&jose.EncrypterOptions{}).WithType(jose.ContentType(passwordCredentialType))
	encrypter, err := jose.NewEncrypter(
		credentialContentAlgorithm,
		jose.Recipient{Algorithm: credentialKeyAlgorithm, Key: &credentials.privateKey.PublicKey, KeyID: credentials.keyID},
		options,
	)
	if err != nil {
		t.Fatal(err)
	}
	payload := passwordChangeCredential{
		ChallengeID: challenge.ChallengeID,
		OldPassword: oldPassword,
		NewPassword: newPassword,
	}
	if len(captcha) == 1 {
		payload.CaptchaID = captcha[0].CaptchaID
		payload.CaptchaPoints = captcha[0].CaptchaPoints
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	object, err := encrypter.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := object.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return EncryptedPasswordChangeInput{ChallengeID: challenge.ChallengeID, Credential: credential}
}

func TestCredentialsRoundTripAndReplay(t *testing.T) {
	credentials := newCredentialTestManager(t, time.Minute)
	challenge, err := credentials.Challenge(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if challenge.KeyID != credentials.keyID || challenge.ExpiredAt <= time.Now().UnixMilli() || challenge.PublicKey.Modulus == "" || challenge.PublicKey.Exponent != "AQAB" {
		t.Fatalf("challenge = %#v", challenge)
	}
	input := EncryptedLoginInput{
		ChallengeID: challenge.ChallengeID,
		Credential: encryptLoginCredential(t, credentials, loginCredential{
			ChallengeID:   challenge.ChallengeID,
			Username:      "readonly",
			Password:      "cinch123",
			CaptchaID:     "captcha-1",
			CaptchaPoints: []CaptchaPoint{CaptchaPoint{X: 42, Y: 84}},
		}, true),
	}
	plain, err := credentials.Open(t.Context(), input)
	if err != nil || plain.Username != "readonly" || plain.Password != "cinch123" || plain.CaptchaID != "captcha-1" || len(plain.CaptchaPoints) != 1 {
		t.Fatalf("open = %#v, %v", plain, err)
	}
	if _, err := credentials.Open(t.Context(), input); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("replay error = %v", err)
	}
}

func TestRegistrationCredentialsRoundTripAndPurpose(t *testing.T) {
	credentials := newCredentialTestManager(t, time.Minute)
	input := encryptedRegisterInput(t, credentials, "new-user", "secret1")
	plain, err := credentials.OpenRegistration(t.Context(), input)
	if err != nil || plain.Username != "new-user" || plain.Password != "secret1" {
		t.Fatalf("open registration = %#v, %v", plain, err)
	}
	if _, err := credentials.OpenRegistration(t.Context(), input); !errors.Is(err, ErrInvalidRegistrationCredential) {
		t.Fatalf("registration replay error = %v", err)
	}

	passwordInput := encryptedRegistrationPasswordInput(t, credentials, "secret1")
	password, err := credentials.OpenRegistrationPassword(t.Context(), passwordInput)
	if err != nil || password != "secret1" {
		t.Fatalf("open registration password = %q, %v", password, err)
	}
	if _, err := credentials.OpenRegistrationPassword(t.Context(), passwordInput); !errors.Is(err, ErrInvalidRegistrationCredential) {
		t.Fatalf("registration password replay error = %v", err)
	}

	loginChallenge, err := credentials.Challenge(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	wrongPurpose := EncryptedRegisterInput{
		ChallengeID: loginChallenge.ChallengeID,
		Credential: encryptRegisterCredential(t, credentials, registerCredential{
			ChallengeID: loginChallenge.ChallengeID,
			Username:    "new-user",
			Password:    "secret1",
		}, true),
	}
	if _, err := credentials.OpenRegistration(t.Context(), wrongPurpose); !errors.Is(err, ErrInvalidRegistrationCredential) {
		t.Fatalf("cross-purpose challenge error = %v", err)
	}
}

func TestPasswordChangeCredentialsRoundTripReplayAndUserBinding(t *testing.T) {
	credentials := newCredentialTestManager(t, time.Minute)
	input := encryptedPasswordChangeInput(t, credentials, 7, "current-password", "new-password", PasswordChangeInput{
		CaptchaID: "captcha-id", CaptchaPoints: []CaptchaPoint{CaptchaPoint{X: 42, Y: 84}},
	})
	plain, err := credentials.OpenPasswordChange(t.Context(), 7, input)
	if err != nil || plain.OldPassword != "current-password" || plain.NewPassword != "new-password" || plain.CaptchaID != "captcha-id" || len(plain.CaptchaPoints) != 1 {
		t.Fatalf("open password change = %#v, %v", plain, err)
	}
	if _, err := credentials.OpenPasswordChange(t.Context(), 7, input); !errors.Is(err, ErrInvalidPasswordCredential) {
		t.Fatalf("password change replay error = %v", err)
	}

	otherUserInput := encryptedPasswordChangeInput(t, credentials, 7, "current-password", "new-password")
	if _, err := credentials.OpenPasswordChange(t.Context(), 8, otherUserInput); !errors.Is(err, ErrInvalidPasswordCredential) {
		t.Fatalf("cross-user password change error = %v", err)
	}
	if _, err := credentials.PasswordChangeChallenge(t.Context(), 0); !errors.Is(err, ErrInvalidPasswordCredential) {
		t.Fatalf("invalid password change challenge error = %v", err)
	}
}

func TestCredentialsRejectInvalidInput(t *testing.T) {
	credentials := newCredentialTestManager(t, time.Minute)
	for _, input := range []EncryptedLoginInput{
		{},
		{ChallengeID: "missing", Credential: "invalid"},
		{ChallengeID: string(make([]byte, 129)), Credential: "invalid"},
		{ChallengeID: "missing", Credential: string(make([]byte, maxEncryptedCredential+1))},
	} {
		if _, err := credentials.Open(t.Context(), input); !errors.Is(err, ErrInvalidCredential) {
			t.Fatalf("input %#v error = %v", input, err)
		}
	}

	input := encryptedLoginInput(t, credentials, "readonly", "cinch123")
	input.ChallengeID = "different"
	if _, err := credentials.Open(t.Context(), input); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("mismatched challenge error = %v", err)
	}

	challenge, err := credentials.Challenge(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	withoutType := encryptLoginCredential(t, credentials, loginCredential{ChallengeID: challenge.ChallengeID, Username: "readonly", Password: "cinch123"}, false)
	if _, err := credentials.Open(t.Context(), EncryptedLoginInput{ChallengeID: challenge.ChallengeID, Credential: withoutType}); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("missing type error = %v", err)
	}
}

func TestRegistrationCredentialsRejectInvalidInput(t *testing.T) {
	credentials := newCredentialTestManager(t, time.Minute)
	for _, input := range []EncryptedRegisterInput{
		{},
		{ChallengeID: "missing", Credential: "invalid"},
		{ChallengeID: string(make([]byte, 129)), Credential: "invalid"},
		{ChallengeID: "missing", Credential: string(make([]byte, maxEncryptedCredential+1))},
	} {
		if _, err := credentials.OpenRegistration(t.Context(), input); !errors.Is(err, ErrInvalidRegistrationCredential) {
			t.Fatalf("input %#v error = %v", input, err)
		}
	}

	challenge, err := credentials.RegistrationChallenge(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	withoutType := encryptRegisterCredential(t, credentials, registerCredential{
		ChallengeID: challenge.ChallengeID,
		Username:    "new-user",
		Password:    "secret1",
	}, false)
	if _, err := credentials.OpenRegistration(t.Context(), EncryptedRegisterInput{ChallengeID: challenge.ChallengeID, Credential: withoutType}); !errors.Is(err, ErrInvalidRegistrationCredential) {
		t.Fatalf("missing registration type error = %v", err)
	}
}

func TestPasswordChangeCredentialsRejectInvalidInput(t *testing.T) {
	credentials := newCredentialTestManager(t, time.Minute)
	for _, input := range []EncryptedPasswordChangeInput{
		{},
		{ChallengeID: "missing", Credential: "invalid"},
		{ChallengeID: string(make([]byte, 129)), Credential: "invalid"},
		{ChallengeID: "missing", Credential: string(make([]byte, maxEncryptedCredential+1))},
	} {
		if _, err := credentials.OpenPasswordChange(t.Context(), 7, input); !errors.Is(err, ErrInvalidPasswordCredential) {
			t.Fatalf("input %#v error = %v", input, err)
		}
	}
	if _, err := credentials.OpenPasswordChange(t.Context(), 0, EncryptedPasswordChangeInput{}); !errors.Is(err, ErrInvalidPasswordCredential) {
		t.Fatalf("invalid user password change error = %v", err)
	}
}

func TestMemoryChallengeStore(t *testing.T) {
	store := NewMemoryChallengeStore()
	if err := store.Put(t.Context(), "valid", "key-1", time.Minute); err != nil {
		t.Fatal(err)
	}
	if keyID, err := store.Take(t.Context(), "valid"); err != nil || keyID != "key-1" {
		t.Fatalf("take = %q, %v", keyID, err)
	}
	if _, err := store.Take(t.Context(), "valid"); !errors.Is(err, ErrChallengeNotFound) {
		t.Fatalf("second take error = %v", err)
	}
	if err := store.Put(t.Context(), "expired", "key-1", -time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Take(t.Context(), "expired"); !errors.Is(err, ErrChallengeNotFound) {
		t.Fatalf("expired take error = %v", err)
	}
}

func TestNewCredentialsAndPrivateKeyFiles(t *testing.T) {
	if _, err := NewCredentials("", "", time.Minute, NewMemoryChallengeStore()); err == nil {
		t.Fatal("empty key id was accepted")
	}
	if _, err := NewCredentials("key", "", 0, NewMemoryChallengeStore()); err == nil {
		t.Fatal("zero ttl was accepted")
	}
	if _, err := NewCredentials("key", "", 11*time.Minute, NewMemoryChallengeStore()); err == nil {
		t.Fatal("long ttl was accepted")
	}
	if _, err := NewCredentials("key", "", time.Minute, nil); err == nil {
		t.Fatal("nil challenge store was accepted")
	}
	if _, err := newCredentials("key", time.Minute, NewMemoryChallengeStore(), nil); err == nil {
		t.Fatal("nil private key was accepted")
	}
	if _, err := NewCredentials("key", filepath.Join(t.TempDir(), "missing.pem"), time.Minute, NewMemoryChallengeStore()); err == nil {
		t.Fatal("missing private key was accepted")
	}

	privateKey := loginTestKey(t)
	for name, content := range map[string][]byte{
		"pkcs1.pem": pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}),
		"pkcs8.pem": func() []byte {
			encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
			if err != nil {
				t.Fatal(err)
			}
			return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})
		}(),
	} {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		credentials, err := NewCredentials("key", path, time.Minute, NewMemoryChallengeStore())
		if err != nil || credentials.publicKey.Modulus == "" {
			t.Fatalf("load %s: %#v, %v", name, credentials, err)
		}
	}
}

{{ if .Computed.enable_redis_final -}}
type fakeRedisChallengeClient struct {
	values map[string]string
	err    error
}

func (f *fakeRedisChallengeClient) Set(ctx context.Context, key string, value interface{}, _ time.Duration) *redis.StatusCmd {
	command := redis.NewStatusCmd(ctx)
	if f.err != nil {
		command.SetErr(f.err)
		return command
	}
	f.values[key] = value.(string)
	command.SetVal("OK")
	return command
}

func (f *fakeRedisChallengeClient) GetDel(ctx context.Context, key string) *redis.StringCmd {
	command := redis.NewStringCmd(ctx)
	if f.err != nil {
		command.SetErr(f.err)
		return command
	}
	value, ok := f.values[key]
	delete(f.values, key)
	if !ok {
		command.SetErr(redis.Nil)
		return command
	}
	command.SetVal(value)
	return command
}

func (f *fakeRedisChallengeClient) Get(ctx context.Context, key string) *redis.StringCmd {
	command := redis.NewStringCmd(ctx)
	if f.err != nil {
		command.SetErr(f.err)
		return command
	}
	value, ok := f.values[key]
	if !ok {
		command.SetErr(redis.Nil)
		return command
	}
	command.SetVal(value)
	return command
}

func (f *fakeRedisChallengeClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	command := redis.NewIntCmd(ctx)
	if f.err != nil {
		command.SetErr(f.err)
		return command
	}
	var count int64
	for _, key := range keys {
		if _, ok := f.values[key]; ok {
			delete(f.values, key)
			count++
		}
	}
	command.SetVal(count)
	return command
}

func TestRedisChallengeStore(t *testing.T) {
	client := &fakeRedisChallengeClient{values: make(map[string]string)}
	store := NewRedisChallengeStore(client)
	if err := store.Put(t.Context(), "abc", "key-1", time.Minute); err != nil {
		t.Fatal(err)
	}
	if keyID, err := store.Take(t.Context(), "abc"); err != nil || keyID != "key-1" {
		t.Fatalf("take = %q, %v", keyID, err)
	}
	if _, err := store.Take(t.Context(), "abc"); !errors.Is(err, ErrChallengeNotFound) {
		t.Fatalf("missing error = %v", err)
	}
	client.err = errors.New("redis unavailable")
	if err := store.Put(t.Context(), "def", "key-1", time.Minute); err == nil {
		t.Fatal("put error was ignored")
	}
	if _, err := store.Take(t.Context(), "def"); err == nil {
		t.Fatal("take error was ignored")
	}
}

func (f *fakeRedisChallengeClient) Eval(ctx context.Context, _ string, keys []string, _ ...interface{}) *redis.Cmd {
	command := redis.NewCmd(ctx)
	if f.err != nil {
		command.SetErr(f.err)
		return command
	}
	count, _ := strconv.ParseInt(f.values[keys[0]], 10, 64)
	count++
	f.values[keys[0]] = strconv.FormatInt(count, 10)
	command.SetVal(count)
	return command
}
{{ end -}}
