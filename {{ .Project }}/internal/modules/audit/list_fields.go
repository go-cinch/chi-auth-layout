package audit

import "{{ .Computed.module_name_final }}/internal/common/report"

var listFields = []string{"id", "occurred_at", "kind", "user_id", "username", "module", "method", "path", "ip", "user_agent", "trace_id", "error_code", "status", "duration_ms", "operation", "resource_type", "resource_id"}

func listProjection(value *Event, fields report.Fields) report.Projection {
	value.selectedFields = fields
	p := report.Projection{Fields: fields}
	p.Add("id", "id", &value.ID)
	p.Add("occurred_at", "occurred_at", &value.OccurredAt)
	p.Add("kind", "kind", &value.Kind)
	p.Add("user_id", "COALESCE(user_id,0)", &value.UserID)
	p.Add("username", "username", &value.Username)
	p.Add("module", "module", &value.Module)
	p.Add("method", "method", &value.Method)
	p.Add("path", "path", &value.Path)
	p.Add("ip", "ip", &value.IP)
	p.Add("user_agent", "user_agent", &value.UserAgent)
	p.Add("trace_id", "trace_id", &value.TraceID)
	p.Add("error_code", "error_code", &value.ErrorCode)
	p.Add("status", "status", &value.Status)
	p.Add("duration_ms", "duration_ms", &value.DurationMS)
	p.Add("operation", "operation", &value.Operation)
	p.Add("resource_type", "resource_type", &value.ResourceType)
	p.Add("resource_id", "resource_id", &value.ResourceID)
	return p
}

func (value Event) MarshalJSON() ([]byte, error) {
	type plain Event
	return report.MarshalFields(plain(value), value.selectedFields)
}
