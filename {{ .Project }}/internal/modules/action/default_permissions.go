package action

import (
	"context"
	"fmt"
)

// DefaultUserCodes joins the account creation transaction. Read the committed
// dictionary and membership after taking the same lock as catalog/action edits.
// Explicit selections are checked by the caller before these policy grants are
// merged, so ordinary account creators do not need permission-grant authority.
func (m *Module) DefaultUserCodes(ctx context.Context, selected []string) ([]string, error) {
	executor := m.store.SQL(ctx)
	if _, err := executor.ExecContext(ctx, `SELECT pg_advisory_xact_lock(73482301991337)`); err != nil {
		return nil, fmt.Errorf("lock default permission assignment: %w", err)
	}
	const query = `SELECT a.code FROM t_action AS a
	JOIN t_dictionary AS d ON d.dictionary_key = 'I18N_CATALOG' AND d.enabled
	WHERE d.value -> 'group_defaults' -> a.action_group = 'true'::JSONB
	ORDER BY a.id`
	rows, err := executor.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query default permission codes: %w", err)
	}
	defer rows.Close()
	codes := append([]string{}, selected...)
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return NormalizeCodes(codes), nil
}
