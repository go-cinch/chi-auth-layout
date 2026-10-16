package action

import (
	"context"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var listFields = []string{"id", "created_at", "updated_at", "code", "name", "group", "word", "resource", "menu", "button"}

func listProjection(value *Action, fields report.Fields) report.Projection {
	value.selectedFields = fields
	p := report.Projection{Fields: fields}
	p.Add("id", "id", &value.ID)
	p.Add("created_at", "created_at", &value.CreatedAt)
	p.Add("updated_at", "updated_at", &value.UpdatedAt)
	p.Add("code", "code", &value.Code)
	p.Add("name", "name", &value.Name)
	p.Add("group", "action_group", &value.Group)
	p.Add("word", "word", &value.Word)
	p.Add("resource", "resource", &value.Resource)
	p.Add("menu", "menu", &value.Menu)
	p.Add("button", "btn", &value.Button)
	return p
}

func (value Action) MarshalJSON() ([]byte, error) {
	type plain Action
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
	rows, err := executor.QueryContext(ctx, "SELECT code, name, action_group FROM t_action WHERE code = ANY($1)", pq.Array(codes))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var value Summary
		if err := rows.Scan(&value.Code, &value.Name, &value.Group); err != nil {
			return nil, err
		}
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
