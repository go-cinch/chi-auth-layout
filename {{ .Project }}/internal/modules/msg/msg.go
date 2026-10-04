package msg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var (
	ErrInvalid   = apperror.New("MSG_INVALID", "invalid message fields")
	ErrRecipient = apperror.New("MSG_INVALID_RECIPIENT", "provide 1-1000 existing recipient ids for a targeted message")
	ErrNotFound  = apperror.New("MSG_NOT_FOUND", "message not found")
)

type Message struct {
	ID           int64   `json:"id"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
	Title        string  `json:"title"`
	Content      string  `json:"content"`
	Type         string  `json:"type"`
	Scope        string  `json:"scope"`
	SenderID     *int64  `json:"sender_id"`
	PublishedAt  int64   `json:"published_at"`
	ExpiredAt    *int64  `json:"expired_at"`
	ReadAt       *int64  `json:"read_at"`
	RecipientIDs []int64 `json:"recipient_ids,omitempty"`
}

type CreateInput struct {
	Title        string  `json:"title"`
	Content      string  `json:"content"`
	Type         string  `json:"type"`
	Scope        string  `json:"scope"`
	RecipientIDs []int64 `json:"recipient_ids"`
	ExpiredAt    *int64  `json:"expired_at"`
}

type ListInput struct {
	Page     *int32
	PageSize *int32
	Read     *bool
	Type     string
	Scope    string
}

type ListResult struct {
	Items    []Message `json:"items"`
	Total    int64     `json:"t"`
	Page     int32     `json:"p"`
	PageSize int32     `json:"s"`
}

type RecipientOption struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type Module struct {
	store  *db.Store
	limits pagination.Limits
}

func New(store *db.Store, limits pagination.Limits) *Module {
	return &Module{store: store, limits: limits}
}

// All recipient operations use this same visibility predicate. Deleted states are
// retained so a broadcast cannot reappear. Users created later do not see old broadcasts.
// Drive targeted lookups from the user's recipient index. The disjoint union
// avoids scanning every other user's targeted messages for each inbox request.
const inboxFrom = ` FROM (
 SELECT * FROM t_msg WHERE scope = 'all'
 UNION ALL
 SELECT target.* FROM t_msg_recipient delivery JOIN t_msg target ON target.id = delivery.msg_id
 WHERE delivery.user_id = $1 AND target.scope = 'targeted'
) m
 JOIN t_user u ON u.id = $1
 LEFT JOIN t_msg_recipient r ON r.msg_id = m.id AND r.user_id = u.id
 WHERE ((m.scope = 'all' AND m.published_at >= u.created_at) OR (m.scope = 'targeted' AND r.id IS NOT NULL))
 AND m.published_at <= $2 AND (m.expired_at IS NULL OR m.expired_at > $2)`

const columns = `m.id, m.created_at, m.updated_at, m.title, m.content, m.type, m.scope, m.sender_id, m.published_at, m.expired_at`

type scanner interface{ Scan(...any) error }

func scanMessage(row scanner) (Message, error) {
	var value Message
	var created, updated, published time.Time
	var expired, read sql.NullTime
	err := row.Scan(&value.ID, &created, &updated, &value.Title, &value.Content, &value.Type, &value.Scope, &value.SenderID, &published, &expired, &read)
	value.CreatedAt, value.UpdatedAt, value.PublishedAt = created.UnixMilli(), updated.UnixMilli(), published.UnixMilli()
	if expired.Valid {
		n := expired.Time.UnixMilli()
		value.ExpiredAt = &n
	}
	if read.Valid {
		n := read.Time.UnixMilli()
		value.ReadAt = &n
	}
	return value, err
}

func (m *Module) Create(ctx context.Context, senderID int64, input CreateInput) (*Message, error) {
	input.Title, input.Content = strings.TrimSpace(input.Title), strings.TrimSpace(input.Content)
	if senderID <= 0 || input.Title == "" || utf8.RuneCountInString(input.Title) > 200 || input.Content == "" || utf8.RuneCountInString(input.Content) > 20000 || !validType(input.Type) || !validScope(input.Scope) {
		return nil, ErrInvalid
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	var expiry any
	if input.ExpiredAt != nil {
		if *input.ExpiredAt <= now.UnixMilli() || *input.ExpiredAt > 253402300799999 {
			return nil, ErrInvalid
		}
		expiry = time.UnixMilli(*input.ExpiredAt).UTC()
	}
	ids := make([]int64, 0, len(input.RecipientIDs))
	seen := make(map[int64]bool)
	if (input.Scope == "all" && len(input.RecipientIDs) != 0) || (input.Scope == "targeted" && (len(input.RecipientIDs) == 0 || len(input.RecipientIDs) > 1000)) {
		return nil, ErrRecipient
	}
	for _, id := range input.RecipientIDs {
		if id <= 0 {
			return nil, ErrRecipient
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	var value *Message
	err := m.store.Tx(ctx, func(ctx context.Context) error {
		executor := m.store.SQL(ctx)
		var id int64
		err := executor.QueryRowContext(ctx, `INSERT INTO t_msg (created_at, updated_at, title, content, type, scope, sender_id, published_at, expired_at) VALUES ($1,$1,$2,$3,$4,$5,$6,$1,$7) RETURNING id`, now, input.Title, input.Content, input.Type, input.Scope, senderID, expiry).Scan(&id)
		if err != nil {
			return err
		}
		if input.Scope == "targeted" {
			result, err := executor.ExecContext(ctx, `INSERT INTO t_msg_recipient (msg_id, user_id) SELECT $1, id FROM t_user WHERE id = ANY($2)`, id, pq.Array(ids))
			if err != nil {
				return err
			}
			count, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if count != int64(len(ids)) {
				return ErrRecipient
			}
		}
		value, err = m.GetSent(ctx, id)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}
	return value, nil
}

func (m *Module) Get(ctx context.Context, userID, id int64) (*Message, error) {
	if userID <= 0 || id <= 0 {
		return nil, ErrInvalid
	}
	value, err := scanMessage(m.store.SQL(ctx).QueryRowContext(ctx, `SELECT `+columns+`, r.read_at`+inboxFrom+` AND r.deleted_at IS NULL AND m.id = $3`, userID, time.Now().UTC(), id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (m *Module) GetSent(ctx context.Context, id int64) (*Message, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	value, err := scanMessage(m.store.SQL(ctx).QueryRowContext(ctx, `SELECT `+columns+`, NULL::timestamp FROM t_msg m WHERE m.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if value.Scope == "targeted" {
		rows, err := m.store.SQL(ctx).QueryContext(ctx, `SELECT user_id FROM t_msg_recipient WHERE msg_id = $1 ORDER BY user_id`, id)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		value.RecipientIDs = make([]int64, 0)
		for rows.Next() {
			var userID int64
			if err := rows.Scan(&userID); err != nil {
				return nil, err
			}
			value.RecipientIDs = append(value.RecipientIDs, userID)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return &value, nil
}

func validType(value string) bool { return value == "system" || value == "notice" }

func validScope(value string) bool { return value == "all" || value == "targeted" }

func (m *Module) List(ctx context.Context, userID int64, input ListInput) (*ListResult, error) {
	if userID <= 0 {
		return nil, ErrInvalid
	}
	return m.listMessages(ctx, userID, input, false)
}

func (m *Module) ListSent(ctx context.Context, input ListInput) (*ListResult, error) {
	return m.listMessages(ctx, 0, input, true)
}

func (m *Module) listMessages(ctx context.Context, userID int64, input ListInput, sent bool) (*ListResult, error) {
	if (input.Type != "" && !validType(input.Type)) || (input.Scope != "" && !validScope(input.Scope)) || (sent && input.Read != nil) {
		return nil, ErrInvalid
	}
	page, size, inRange := m.limits.Normalize(input.Page, input.PageSize)
	value := &ListResult{Items: make([]Message, 0), Page: page, PageSize: size}
	if !inRange {
		return value, nil
	}
	from, selected := inboxFrom+` AND r.deleted_at IS NULL`, columns+`, r.read_at`
	args := []any{userID, time.Now().UTC()}
	if sent {
		from, selected, args = ` FROM t_msg m WHERE TRUE`, columns+`, NULL::timestamp`, []any{}
	}
	if input.Read != nil {
		if *input.Read {
			from += ` AND r.read_at IS NOT NULL`
		} else {
			from += ` AND r.read_at IS NULL`
		}
	}
	if input.Type != "" {
		args = append(args, input.Type)
		from += fmt.Sprintf(" AND m.type = $%d", len(args))
	}
	if input.Scope != "" {
		args = append(args, input.Scope)
		from += fmt.Sprintf(" AND m.scope = $%d", len(args))
	}
	if err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT COUNT(*)`+from, args...).Scan(&value.Total); err != nil {
		return nil, err
	}
	offset := int64(page-1) * int64(size)
	if offset >= value.Total {
		return value, nil
	}
	args = append(args, size, offset)
	rows, err := m.store.SQL(ctx).QueryContext(ctx, `SELECT `+selected+from+fmt.Sprintf(` ORDER BY m.published_at DESC, m.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		value.Items = append(value.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return value, nil
}

func (m *Module) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, ErrInvalid
	}
	var count int64
	err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT COUNT(*)`+inboxFrom+` AND r.deleted_at IS NULL AND r.read_at IS NULL`, userID, time.Now().UTC()).Scan(&count)
	return count, err
}

// Read and Delete use one statement: authorization and the sparse state write
// share a snapshot. Concurrent upserts preserve deletion and the first read time.
func (m *Module) Read(ctx context.Context, userID, id int64) error {
	if userID <= 0 || id <= 0 {
		return ErrInvalid
	}
	result, err := m.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_msg_recipient (msg_id, user_id, read_at)
 SELECT m.id, $1, $2`+inboxFrom+` AND r.deleted_at IS NULL AND m.id = $3
 ON CONFLICT (msg_id, user_id) DO UPDATE SET read_at = COALESCE(t_msg_recipient.read_at, EXCLUDED.read_at), updated_at = EXCLUDED.read_at
 WHERE t_msg_recipient.deleted_at IS NULL`, userID, time.Now().UTC(), id)
	return affected(result, err)
}

func (m *Module) Delete(ctx context.Context, userID, id int64) error {
	if userID <= 0 || id <= 0 {
		return ErrInvalid
	}
	result, err := m.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_msg_recipient (msg_id, user_id, deleted_at)
 SELECT m.id, $1, $2`+inboxFrom+` AND m.id = $3
 ON CONFLICT (msg_id, user_id) DO UPDATE SET deleted_at = COALESCE(t_msg_recipient.deleted_at, EXCLUDED.deleted_at), updated_at = EXCLUDED.deleted_at`, userID, time.Now().UTC(), id)
	return affected(result, err)
}

func (m *Module) ReadAll(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return ErrInvalid
	}
	_, err := m.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_msg_recipient (msg_id, user_id, read_at)
 SELECT m.id, $1, $2`+inboxFrom+` AND r.deleted_at IS NULL AND r.read_at IS NULL
 ON CONFLICT (msg_id, user_id) DO UPDATE SET read_at = COALESCE(t_msg_recipient.read_at, EXCLUDED.read_at), updated_at = EXCLUDED.read_at
 WHERE t_msg_recipient.deleted_at IS NULL`, userID, time.Now().UTC())
	return err
}

// DeleteSent removes the published message for everyone. It is a separately
// authorized management operation, never a recipient operation.
func (m *Module) DeleteSent(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrInvalid
	}
	result, err := m.store.SQL(ctx).ExecContext(ctx, `DELETE FROM t_msg WHERE id = $1`, id)
	return affected(result, err)
}

func affected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Module) RecipientOptions(ctx context.Context, query string) ([]RecipientOption, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) > 200 {
		return nil, ErrInvalid
	}
	rows, err := m.store.SQL(ctx).QueryContext(ctx, `SELECT id, username FROM t_user WHERE position(lower($1) in lower(username)) > 0 ORDER BY id LIMIT 50`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]RecipientOption, 0)
	for rows.Next() {
		var value RecipientOption
		if err := rows.Scan(&value.ID, &value.Username); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
