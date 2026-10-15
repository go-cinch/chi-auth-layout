package action

import "{{ .Computed.module_name_final }}/internal/common/report"

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
