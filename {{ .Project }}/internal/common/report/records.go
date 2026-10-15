package report

import (
	"context"
	"encoding/json"

	"{{ .Computed.module_name_final }}/internal/infra/db"
)

// MarshalFields preserves explicitly selected zero values and nulls.
// The caller passes an alias to avoid recursively invoking MarshalJSON.
func MarshalFields(value any, fields Fields) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil || fields == nil {
		return data, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	selected := make(map[string]json.RawMessage, len(fields))
	for _, field := range fields {
		selected[field] = all[field]
	}
	return json.Marshal(selected)
}

// ReadRecords executes only the server-owned projections selected by the caller.
func ReadRecords[T any](ctx context.Context, executor db.SQLExecutor, fields Fields, build func(*T, Fields) Projection, suffix string, args ...any) ([]T, error) {
	var sample T
	projection := build(&sample, fields)
	rows, err := executor.QueryContext(ctx, projection.SQL("")+suffix, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []T{}
	for rows.Next() {
		var item T
		projection := build(&item, fields)
		if err := rows.Scan(projection.Targets...); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
