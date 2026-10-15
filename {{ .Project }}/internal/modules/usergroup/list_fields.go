package usergroup

import "{{ .Computed.module_name_final }}/internal/common/report"

var listFields = []string{"id", "created_at", "updated_at", "name", "word"}

func listProjection(value *UserGroup, fields report.Fields) report.Projection {
	value.selectedFields = fields
	p := report.Projection{Fields: fields}
	p.Add("id", "id", &value.ID)
	p.Add("created_at", "created_at", &value.CreatedAt)
	p.Add("updated_at", "updated_at", &value.UpdatedAt)
	p.Add("name", "name", &value.Name)
	p.Add("word", "word", &value.Word)
	// Association keys are internal dependencies, not automatically returned fields.
	if !fields.Has("id") && (fields.Has("users") || fields.Has("member_count") || fields.Has("member_preview")) {
		p.Require("id", &value.ID)
	}
	if fields.Has("actions") {
		p.Add("actions", "action", &value.selectedActions)
	}
	return p
}

func hasListColumns(fields report.Fields) bool {
	if fields == nil {
		return false
	}
	for _, name := range listFields {
		if fields.Has(name) {
			return true
		}
	}
	return false
}
