package authn_test

import (
	"net/http/httptest"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestManager(t *testing.T) {
	manager, err := authn.New("test-service", testKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	want := authn.Identity{Methods: authn.Methods{Primary: authn.Password}, CredentialVersion: 1, UserID: 7, Username: "operator", Code: "ABCDEFGH"}
	token, expiredAt, err := manager.Issue(want)
	if err != nil || token == "" || !expiredAt.After(time.Now()) {
		t.Fatalf("issue: %q %v %v", token, expiredAt, err)
	}
	secondToken, _, err := manager.Issue(want)
	if err != nil || secondToken == token {
		t.Fatalf("second issue returned duplicate token: %v", err)
	}
	got, err := manager.Parse(token)
	if err != nil || got != want {
		t.Fatalf("parse: %#v %v", got, err)
	}
	request := httptest.NewRequest("GET", "/user", nil)
	request.Header.Set("Authorization", "bearer "+token)
	request, err = manager.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := authn.FromContext(request.Context())
	if !ok || got != want {
		t.Fatalf("context identity: %#v %v", got, ok)
	}
}

func TestManagerRejectsInvalidValues(t *testing.T) {
	for _, test := range []struct {
		issuer string
		key    string
		ttl    time.Duration
	}{
		{"", testKey, time.Hour},
		{"test", "short", time.Hour},
		{"test", testKey, 0},
	} {
		if _, err := authn.New(test.issuer, test.key, test.ttl); err == nil {
			t.Fatalf("accepted config: %#v", test)
		}
	}
	manager, err := authn.New("test", testKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authn.New("test", testKey, time.Hour, nil); err == nil {
		t.Fatal("nil session validator was accepted")
	}
	if _, _, err := manager.Issue(authn.Identity{}); err == nil {
		t.Fatal("accepted empty identity")
	}
	for _, header := range []string{"", "Basic abc", "Bearer", "Bearer invalid", "Bearer one two"} {
		request := httptest.NewRequest("GET", "/user", nil)
		request.Header.Set("Authorization", header)
		if _, err := manager.AuthenticateRequest(request); err == nil {
			t.Fatalf("accepted authorization header %q", header)
		}
	}
	other, err := authn.New("other", testKey, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := other.Issue(authn.Identity{Methods: authn.Methods{Primary: authn.Password}, CredentialVersion: 1, UserID: 1, Username: "user", Code: "CODE0001"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Parse(token); err == nil {
		t.Fatal("accepted token from another issuer")
	}
}
