package audit

import (
	"encoding/json"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

var listFields = []string{"step_up_fingerprint", "step_up_verified_at", "id", "occurred_at", "kind", "user_id", "username", "module", "method", "path", "ip", "user_agent", "trace_id", "error_code", "status", "duration_ms", "operation", "resource_type", "resource_id"}

// List and export load metadata only. Nil fields preserve the complete JSON
// shape and RawMessage null values; detail adds and scans its own payload columns.
func listProjection(value *Event, fields report.Fields) report.Projection {
	value.selectedFields = fields
	if fields == nil {
		value.Header = json.RawMessage("null")
		value.RequestParams = json.RawMessage("null")
		value.ResponseParams = json.RawMessage("null")
	}
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
	p.Add("step_up_fingerprint", "step_up_fingerprint", &value.StepUpFingerprint)
	p.Add("step_up_verified_at", "COALESCE(step_up_verified_at,0)", &value.StepUpVerifiedAt)
	return p
}

func (value Event) MarshalJSON() ([]byte, error) {
	type plain Event
	return report.MarshalFields(plain(value), value.selectedFields)
}
