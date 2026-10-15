package dictionary

import "{{ .Computed.module_name_final }}/internal/common/report"

var listFields = []string{"id", "created_at", "updated_at", "key", "name", "description", "enabled"}

func listProjection(value *Dictionary, fields report.Fields) report.Projection {
	value.selectedFields = fields
	p := report.Projection{Fields: fields}
	p.Add("id", "id", &value.ID)
	p.Add("created_at", "created_at", &value.CreatedAt)
	p.Add("updated_at", "updated_at", &value.UpdatedAt)
	p.Add("key", "dictionary_key", &value.Key)
	p.Add("name", "name", &value.Name)
	p.Add("description", "description", &value.Description)
	p.Add("enabled", "enabled", &value.Enabled)
	return p
}

func (value Dictionary) MarshalJSON() ([]byte, error) {
	type plain Dictionary
	return report.MarshalFields(plain(value), value.selectedFields)
}
