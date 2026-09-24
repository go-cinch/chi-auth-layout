package apperror_test

import (
	"errors"
	"fmt"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
)

func TestPublicErrors(t *testing.T) {
	expected := apperror.New("USER_NOT_FOUND", "user not found")
	wrapped := fmt.Errorf("lookup: %w", expected)
	if !errors.Is(wrapped, expected) || apperror.Code(wrapped) != "USER_NOT_FOUND" || apperror.Public(wrapped).Error() != "user not found" {
		t.Fatal("wrapped error lost identity")
	}
	for _, err := range []error{nil, errors.New("database secret"), (*apperror.Error)(nil)} {
		if apperror.Public(err) != apperror.Internal {
			t.Fatal("unexpected error exposed")
		}
	}
}
