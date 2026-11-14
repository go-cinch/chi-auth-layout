package action

import "context"

type permissionMutationKey struct{}

// WithPermissionMutation starts fresh metadata for one business write transaction.
func WithPermissionMutation(ctx context.Context) context.Context {
	return context.WithValue(ctx, permissionMutationKey{}, new(bool))
}

func MarkPermissionMutation(ctx context.Context) {
	if related, ok := ctx.Value(permissionMutationKey{}).(*bool); ok {
		*related = true
	}
}

func IsPermissionMutation(ctx context.Context) bool {
	related, _ := ctx.Value(permissionMutationKey{}).(*bool)
	return related != nil && *related
}
