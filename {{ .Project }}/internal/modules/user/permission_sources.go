package user

import (
	"context"
	"encoding/json"
	"fmt"

	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
)

// PermissionSource describes an assignment owner, including empty roles/groups.
// It is loaded only by an explicit detail projection, never by user lists.
type PermissionSource struct {
	Kind        string             `json:"kind"`
	ID          int64              `json:"id"`
	Name        string             `json:"name"`
	ActionCodes []string           `json:"action_codes"`
	Actions     []PermissionAction `json:"actions"`
}

type PermissionAction struct {
	Code   string              `json:"code"`
	Word   string              `json:"word"`
	Config actionmodule.Config `json:"config"`
}

func (m *Module) permissionSources(ctx context.Context, userID int64) ([]PermissionSource, error) {
	// Read assignments and definitions in one statement snapshot. Keep every
	// owner when codes overlap; effective permissions are their union.
	const query = `WITH owners AS (
		SELECT 'direct' AS kind, u.id, u.username AS name, u.action, 1 AS source_order
		FROM t_user u WHERE u.id = $1
		UNION ALL
		SELECT 'role', r.id, r.name, r.action, 2
		FROM t_user u JOIN t_role r ON r.id = u.role_id WHERE u.id = $1
		UNION ALL
		SELECT 'group', g.id, g.name, g.action, 3
		FROM t_user_user_group_relation relation
		JOIN t_user_group g ON g.id = relation.user_group_id WHERE relation.user_id = $1
	)
	SELECT owners.kind, owners.id, owners.name, COALESCE(BTRIM(assigned.code), ''),
		COALESCE(a.word, ''), COALESCE(a.config - 'ui' - 'endpoints', '{}'::jsonb)
	FROM owners
	LEFT JOIN LATERAL UNNEST(STRING_TO_ARRAY(owners.action, ',')) WITH ORDINALITY AS assigned(code, position)
		ON BTRIM(assigned.code) <> ''
	LEFT JOIN t_action a ON a.code = BTRIM(assigned.code)
	ORDER BY owners.source_order, owners.id, assigned.position`
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query user permission sources: %w", err)
	}
	defer rows.Close()
	sources := []PermissionSource{}
	for rows.Next() {
		var kind, name, code, word string
		var id int64
		var config []byte
		if err := rows.Scan(&kind, &id, &name, &code, &word, &config); err != nil {
			return nil, fmt.Errorf("scan user permission source: %w", err)
		}
		if len(sources) == 0 || sources[len(sources)-1].Kind != kind || sources[len(sources)-1].ID != id {
			sources = append(sources, PermissionSource{Kind: kind, ID: id, Name: name, ActionCodes: []string{}, Actions: []PermissionAction{}})
		}
		source := &sources[len(sources)-1]
		if code == "" {
			continue
		}
		source.ActionCodes = append(source.ActionCodes, code)
		if word == "" {
			continue
		}
		action := PermissionAction{Code: code, Word: word}
		if err := json.Unmarshal(config, &action.Config); err != nil {
			return nil, fmt.Errorf("decode user permission source: %w", err)
		}
		source.Actions = append(source.Actions, action)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read user permission sources: %w", err)
	}
	return sources, nil
}
