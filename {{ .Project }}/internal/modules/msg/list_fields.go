package msg

import "{{ .Computed.module_name_final }}/internal/common/report"

var listFields = []string{"id", "created_at", "updated_at", "title", "content", "summary", "type", "scope", "sender_id", "published_at", "expired_at", "read_at"}

func listProjection(value *Message, fields report.Fields) report.Projection {
	value.selectedFields = fields
	p := report.Projection{Fields: fields}
	p.Add("id", "m.id", &value.ID)
	p.Add("created_at", "m.created_at", &value.CreatedAt)
	p.Add("updated_at", "m.updated_at", &value.UpdatedAt)
	p.Add("title", "m.title", &value.Title)
	p.Add("content", "m.content", &value.Content)
	p.Add("summary", "LEFT(m.content, 160)", &value.Summary)
	p.Add("type", "m.type", &value.Type)
	p.Add("scope", "m.scope", &value.Scope)
	p.Add("sender_id", "m.sender_id", &value.SenderID)
	p.Add("published_at", "m.published_at", &value.PublishedAt)
	p.Add("expired_at", "m.expired_at", &value.ExpiredAt)
	p.Add("read_at", "NULL::bigint", &value.ReadAt)
	return p
}

func (value Message) MarshalJSON() ([]byte, error) {
	type plain Message
	return report.MarshalFields(plain(value), value.selectedFields)
}
