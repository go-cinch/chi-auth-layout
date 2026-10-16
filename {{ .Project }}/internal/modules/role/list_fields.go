package role

import (
	"context"
	"fmt"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
)

var listFields = []string{"id", "created_at", "updated_at", "name", "word", "action_codes", "actions", "action_summaries"}

func listProjection(value *Role, fields report.Fields) report.Projection {
	value.selectedFields = fields
	p := report.Projection{Fields: fields}
	p.Add("id", "id", &value.ID)
	p.Add("created_at", "created_at", &value.CreatedAt)
	p.Add("updated_at", "updated_at", &value.UpdatedAt)
	p.Add("name", "name", &value.Name)
	p.Add("word", "word", &value.Word)
	if fields.Has("action_codes") || fields.Has("actions") || fields.Has("action_summaries") {
		p.Require("action", &value.selectedActions)
	}
	return p
}

func (value Role) MarshalJSON() ([]byte, error) {
	type plain Role
	return report.MarshalFields(plain(value), value.selectedFields)
}

func (m *Module) enrichList(ctx context.Context, items []Role) error {
	codes := make([]string, 0)
	for _, item := range items {
		codes = append(codes, item.ActionCodes...)
	}
	actions, err := m.actions.Lookup(ctx, m.store.SQL(ctx), codes)
	if err != nil {
		return err
	}
	byCode := make(map[string]ActionView, len(actions))
	for _, item := range actions {
		byCode[item.Code] = actionView(item)
	}
	for i := range items {
		item := &items[i]
		item.Actions = orderedActions(item.ActionCodes, byCode)
	}
	return nil
}

// Summary identifies a role without loading its permission rules.
type Summary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Word string `json:"word"`
}

func (m *Module) LookupSummaries(ctx context.Context, executor db.SQLExecutor, ids []int64) (map[int64]Summary, error) {
	result := map[int64]Summary{}
	unique := []int64{}
	seen := map[int64]bool{}
	for _, id := range ids {
		if !seen[id] {
			unique = append(unique, id)
			seen[id] = true
		}
	}
	if len(unique) == 0 {
		return result, nil
	}
	rows, err := executor.QueryContext(ctx, "SELECT id, name, word FROM t_role WHERE id = ANY($1)", pq.Array(unique))
	if err != nil {
		return nil, fmt.Errorf("lookup role summaries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var value Summary
		if err := rows.Scan(&value.ID, &value.Name, &value.Word); err != nil {
			return nil, err
		}
		result[value.ID] = value
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) != len(unique) {
		return nil, ErrNotFound
	}
	return result, nil
}

func (m *Module) enrichSummaries(ctx context.Context, items []Role, actionsLoaded bool) error {
	byCode := map[string]actionmodule.Summary{}
	if actionsLoaded {
		for _, item := range items {
			for _, action := range item.Actions {
				byCode[action.Code] = actionmodule.Summary{Code: action.Code, Name: action.Name, Group: action.Group}
			}
		}
	} else {
		codes := []string{}
		for _, item := range items {
			codes = append(codes, item.ActionCodes...)
		}
		var err error
		byCode, err = m.actions.LookupSummaries(ctx, m.store.SQL(ctx), codes)
		if err != nil {
			return err
		}
	}
	for i := range items {
		summaries := actionmodule.OrderedSummaries(items[i].ActionCodes, byCode)
		items[i].ActionSummaries = &summaries
	}
	return nil
}
