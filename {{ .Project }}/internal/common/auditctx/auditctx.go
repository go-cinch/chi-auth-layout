// Package auditctx carries allowlisted audit metadata across HTTP middleware.
package auditctx

import (
	"context"
	"encoding/json"
	"sync"
)

// Target is supplied by the business capability, not inferred by server infrastructure.
type Target struct {
	Kind         string
	Module       string
	Operation    string
	ResourceType string
	ResourceID   string
}

type Event struct {
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

func Error(ctx context.Context, code string) {
	if s, ok := ctx.Value(key{}).(*State); ok {
		s.mu.Lock()
		s.event.ErrorCode = code
		s.mu.Unlock()
	}
}

func (s *State) Snapshot() Event { s.mu.Lock(); defer s.mu.Unlock(); return s.event }
