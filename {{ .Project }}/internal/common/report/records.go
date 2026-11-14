package report

import (
	"context"
	"database/sql"
)

// Querier executes the selected projection using a database or transaction.
// Consumers need no connection management or mutation methods.
type Querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// ReadRecords executes only the server-owned projections selected by the caller.
func ReadRecords[T any](ctx context.Context, executor Querier, fields Fields, build func(*T, Fields) Projection, suffix string, args ...any) ([]T, error) {
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
