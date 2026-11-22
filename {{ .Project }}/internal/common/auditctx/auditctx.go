// Package auditctx carries allowlisted audit metadata across HTTP middleware.
package auditctx

import (
	"context"
	"encoding/json"
	"sync"
)

// Target is supplied by the business capability, not inferred by server infrastructure.
type Target struct {
	OmitPayload  bool
	Kind         string
	Module       string
	Operation    string
	ResourceType string
	ResourceID   string
}

type Event struct {
	// A login request may also perform a separately auditable operation on the account.
	AdditionalUserOperation string `json:"-"`
	StepUpFingerprint string          `json:"step_up_fingerprint"`
	StepUpVerifiedAt  int64           `json:"step_up_verified_at"`
	Header         json.RawMessage `json:"header"`
	Operation      string          `json:"operation"`
	ResourceType   string          `json:"resource_type"`
	ResourceID     string          `json:"resource_id"`
	RequestParams  json.RawMessage `json:"request_params"`
	ResponseParams json.RawMessage `json:"response_params"`
	Kind           string          `json:"kind"`
	UserID         int64           `json:"user_id"`
	Username       string          `json:"username"`
	Module         string          `json:"module"`
	Method         string          `json:"method"`
	Path           string          `json:"path"`
	IP             string          `json:"ip"`
	UserAgent      string          `json:"user_agent"`
	TraceID        string          `json:"trace_id"`
	ErrorCode      string          `json:"error_code"`
	Status         int             `json:"status"`
	DurationMS     int64           `json:"duration_ms"`
	OccurredAt     int64           `json:"occurred_at"`
}

type key struct{}
type State struct {
	mu    sync.Mutex
	event Event
}

func WithState(ctx context.Context) (context.Context, *State) {
	s := &State{}
	return context.WithValue(ctx, key{}, s), s
}

func Identity(ctx context.Context, id int64, username string) {
	if s, ok := ctx.Value(key{}).(*State); ok {
		s.mu.Lock()
		s.event.UserID = id
		s.event.Username = username
		s.mu.Unlock()
	}
}

func AlsoRecordUserOperation(ctx context.Context, operation string) {
	if s, ok := ctx.Value(key{}).(*State); ok {
		s.mu.Lock()
		s.event.AdditionalUserOperation = operation
		s.mu.Unlock()
	}
}

// ResourceOperation records the verified subject of an operation such as an
// email link. It does not identify the link holder as a signed-in actor.
func ResourceOperation(ctx context.Context, operation, resourceID string) {
	if s, ok := ctx.Value(key{}).(*State); ok {
		s.mu.Lock()
		s.event.Operation = operation
		s.event.ResourceID = resourceID
		s.mu.Unlock()
	}
}

func Error(ctx context.Context, code string) {
	if s, ok := ctx.Value(key{}).(*State); ok {
		s.mu.Lock()
		s.event.ErrorCode = code
		s.mu.Unlock()
	}
}

func (s *State) Snapshot() Event { s.mu.Lock(); defer s.mu.Unlock(); return s.event }

// StepUp records a server-generated verification reference, never client data.
func StepUp(ctx context.Context, fingerprint string, verifiedAt int64) {
	if s, ok := ctx.Value(key{}).(*State); ok {
		s.mu.Lock()
		s.event.StepUpFingerprint = fingerprint
		s.event.StepUpVerifiedAt = verifiedAt
		s.mu.Unlock()
	}
}
