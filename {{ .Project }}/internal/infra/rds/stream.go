package rds

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Stream is a dedicated work queue. Only this consumer group may delete entries.
type Stream struct {
	client *Client
	key    string
	group  string
}

func NewStream(client *Client, key, group string) *Stream {
	return &Stream{client: client, key: key, group: group}
}

func (s *Stream) Add(ctx context.Context, fields map[string]any) error {
	return s.client.client.XAdd(ctx, &redis.XAddArgs{Stream: s.client.prefix + s.key, Values: fields}).Err()
}

func (s *Stream) EnsureGroup(ctx context.Context) error {
	err := s.client.client.XGroupCreateMkStream(ctx, s.client.prefix+s.key, s.group, "0").Err()
	if redis.HasErrorPrefix(err, "BUSYGROUP") {
		return nil
	}
	return err
}

func (s *Stream) Read(ctx context.Context, consumer string) ([]redis.XMessage, error) {
	streams, err := s.client.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: s.group, Consumer: consumer, Streams: []string{s.client.prefix + s.key, ">"},
		Count: 1, Block: time.Second,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil || len(streams) == 0 {
		return nil, err
	}
	return streams[0].Messages, nil
}

func (s *Stream) Claim(ctx context.Context, consumer, cursor string, idle time.Duration) ([]redis.XMessage, string, error) {
	return s.client.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream: s.client.prefix + s.key, Group: s.group, Consumer: consumer, Start: cursor, MinIdle: idle, Count: 1,
	}).Result()
}

// Retry of a lost Redis reply is safe. A stale consumer cannot remove a new owner's entry.
// Move the complete original message before acknowledging it if retries were exhausted.
const finishStreamEntry = `
local pending = redis.call('XPENDING', KEYS[1], ARGV[1], ARGV[3], ARGV[3], 1)
if #pending == 0 or pending[1][2] ~= ARGV[2] then return 0 end
if ARGV[4] == 'failed' then
    local entry = redis.call('XRANGE', KEYS[1], ARGV[3], ARGV[3])
    if #entry == 0 then return redis.error_reply('missing pending stream entry') end
    redis.call('XADD', KEYS[2], '*', 'source_id', ARGV[3], unpack(entry[1][2]))
end
redis.call('XACK', KEYS[1], ARGV[1], ARGV[3])
return redis.call('XDEL', KEYS[1], ARGV[3])`

func (s *Stream) Finish(ctx context.Context, consumer, id string, failed bool) error {
	state := "committed"
	if failed {
		state = "failed"
	}
	return s.client.Eval(ctx, finishStreamEntry, []string{s.key, s.key + ":failed"}, s.group, consumer, id, state).Err()
}
