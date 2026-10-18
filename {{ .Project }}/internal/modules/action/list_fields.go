package action

import (
	"context"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var listFields = []string{"id", "created_at", "updated_at", "code", "name", "group", "word", "resource", "menu", "button", "scope", "config"}

func listProjection(value *Action, fields report.Fields) report.Projection {
	value.selectedFields = fields
	p := report.Projection{Fields: fields}
	p.Add("id", "id", &value.ID)
	p.Add("created_at", "created_at", &value.CreatedAt)
	p.Add("updated_at", "updated_at", &value.UpdatedAt)
	p.Add("code", "code", &value.Code)
	p.Add("group", "action_group", &value.Group)
	p.Add("word", "word", &value.Word)
	needsMappings := fields.Has("config") || fields.Has("resource") || fields.Has("menu") || fields.Has("button")
	if fields.Has("name") || needsMappings {
		if !fields.Has("word") {
			p.Require("word", &value.Word)
		}
		if needsMappings {
			p.Require("config", &value.Config)
		} else {
			p.Require("config - 'ui' - 'endpoints'", &value.Config)
		}
	}
	p.Add("scope", "scope", &value.Scope)
	return p
}

func (value Action) MarshalJSON() ([]byte, error) {
	type plain Action
	if value.selectedFields != nil && value.selectedFields.Has("names") {
		names := value.Names
		if names == nil {
			names = map[string]string{}
		}
		return report.MarshalFields(struct {
			plain
			Names map[string]string `json:"names"`
		}{plain(value), names}, value.selectedFields)
	}
	return report.MarshalFields(plain(value), value.selectedFields)
}

// Summary is the identity displayed by permission counts, badges and groups.
type Summary struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Group string `json:"group"`
}

func (m *Module) LookupSummaries(ctx context.Context, executor db.SQLExecutor, codes []string) (map[string]Summary, error) {
	codes = NormalizeCodes(codes)
	result := map[string]Summary{}
	if len(codes) == 0 {
		return result, nil
	}
	rows, err := executor.QueryContext(ctx, "SELECT code, word, action_group, config - 'ui' - 'endpoints' FROM t_action WHERE code = ANY($1)", pq.Array(codes))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	catalog := m.catalogSource.Select([]string{"messages", "default_locale"})
	labels := newActionNameLabels(catalog, i18n.Locale(ctx, ""))
	for rows.Next() {
		var value Summary
		var word string
		var config Config
		if err := rows.Scan(&value.Code, &word, &value.Group, &config); err != nil {
			return nil, err
		}
		value.Name = labels.name(word, config)
		result[value.Code] = value
	}
	return result, rows.Err()
}

func OrderedSummaries(codes []string, byCode map[string]Summary) []Summary {
	result := []Summary{}
	for _, code := range NormalizeCodes(codes) {
		if value, ok := byCode[code]; ok {
			result = append(result, value)
		}
	}
	return result
}
