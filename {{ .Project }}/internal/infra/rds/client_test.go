package rds

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestClientDeadlineInterruptsSocketRead(t *testing.T) {
	connection, peer := net.Pipe()
	defer peer.Close()
	defer connection.Close()
	commandRead := make(chan struct{})
	go func() {
		reader := bufio.NewReader(peer)
		for {
			header, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			count, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "*")))
			if err != nil {
				return
			}
			var command string
			for index := 0; index < count; index++ {
				header, err = reader.ReadString('\n')
				if err != nil {
					return
				}
				length, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "$")))
				if err != nil || length < 0 {
					return
				}
				value := make([]byte, length+2)
				if _, err := io.ReadFull(reader, value); err != nil {
					return
				}
				if index == 0 {
					command = strings.ToLower(string(value[:length]))
				}
			}
			if command == "get" {
				close(commandRead)
				return // Leave the socket open without replying.
			}
			reply := "+PONG\r\n"
			if command == "hello" {
				reply = "-ERR unknown command 'hello'\r\n"
			}
			if _, err := fmt.Fprint(peer, reply); err != nil {
				return
			}
		}
	}()
	options := &redis.Options{
		Protocol: 2, DisableIdentity: true, MaxRetries: -1,
		Dialer: func(context.Context, string, string) (net.Conn, error) { return connection, nil },
	}
	client, err := newClient(options, "test:deadline:")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Ping(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := client.Get(ctx, "probe:value:1").Err(); err == nil {
		t.Fatal("stalled read succeeded")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("request deadline ignored: %s", elapsed)
	}
	select {
	case <-commandRead:
	default:
		t.Fatal("test did not reach socket read")
	}
	if options.ContextTimeoutEnabled {
		t.Fatal("caller options mutated")
	}
}

type captureCommands struct {
	commands [][]interface{}
}

func (h *captureCommands) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *captureCommands) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, command redis.Cmder) error {
		h.commands = append(h.commands, append([]interface{}(nil), command.Args()...))
		return nil
	}
}

func (h *captureCommands) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func testClient(t *testing.T, prefix string, hook *captureCommands) *Client {
	t.Helper()
	client, err := newClient(&redis.Options{Addr: "unused:6379"}, prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	client.client.AddHook(hook)
	return client
}

func TestClientPrefixesEveryKeyCommand(t *testing.T) {
	hook := &captureCommands{}
	client := testClient(t, "dev:auth:", hook)
	ctx := t.Context()
	client.Set(ctx, "session:data:id", "session:value", time.Minute)
	client.Get(ctx, "session:data:id")
	client.GetDel(ctx, "session:refresh:digest")
	keys := []string{"session:data:id", "session:refresh:digest"}
	client.Del(ctx, keys...)
	client.SetNX(ctx, "idempotency:request:id", "1", time.Minute)
	client.Incr(ctx, "dictionary:cache:version")
	script := "return redis.call('GET', KEYS[1])"
	client.Eval(ctx, script, keys, "unchanged:argument")
	client.Ping(ctx)
	if !reflect.DeepEqual(keys, []string{"session:data:id", "session:refresh:digest"}) {
		t.Fatalf("caller keys mutated: %v", keys)
	}
	want := [][]interface{}{
		{"set", "dev:auth:session:data:id", "session:value", "ex", int64(60)},
		{"get", "dev:auth:session:data:id"},
		{"getdel", "dev:auth:session:refresh:digest"},
		{"del", "dev:auth:session:data:id", "dev:auth:session:refresh:digest"},
		{"set", "dev:auth:idempotency:request:id", "1", "ex", int64(60), "nx"},
		{"incr", "dev:auth:dictionary:cache:version"},
		{"eval", script, 2, "dev:auth:session:data:id", "dev:auth:session:refresh:digest", "unchanged:argument"},
		{"ping"},
	}
	if !reflect.DeepEqual(hook.commands, want) {
		t.Fatalf("commands = %#v, want %#v", hook.commands, want)
	}
}

func TestClientNamespaceIsolation(t *testing.T) {
	hook := &captureCommands{}
	for _, prefix := range []string{"dev:auth", "prod:auth:", "dev:order:"} {
		client := testClient(t, prefix, hook)
		client.Get(t.Context(), "session:data:same-id")
		client.Del(t.Context(), "session:data:same-id")
		cache, err := NewNamespaceCache(client, "dictionary:cache")
		if err != nil {
			t.Fatal(err)
		}
		if err := cache.Invalidate(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := cache.Store(t.Context(), "KEY", `[]`, "0", time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	for index, prefix := range []string{"dev:auth:", "prod:auth:", "dev:order:"} {
		for offset, key := range []string{"session:data:same-id", "session:data:same-id", "dictionary:cache:version", "dictionary:cache:value:0:KEY"} {
			if got := hook.commands[index*4+offset][1]; got != prefix+key {
				t.Fatalf("namespace %s command %d key = %v", prefix, offset, got)
			}
		}
	}
}

func TestClientRejectsInvalidPrefixes(t *testing.T) {
	for _, prefix := range []string{"", " ", "auth", ":auth", "dev:", "dev:auth::", "dev:auth:extra", "dev:*", "dev:a b", "dev:a?", "dev:{auth}", "dev:a\\b"} {
		if client, err := newClient(&redis.Options{}, prefix); err == nil {
			_ = client.Close()
			t.Errorf("accepted prefix %q", prefix)
		}
	}
	hook := &captureCommands{}
	client := testClient(t, " dev:auth-service.v2_1: ", hook)
	client.Get(t.Context(), "key")
	if got := hook.commands[0][1]; got != "dev:auth-service.v2_1:key" {
		t.Fatalf("normalized key = %v", got)
	}
}
