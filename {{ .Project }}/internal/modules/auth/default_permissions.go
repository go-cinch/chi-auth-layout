package auth

import "context"

// The action module resolves only creation-time grants in the caller's transaction.
type DefaultUserPermissions interface {
	DefaultUserCodes(context.Context, []string) ([]string, error)
}
