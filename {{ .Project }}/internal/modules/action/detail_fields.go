package action

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

var detailFields = []string{"id", "created_at", "updated_at", "code", "name", "names", "group", "word", "scope", "config", "resource", "menu", "button"}

// GetSelected preserves the complete detail when fields is nil. Explicit
// selections load only the requested columns and their server-owned dependencies.
func (m *Module) GetSelected(ctx context.Context, id int64, fields report.Fields) (*Action, error) {
	if err := fields.Validate(detailFields...); err != nil {
		return nil, err
	}
	if fields == nil {
		return m.Get(ctx, id)
	}
	if id <= 0 {
		return nil, ErrInvalid
	}
	value := &Action{selectedFields: fields}
	projection := detailProjection(value, fields)
	err := m.store.SQL(ctx).QueryRowContext(ctx, projection.SQL("t_action")+" WHERE id = $1", id).Scan(projection.Targets...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query action detail: %w", err)
	}
	needsMappings := fields.Has("config") || fields.Has("resource") || fields.Has("menu") || fields.Has("button")
	catalogFields := report.Fields{}
	if needsMappings {
		catalogFields = append(catalogFields, "resource_catalog")
	}
	if fields.Has("name") || fields.Has("names") {
		catalogFields = append(catalogFields, "frontend_messages")
	}
	if fields.Has("name") {
		catalogFields = append(catalogFields, "default_locale")
	}
	if fields.Has("names") {
		catalogFields = append(catalogFields, "locales")
	}
	if len(catalogFields) > 0 {
		catalog := m.catalogSource.Select(catalogFields)
		if needsMappings {
			value.deriveSelectedDisplay(catalog, fields, nil)
		}
		if fields.Has("name") {
			value.localizeName(catalog, i18n.Locale(ctx, ""))
		}
		if fields.Has("names") {
			value.loadNames(catalog)
		}
	}
	return value, nil
}

func detailProjection(value *Action, fields report.Fields) report.Projection {
	p := report.Projection{Fields: fields}
	p.Add("id", "id", &value.ID)
	p.Add("created_at", "created_at", &value.CreatedAt)
	p.Add("updated_at", "updated_at", &value.UpdatedAt)
	p.Add("code", "code", &value.Code)
	p.Add("group", "action_group", &value.Group)
	p.Add("word", "word", &value.Word)
	p.Add("scope", "scope", &value.Scope)
	needsConfig := fields.Has("name") || fields.Has("config") || fields.Has("resource") || fields.Has("menu") || fields.Has("button")
	if (needsConfig || fields.Has("names")) && !fields.Has("word") {
		p.Require("word", &value.Word)
	}
	if needsConfig {
		// Mapping displays are resolved from the captured catalog; the stored
		// UI and endpoint projections are neither needed nor authoritative.
		p.Require("config - 'ui' - 'endpoints'", &value.Config)
	}
	return p
}
