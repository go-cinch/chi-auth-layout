package msg

import (
	"context"
	"database/sql"
	"errors"

	"{{ .Computed.module_name_final }}/internal/common/report"
)

var sentDetailFields = append(append([]string{}, listFields...), "recipient_ids", "recipients")

// GetSentSelected preserves the complete detail for existing callers. Explicit
// projections read only selected columns and the dependencies of selected recipients.
func (m *Module) GetSentSelected(ctx context.Context, id int64, fields report.Fields) (*Message, error) {
	if err := fields.Validate(sentDetailFields...); err != nil {
		return nil, err
	}
	if fields == nil {
		return m.GetSent(ctx, id)
	}
	if id <= 0 {
		return nil, ErrInvalid
	}
	value := &Message{ID: id}
	projection := listProjection(value, fields)
	if (fields.Has("recipient_ids") || fields.Has("recipients")) && !fields.Has("scope") {
		projection.Require("m.scope", &value.Scope)
	}
	err := m.store.SQL(ctx).QueryRowContext(ctx, projection.SQL("t_msg m")+" WHERE m.id = $1", id).Scan(projection.Targets...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if fields.Has("recipient_ids") || fields.Has("recipients") {
		if err := m.loadRecipients(ctx, value, fields); err != nil {
			return nil, err
		}
	}
	return value, nil
}

func (m *Module) loadRecipients(ctx context.Context, value *Message, fields report.Fields) error {
	if fields.Has("recipient_ids") {
		value.RecipientIDs = []int64{}
	}
	if fields.Has("recipients") {
		value.Recipients = []Recipient{}
	}
	if value.Scope != "targeted" {
		return nil
	}
	query := "SELECT r.user_id FROM t_msg_recipient r WHERE r.msg_id = $1 ORDER BY r.user_id"
	if fields.Has("recipients") {
		query = `SELECT r.user_id, COALESCE(u.username, ''), COALESCE(u.code, '') FROM t_msg_recipient r LEFT JOIN t_user u ON u.id = r.user_id WHERE r.msg_id = $1 ORDER BY r.user_id`
	}
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, value.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var recipient Recipient
		targets := []any{&recipient.ID}
		if fields.Has("recipients") {
			targets = append(targets, &recipient.Username, &recipient.Code)
		}
		if err := rows.Scan(targets...); err != nil {
			return err
		}
		if fields.Has("recipient_ids") {
			value.RecipientIDs = append(value.RecipientIDs, recipient.ID)
		}
		if fields.Has("recipients") {
			value.Recipients = append(value.Recipients, recipient)
		}
	}
	return rows.Err()
}
