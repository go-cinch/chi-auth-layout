package audit

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/infra/rds"
)

const streamGroup = "audit-writers"

func (m *Module) UseRedisStream(client *rds.Client) {
	m.stream = rds.NewStream(client, m.settings.Stream, streamGroup)
}

func (m *Module) enqueue(ctx context.Context, event auditctx.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode audit stream event: %w", err)
	}
	// The ID survives an ambiguous XADD reply, redelivery and failed-stream replay.
	fields := map[string]any{"delivery_id": rand.Text(), "event": string(payload)}
	if event.AdditionalUserOperation != "" {
		fields["additional_user_operation"] = event.AdditionalUserOperation
	}
	if err := m.stream.Add(ctx, fields); err != nil {
		return fmt.Errorf("enqueue audit event in %s: %w", m.settings.Stream, err)
	}
	return nil
}

func decodeDelivery(fields map[string]any) (auditctx.Event, string, error) {
	id, _ := fields["delivery_id"].(string)
	payload, _ := fields["event"].(string)
	var event auditctx.Event
	if id == "" || json.Unmarshal([]byte(payload), &event) != nil ||
		(event.Kind != "operation" && event.Kind != "login") || event.UserID < 0 {
		return event, "", errors.New("invalid audit stream event")
	}
	event.AdditionalUserOperation, _ = fields["additional_user_operation"].(string)
	return event, id, nil
}

func (m *Module) persistDelivery(ctx context.Context, fields map[string]any) error {
	event, id, err := decodeDelivery(fields)
	if err != nil {
		return err
	}
	const retries = 3
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 && !waitStream(ctx, (100*time.Millisecond) << (attempt-1)) {
			return ctx.Err()
		}
		txCtx, cancel := context.WithTimeout(ctx, m.settings.RecordTimeout)
		err = m.store.Tx(txCtx, func(tx context.Context) error {
			return m.writeEvents(tx, event, id)
		})
		cancel()
		if err == nil || ctx.Err() != nil {
			return err
		}
	}
	return fmt.Errorf("write audit delivery after %d transaction attempts: %w", retries+1, err)
}

func waitStream(ctx context.Context, delay time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(delay):
		return true
	}
}

// ConsumeStream is started once per service instance. Reclaim pending entries even
// when new traffic is continuous; keep the scan cursor across batches.
func (m *Module) ConsumeStream(ctx context.Context) {
	consumer := rand.Text()
	for ctx.Err() == nil {
		err := m.stream.EnsureGroup(ctx)
		if err == nil {
			err = m.readDeliveries(ctx, consumer)
		}
		if ctx.Err() != nil {
			return
		}
		slog.ErrorContext(ctx, "consume audit stream failed: "+err.Error(), "operation", "consume_audit_stream",
			"outcome", "failure", "stream", m.settings.Stream, "error", err)
		waitStream(ctx, time.Second)
	}
}

func (m *Module) readDeliveries(ctx context.Context, consumer string) error {
	cursor := "0-0"
	var nextClaim time.Time
	for ctx.Err() == nil {
		var messages []redis.XMessage
		var err error
		if !time.Now().Before(nextClaim) {
			messages, cursor, err = m.stream.Claim(ctx, consumer, cursor, max(time.Minute, 5*m.settings.RecordTimeout))
			if err != nil {
				return err
			}
			if cursor == "0-0" {
				nextClaim = time.Now().Add(5 * time.Second)
			}
		}
		if len(messages) == 0 {
			messages, err = m.stream.Read(ctx, consumer)
			if err != nil {
				return err
			}
		}
		for _, msg := range messages {
			writeErr := m.persistDelivery(ctx, msg.Values)
			if ctx.Err() != nil {
				return ctx.Err() // Leave the entry pending for another worker.
			}
			if writeErr != nil {
				slog.ErrorContext(ctx, "audit stream delivery failed: "+writeErr.Error(), "operation", "write_audit_delivery",
					"outcome", "failure", "stream", m.settings.Stream, "entry_id", msg.ID, "error", writeErr)
			}
			if err := m.stream.Finish(ctx, consumer, msg.ID, writeErr != nil); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}
