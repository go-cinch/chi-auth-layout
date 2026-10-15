package user

import "{{ .Computed.module_name_final }}/internal/common/report"

var listFields = []string{"id", "created_at", "updated_at", "username", "code", "role_id", "last_logged_in_at", "login_count"}

func listProjection(value *User, fields report.Fields) report.Projection {
	value.selectedFields = fields
	p := report.Projection{Fields: fields}
	p.Add("id", "id", &value.ID)
	p.Add("created_at", "created_at", &value.CreatedAt)
	p.Add("updated_at", "updated_at", &value.UpdatedAt)
	p.Add("username", "username", &value.Username)
	p.Add("code", "code", &value.Code)
	p.Add("role_id", "role_id", &value.RoleID)
	p.Add("last_logged_in_at", "last_logged_in_at", &value.LastLoggedInAt)
	p.Add("login_count", "login_count", &value.LoginCount)
	return p
}

func (value User) MarshalJSON() ([]byte, error) {
	type plain User
	return report.MarshalFields(plain(value), value.selectedFields)
}
