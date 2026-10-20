package audit

import (
	"context"
	"database/sql"
	"errors"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

var detailFields = append(append([]string{}, listFields...), "header", "request_params", "response_params")

func (m *Module) GetSelected(ctx context.Context, kind string, id int64, fields report.Fields) (*Event, error) {
	if err := fields.Validate(detailFields...); err != nil {
		return nil, err
	}
	if fields == nil {
		return m.Get(ctx, kind, id)
	}
	if id <= 0 || (kind != "login" && kind != "operation") {
		return nil, ErrFilter
	}
	var value Event
	projection := listProjection(&value, fields)
	projection.Add("header", "header", &value.Header)
	projection.Add("request_params", "request_params", &value.RequestParams)
	projection.Add("response_params", "response_params", &value.ResponseParams)
	err := m.store.SQL(ctx).QueryRowContext(ctx, projection.SQL("t_audit_event")+" WHERE kind=$1 AND id=$2", kind, id).Scan(projection.Targets...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
}
