package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

// GetSelected preserves the legacy detail unless an explicit projection is supplied.
func (m *Module) GetSelected(ctx context.Context, id int64, fields report.Fields) (*User, error) {
	if err := fields.Validate(listFields...); err != nil {
		return nil, err
	}
	if fields == nil {
		return m.Get(ctx, id)
	}
	if id <= 0 {
		return nil, ErrInvalid
	}
	var value User
	projection := listProjection(&value, fields, "")
	err := m.store.SQL(ctx).QueryRowContext(ctx, projection.SQL("t_user")+" WHERE id = $1", id).Scan(projection.Targets...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query user detail: %w", err)
	}
	values, err := m.enrichSelected(ctx, []User{value}, fields)
	if err != nil {
		return nil, err
	}
	return &values[0], nil
}
