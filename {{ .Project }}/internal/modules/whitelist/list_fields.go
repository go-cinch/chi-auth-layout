package whitelist

import "{{ .Computed.module_name_final }}/internal/common/report"

var listFields = []string{"id", "created_at", "updated_at", "category", "resource"}

func listProjection(value *Whitelist, fields report.Fields) report.Projection {
	value.selectedFields = fields
	p := report.Projection{Fields: fields}
	p.Add("id", "id", &value.ID)
	p.Add("created_at", "created_at", &value.CreatedAt)
	p.Add("updated_at", "updated_at", &value.UpdatedAt)
	p.Add("category", "category", &value.Category)
	p.Add("resource", "resource", &value.Resource)
	return p
}

func (value Whitelist) MarshalJSON() ([]byte, error) {
	type plain Whitelist
	return report.MarshalFields(plain(value), value.selectedFields)
}
