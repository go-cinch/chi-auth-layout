package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"{{ .Computed.module_name_final }}/internal/common/config"
)

func TestUnary(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	cfg := &config.Config{}
	cfg.GRPC.Timeout = time.Millisecond
	ctx := peer.NewContext(t.Context(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1234}})
	method := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Call"}
	for _, test := range []struct {
		name    string
		code    codes.Code
		handler grpc.UnaryHandler
	}{
		{"ok", codes.OK, func(context.Context, any) (any, error) { return "ok", nil }},
		{"timeout", codes.DeadlineExceeded, func(ctx context.Context, _ any) (any, error) { <-ctx.Done(); return nil, nil }},
		{"canceled", codes.Canceled, func(context.Context, any) (any, error) { return nil, context.Canceled }},
		{"panic", codes.Internal, func(context.Context, any) (any, error) { panic("boom") }},
		{"application error", codes.NotFound, func(context.Context, any) (any, error) { return nil, status.Error(codes.NotFound, "missing") }},
	} {
		output.Reset()
		_, err := Unary(cfg)(ctx, "secret-request", method, test.handler)
		if status.Code(err) != test.code {
			t.Fatalf("%s: %v", test.name, err)
		}
		if !strings.Contains(output.String(), method.FullMethod+" "+test.code.String()) || strings.Contains(output.String(), "secret-request") || !strings.Contains(output.String(), "127.0.0.1:1234") {
			t.Fatalf("log: %s", output.String())
		}
		entries := readRPCLogs(t, &output)
		entry := entries[len(entries)-1]
		duration := assertRPCLogFields(t, entry, method.FullMethod, test.code)
		if entry["msg"] != fmt.Sprintf("%s %s %dms", method.FullMethod, test.code, duration) || entry["remote_addr"] != "127.0.0.1:1234" {
			t.Fatalf("access log: %#v", entry)
		}
		if test.name == "panic" {
			if len(entries) != 2 || entries[0]["error"] != "boom" || entries[0]["msg"] != "panic recovered: boom" || entries[0]["method"] != method.FullMethod || entries[0]["operation"] != "recover_panic" || entries[0]["outcome"] != "failure" {
				t.Fatalf("panic log: %#v", entries)
			}
			if stack, ok := entries[0]["stack"].(string); !ok || stack == "" {
				t.Fatalf("panic stack: %#v", entries[0])
			}
		} else if len(entries) != 1 {
			t.Fatalf("unexpected logs: %#v", entries)
		}
	}
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeStream) Context() context.Context { return s.ctx }

func TestStream(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	cfg := &config.Config{}
	cfg.GRPC.Timeout = time.Millisecond
	expected := errors.New("handler error")
	err := Stream(cfg)(nil, &fakeStream{ctx: t.Context()}, &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}, func(_ any, stream grpc.ServerStream) error {
		if _, ok := stream.Context().Deadline(); ok { // t.Context normally has no deadline, compare instead of assuming.
			original, originalOK := t.Context().Deadline()
			actual, _ := stream.Context().Deadline()
			if !originalOK || !actual.Equal(original) {
				t.Error("unary deadline applied to stream")
			}
		}
		return expected
	})
	if !errors.Is(err, expected) {
		t.Fatal(err)
	}
	entries := readRPCLogs(t, &output)
	if len(entries) != 1 {
		t.Fatalf("logs: %#v", entries)
	}
	duration := assertRPCLogFields(t, entries[0], "/test.Service/Stream", codes.Unknown)
	if entries[0]["msg"] != fmt.Sprintf("/test.Service/Stream Unknown %dms", duration) || entries[0]["remote_addr"] != "" {
		t.Fatalf("stream log: %#v", entries[0])
	}
}

func readRPCLogs(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(output)
	decoder.UseNumber()
	var entries []map[string]any
	for {
		var entry map[string]any
		if err := decoder.Decode(&entry); err == io.EOF {
			return entries
		} else if err != nil {
			t.Fatalf("invalid JSON log: %v", err)
		}
		entries = append(entries, entry)
	}
}

func assertRPCLogFields(t *testing.T, entry map[string]any, method string, code codes.Code) int64 {
	t.Helper()
	if entry["method"] != method || entry["grpc_code"] != json.Number(fmt.Sprint(int(code))) || entry["grpc_status"] != code.String() {
		t.Fatalf("rpc fields: %#v", entry)
	}
	duration, ok := entry["duration_ms"].(json.Number)
	if !ok {
		t.Fatalf("duration is not a number: %#v", entry)
	}
	milliseconds, err := duration.Int64()
	if err != nil || milliseconds < 0 {
		t.Fatalf("invalid duration: %#v", entry)
	}
	return milliseconds
}
