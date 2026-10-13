package auditctx

import (
	"context"
	"testing"
)

func TestMetadata(t *testing.T) {
	Identity(context.Background(), 1, "ignored")
	Error(context.Background(), "ignored")
	ctx, s := WithState(context.Background())
	Identity(ctx, 12, "alice")
	Error(ctx, "AUTH_LOGIN_FAILED")
	e := s.Snapshot()
	if e.UserID != 12 || e.Username != "alice" || e.ErrorCode != "AUTH_LOGIN_FAILED" {
		t.Fatal(e)
	}
}
