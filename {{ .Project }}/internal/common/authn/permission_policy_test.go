package authn

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
)

type deniedPermissionPolicy struct{}

func (deniedPermissionPolicy) RequireAllPermissions(context.Context) error {
	return ErrPermissionAdministrationRequired
}
func (deniedPermissionPolicy) RequirePermissionGrant(context.Context) error {
	return ErrPermissionGrantRequired
}
func (deniedPermissionPolicy) LockPermissionMutation(context.Context) error {
	return ErrPermissionAdministrationRequired
}

func TestPermissionPoliciesFailClosedForHTTP(t *testing.T) {
	request, err := ScopeRequest(httptest.NewRequest("PATCH", "/user/1", nil))
	if err != nil {
		t.Fatal(err)
	}
	for name, check := range map[string]func(context.Context) error{
		"all":   func(ctx context.Context) error { return RequireAllPermissions(ctx, nil) },
		"grant": func(ctx context.Context) error { return RequirePermissionGrant(ctx, nil) },
		"lock":  func(ctx context.Context) error { return LockPermissionMutation(ctx, nil) },
	} {
		if err := check(request.Context()); err == nil {
			t.Fatalf("%s bypassed HTTP with unwired policy", name)
		}
		if err := check(t.Context()); err != nil {
			t.Fatalf("internal construction: %v", err)
		}
	}
	policy := deniedPermissionPolicy{}
	if !errors.Is(RequireAllPermissions(t.Context(), policy), ErrPermissionAdministrationRequired) || !errors.Is(RequirePermissionGrant(t.Context(), policy), ErrPermissionGrantRequired) || !errors.Is(LockPermissionMutation(t.Context(), policy), ErrPermissionAdministrationRequired) {
		t.Fatal("installed policy was bypassed for internal context")
	}
}
