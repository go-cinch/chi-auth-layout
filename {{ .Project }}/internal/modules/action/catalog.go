package action

import (
	"context"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/modules/localization"

	"{{ .Computed.module_name_final }}/internal/common/report"
)

type PermissionPaths struct {
	Btn  []string                `json:"btn"`
	HTTP []localization.HTTPPath `json:"http"`
	GRPC []string                `json:"grpc"`
}

type permissionInspector interface {
	PermissionPaths(context.Context, string, report.Fields) (PermissionPaths, error)
}

type ActionReference struct {
	ID   int64  `json:"id"`
	Word string `json:"word"`
}

type CatalogResult struct {
	selectedFields  report.Fields
	Scopes          []string                     `json:"scopes"`
	Operations      []string                     `json:"operations"`
	ResourceCatalog localization.ResourceCatalog `json:"resource_catalog"`
	DefinedActions  []ActionReference            `json:"defined_actions"`
}

func (value CatalogResult) MarshalJSON() ([]byte, error) {
	type plain CatalogResult
	return report.MarshalFields(plain(value), value.selectedFields)
}

type EffectiveResult struct {
	selectedFields report.Fields
	Scope          string                  `json:"scope"`
	Btn            []string                `json:"btn"`
	HTTP           []localization.HTTPPath `json:"http"`
	GRPC           []string                `json:"grpc"`
}

func (value EffectiveResult) MarshalJSON() ([]byte, error) {
	type plain EffectiveResult
	if value.selectedFields == nil {
		return report.MarshalFields(plain(value), nil)
	}
	fields := append(report.Fields{"scope"}, value.selectedFields...)
	return report.MarshalFields(plain(value), fields)
}

func (m *Module) Catalog(ctx context.Context, fields report.Fields) (*CatalogResult, error) {
	if err := fields.Validate("scopes", "operations", "resource_catalog", "defined_actions"); err != nil {
		return nil, err
	}
	if err := authn.RequirePermissionOperation(ctx, m.administration, "action", "read"); err != nil {
		return nil, err
	}
	var catalog localization.Catalog
	if fields.Has("scopes") || fields.Has("operations") || fields.Has("resource_catalog") {
		catalog = m.catalogSource.Select(fields)
	}
	scopes, operations := catalog.Scopes, catalog.Operations
	value := &CatalogResult{selectedFields: fields}
	if fields.Has("scopes") {
		value.Scopes = scopes
	}
	if fields.Has("operations") {
		value.Operations = operations
	}
	if fields.Has("resource_catalog") {
		value.ResourceCatalog = catalog.ResourceCatalog
	}
	if fields.Has("defined_actions") {
		rows, err := m.store.SQL(ctx).QueryContext(ctx, "SELECT id, word FROM t_action ORDER BY id")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		definitions := []ActionReference{}
		for rows.Next() {
			var entry ActionReference
			if err := rows.Scan(&entry.ID, &entry.Word); err != nil {
				return nil, err
			}
			definitions = append(definitions, entry)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		value.DefinedActions = definitions
	}
	return value, nil
}

func (m *Module) Effective(ctx context.Context, scope string, fields report.Fields) (*EffectiveResult, error) {
	if err := fields.Validate("btn", "http", "grpc"); err != nil {
		return nil, err
	}
	found := false
	scopes := m.catalogSource.Select(report.Fields{"scopes"}).Scopes
	for _, known := range scopes {
		found = found || known == scope
	}
	if !found {
		return nil, ErrInvalid
	}
	if err := authn.RequirePermissionOperation(ctx, m.administration, "action", "read"); err != nil {
		return nil, err
	}
	inspector, ok := m.administration.(permissionInspector)
	if !ok {
		return nil, authn.ErrPermissionAdministrationRequired
	}
	paths, err := inspector.PermissionPaths(ctx, scope, fields)
	if err != nil {
		return nil, err
	}
	value := &EffectiveResult{selectedFields: fields, Scope: scope}
	if fields.Has("btn") {
		value.Btn = paths.Btn
	}
	if fields.Has("http") {
		value.HTTP = paths.HTTP
	}
	if fields.Has("grpc") {
		value.GRPC = paths.GRPC
	}
	return value, nil
}
