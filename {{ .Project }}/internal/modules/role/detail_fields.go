package role

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

func (m *Module) GetSelected(ctx context.Context, id int64, fields report.Fields) (*Role, error) {
	if err := fields.Validate(listFields...); err != nil {
		return nil, err
	}
	if fields == nil {
		return m.Get(ctx, id)
	}
	if id <= 0 {
		return nil, ErrInvalid
	}
	var value Role
	projection := listProjection(&value, fields)
	err := m.store.SQL(ctx).QueryRowContext(ctx, projection.SQL("t_role")+" WHERE id = $1", id).Scan(projection.Targets...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query role detail: %w", err)
	}
	if fields.Has("action_codes") || fields.Has("actions") || fields.Has("action_summaries") {
		value.ActionCodes = m.actions.SplitCodes(value.selectedActions)
	}
	items := []Role{value}
	if fields.Has("actions") {
		if err := m.enrichList(ctx, items); err != nil {
			return nil, err
		}
	}
	if fields.Has("action_summaries") {
		if err := m.enrichSummaries(ctx, items, fields.Has("actions")); err != nil {
			return nil, err
		}
	}
	return &items[0], nil
}
