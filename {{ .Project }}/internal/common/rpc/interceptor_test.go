package rpc

import (
	"bytes"
	"context"
	"errors"
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
	}
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeStream) Context() context.Context { return s.ctx }

func TestStream(t *testing.T) {
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
}
