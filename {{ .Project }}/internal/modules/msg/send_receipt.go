package msg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"slices"

	"{{ .Computed.module_name_final }}/internal/common/apperror"
)

// The unique insert waits for a concurrent send to commit or roll back.
// A committed receipt with no message is a withdrawal, never a new delivery.
func (m *Module) claimSend(ctx context.Context, senderID int64, input CreateInput) (int64, error) {
	slices.Sort(input.RecipientIDs)
	payload, err := json.Marshal(input)
	if err != nil {
		return 0, err
	}
	hash := sha256.Sum256(payload)
	executor := m.store.SQL(ctx)
	insert, err := executor.ExecContext(ctx, `INSERT INTO t_msg_send_receipt(sender_id, request_key, payload_hash)
 VALUES ($1, $2, $3) ON CONFLICT (sender_id, request_key) DO NOTHING`, senderID, input.requestKey, hash[:])
	if err != nil {
		return 0, err
	}
	inserted, err := insert.RowsAffected()
	if err != nil || inserted == 1 {
		return 0, err
	}
	var storedHash []byte
	var original sql.NullInt64
	err = executor.QueryRowContext(ctx, `SELECT payload_hash, msg_id FROM t_msg_send_receipt WHERE sender_id = $1 AND request_key = $2`, senderID, input.requestKey).Scan(&storedHash, &original)
	if err != nil {
		return 0, err
	}
	if !bytes.Equal(storedHash, hash[:]) {
		return 0, apperror.DuplicateRequest
	}
	if !original.Valid {
		return 0, ErrNotFound
	}
	return original.Int64, nil
}
