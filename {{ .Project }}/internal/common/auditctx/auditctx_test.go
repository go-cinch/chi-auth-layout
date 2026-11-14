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

func TestLinkResourceDoesNotSetActor(t *testing.T) {
	ResourceOperation(context.Background(), "confirm_email_unlink", "102")
	ctx, state := WithState(t.Context())
	ResourceOperation(ctx, "view_email_unlink_result", "102")
	event := state.Snapshot()
	if event.Operation != "view_email_unlink_result" || event.ResourceID != "102" || event.UserID != 0 || event.Username != "" {
		t.Fatal(event)
	}
}
