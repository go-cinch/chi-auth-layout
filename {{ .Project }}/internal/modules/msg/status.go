package msg

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"
)

// The ordinary polling path reads one indexed row. Recalculation happens only
// on mutations, first use by a new user, or when a message becomes visible or expires.
func (m *Module) Status(ctx context.Context, userID int64) (*UnreadStatus, error) {
	if userID <= 0 {
		return nil, ErrInvalid
	}
	value, due, err := m.storedStatus(ctx, userID)
	if err == nil && !due {
		return value, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	err = m.statusTx(ctx, []int64{userID}, func(ctx context.Context) error {
		if err := m.refreshStatus(ctx, []int64{userID}, true); err != nil {
			return err
		}
		value, _, err = m.storedStatus(ctx, userID)
		return err
	})
	return value, err
}

func (m *Module) storedStatus(ctx context.Context, userID int64) (*UnreadStatus, bool, error) {
	value := &UnreadStatus{UserID: userID}
	var latest, expiry sql.NullInt64
	var observed int64
	err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT unread_count, latest_unread_at, next_refresh_at, `+messageTime+` FROM t_user_msg_status WHERE user_id = $1`, userID).Scan(&value.Count, &latest, &expiry, &observed)
	value.ObservedAt = observed
	if latest.Valid {
		at := latest.Int64
		value.LatestUnreadAt = &at
	}
	return value, expiry.Valid && expiry.Int64 <= observed, err
}

// nil users denotes a broadcast/global deletion. A database-wide advisory gate
// makes it exclusive against per-user mutations/initialization, including new
// users arriving during a broadcast. Targeted operations share the gate, then
// lock affected status rows in user-ID order. All replicas use these same locks.
func (m *Module) statusTx(ctx context.Context, users []int64, change func(context.Context) error) error {
	return m.store.Tx(ctx, func(ctx context.Context) error {
		lock := `SELECT pg_advisory_xact_lock_shared(1735290211, 1)`
		if users == nil {
			lock = `SELECT pg_advisory_xact_lock(1735290211, 1)`
		}
		if _, err := m.store.SQL(ctx).ExecContext(ctx, lock); err != nil {
			return err
		}
		// An epoch deadline marks a newly initialized row for recalculation.
		if _, err := m.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_user_msg_status(user_id, next_refresh_at)
 SELECT id, 0 FROM t_user WHERE $1 OR id = ANY($2) ORDER BY id
 ON CONFLICT (user_id) DO NOTHING`, users == nil, pq.Array(users)); err != nil {
			return err
		}
		rows, err := m.store.SQL(ctx).QueryContext(ctx, `SELECT user_id FROM t_user_msg_status WHERE $1 OR user_id = ANY($2) ORDER BY user_id FOR UPDATE`, users == nil, pq.Array(users))
		if err != nil {
			return err
		}
		for rows.Next() {
			// Exhaust the cursor so every selected row is locked before writing.
		}
		err = rows.Err()
		if closeErr := rows.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		return change(ctx)
	})
}

func (m *Module) refreshStatus(ctx context.Context, users []int64, dueOnly bool) error {
	_, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user_msg_status s SET
 unread_count = v.n, latest_unread_at = v.latest, next_refresh_at = v.expiry, updated_at = `+messageTime+`
 FROM (SELECT state.user_id, aggregate.* FROM t_user_msg_status state
 CROSS JOIN LATERAL (
 SELECT COUNT(*) FILTER (WHERE m.published_at <= `+messageTime+`) AS n,
 MAX(m.published_at) FILTER (WHERE m.published_at <= `+messageTime+`) AS latest,
 MIN(CASE WHEN m.published_at > `+messageTime+` THEN m.published_at ELSE m.expired_at END) AS expiry FROM (
 SELECT b.* FROM t_msg b JOIN t_user u ON u.id = state.user_id WHERE b.scope = 'all' AND b.published_at >= u.created_at
 UNION ALL
 SELECT target.* FROM t_msg_recipient d JOIN t_msg target ON target.id = d.msg_id WHERE d.user_id = state.user_id AND target.scope = 'targeted'
 ) m LEFT JOIN t_msg_recipient r ON r.msg_id = m.id AND r.user_id = state.user_id
 WHERE (m.expired_at IS NULL OR m.expired_at > `+messageTime+`)
 AND r.read_at IS NULL AND r.deleted_at IS NULL
 ) aggregate WHERE ($1 OR state.user_id = ANY($2)) AND (NOT $3 OR state.next_refresh_at <= `+messageTime+`)
 ) v WHERE s.user_id = v.user_id`, users == nil, pq.Array(users), dueOnly)
	return err
}

// Sending increments persisted statistics without rescanning every recipient's
// message history. Only overdue/new rows need repair before this increment.
func (m *Module) addStatus(ctx context.Context, users []int64, published int64, expiry any) error {
	_, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user_msg_status s SET
 unread_count = unread_count + 1, latest_unread_at = GREATEST(latest_unread_at, $3),
 next_refresh_at = LEAST(next_refresh_at, $4::bigint), updated_at = `+messageTime+`
 FROM t_user u WHERE u.id = s.user_id AND ($1 AND u.created_at <= $3 OR s.user_id = ANY($2))`, users == nil, pq.Array(users), published, expiry)
	return err
}

// ExpireStatus bounds each background sweep for publication and expiry deadlines.
// Status also repairs on access, so callers never depend on the worker's timing.
func (m *Module) ExpireStatus(ctx context.Context) error {
	rows, err := m.store.SQL(ctx).QueryContext(ctx, `SELECT user_id FROM t_user_msg_status WHERE next_refresh_at <= `+messageTime+` ORDER BY next_refresh_at, user_id LIMIT 100`)
	if err != nil {
		return err
	}
	var users []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		users = append(users, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil || len(users) == 0 {
		return err
	}
	return m.statusTx(ctx, users, func(ctx context.Context) error { return m.refreshStatus(ctx, users, true) })
}
