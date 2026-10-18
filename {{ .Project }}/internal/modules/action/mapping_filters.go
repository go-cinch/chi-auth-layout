package action

import (
	"context"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

// Mapping filters use the same captured catalog as the displayed results, even
// before an older persisted projection has been rebuilt or a replica has polled.
// Resolve candidates before counting and pagination, never just the current page.
func (m *Module) matchingMappings(ctx context.Context, input ListActionsInput, catalog localization.Catalog, where string, args []any) (map[string]Config, error) {
	rows, err := m.store.SQL(ctx).QueryContext(ctx, "SELECT word, config FROM t_action"+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fields := mappingSelection(input.Fields, input.Resource != "", input.Menu != "", input.Button != "")
	displayFields := report.Fields{}
	if strings.TrimSpace(input.Resource) != "" {
		displayFields = append(displayFields, "resource")
	}
	if strings.TrimSpace(input.Menu) != "" {
		displayFields = append(displayFields, "menu")
	}
	if strings.TrimSpace(input.Button) != "" {
		displayFields = append(displayFields, "button")
	}
	result := map[string]Config{}
	for rows.Next() {
		var value Action
		if err := rows.Scan(&value.Word, &value.Config); err != nil {
			return nil, err
		}
		resolved, err := ResolveConfig(catalog, value.Word, value.Config, fields)
		if err != nil {
			return nil, err
		}
		value.Config = resolved
		value.displayMappings(displayFields)
		if matchesMapping(value.Resource, input.Resource) && matchesMapping(value.Menu, input.Menu) && matchesMapping(value.Button, input.Button) {
			result[value.Word] = resolved
		}
	}
	return result, rows.Err()
}

func matchesMapping(value, filter string) bool {
	if strings.TrimSpace(filter) == "" {
		return true
	}
	keywords := splitFilterValues(filter)
	if len(keywords) == 0 {
		return true
	}
	for _, keyword := range keywords {
		if strings.Contains(strings.ToLower(value), strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func mappingSelection(fields report.Fields, resource, menu, button bool) report.Fields {
	if fields.Has("config") {
		return nil
	}
	selected := report.Fields{}
	if resource || fields.Has("resource") {
		selected = append(selected, "http", "grpc")
	}
	if menu || button || fields.Has("menu") || fields.Has("button") {
		selected = append(selected, "ui")
	}
	return selected
}

func (value *Action) deriveSelectedDisplay(catalog localization.Catalog, fields report.Fields, cached map[string]Config) {
	resolved, ok := cached[value.Word]
	if !ok {
		var err error
		resolved, err = ResolveConfig(catalog, value.Word, value.Config, mappingSelection(fields, false, false, false))
		if err != nil {
			resolved = value.Config
			resolved.UI, resolved.Endpoints = []UIBinding{}, []Endpoint{}
		}
	}
	value.Config = resolved
	value.Name = value.Word
	value.displayMappings(fields)
}

func (value *Action) displayMappings(fields report.Fields) {
	if fields.Has("resource") {
		items := []string{}
		for _, endpoint := range value.Config.Endpoints {
			items = append(items, endpoint.Display())
		}
		value.Resource = strings.Join(items, "\n")
	}
	if fields.Has("menu") {
		items := []string{}
		for _, ui := range value.Config.UI {
			items = append(items, ui.MenuPath)
		}
		value.Menu = strings.Join(items, "\n")
	}
	if fields.Has("button") {
		items := []string{}
		for _, ui := range value.Config.UI {
			items = append(items, ui.ButtonKeys...)
		}
		value.Button = strings.Join(items, "\n")
	}
}
