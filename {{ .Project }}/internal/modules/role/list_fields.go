package role

import "{{ .Computed.module_name_final }}/internal/common/report"

var listFields = []string{"id", "created_at", "updated_at", "name", "word"}

func listProjection(value *Role, fields report.Fields) report.Projection {
	value.selectedFields = fields
	p := report.Projection{Fields: fields}
	p.Add("id", "id", &value.ID)
	p.Add("created_at", "created_at", &value.CreatedAt)
	p.Add("updated_at", "updated_at", &value.UpdatedAt)
	p.Add("name", "name", &value.Name)
	p.Add("word", "word", &value.Word)
	return p
}

func (value Role) MarshalJSON() ([]byte, error) {
	type plain Role
	return report.MarshalFields(plain(value), value.selectedFields)
}
