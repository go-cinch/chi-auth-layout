package usergroup

import (
	"context"
	"{{ .Computed.module_name_final }}/internal/common/report"
	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
)

var listFields = []string{"id", "created_at", "updated_at", "name", "word", "action_codes", "action_summaries"}

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
	if fields.Has("actions") || fields.Has("action_codes") || fields.Has("action_summaries") {
		p.Require("action", &value.selectedActions)
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

func (m *Module) enrichSummaries(ctx context.Context, items []UserGroup, actionsLoaded bool) error {
	byCode := map[string]actionmodule.Summary{}
	if actionsLoaded {
		for _, item := range items {
			for _, action := range item.Actions {
				byCode[action.Code] = actionmodule.Summary{Code: action.Code, Name: action.Name, Group: action.Group}
			}
		}
	} else {
		codes := []string{}
		for _, item := range items {
			codes = append(codes, item.ActionCodes...)
		}
		var err error
		byCode, err = m.actions.LookupSummaries(ctx, m.store.SQL(ctx), codes)
		if err != nil {
			return err
		}
	}
	for i := range items {
		summaries := actionmodule.OrderedSummaries(items[i].ActionCodes, byCode)
		items[i].ActionSummaries = &summaries
	}
	return nil
}
