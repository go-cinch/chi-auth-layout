package user

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"{{ .Computed.module_name_final }}/internal/common/report"
	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
	rolemodule "{{ .Computed.module_name_final }}/internal/modules/role"
)

var listFields = []string{"id", "created_at", "updated_at", "username", "code", "role_id", "last_logged_in_at", "login_count", "status", "metadata", "wrong", "action_codes", "actions", "role", "role_summary"}

func listProjection(value *User, fields report.Fields, now string) report.Projection {
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
	p.Add("status", "CASE WHEN "+expiredLockSQL(now)+" THEN 1 ELSE status END", &value.Status)
	p.Add("metadata", "CASE WHEN "+expiredLockSQL(now)+" THEN metadata - 'lock_expired_at' ELSE metadata END", &value.selectedMetadata)
	p.Add("wrong", "wrong", &value.Wrong)
	if fields.Has("action_codes") || fields.Has("actions") {
		p.Require("action", &value.selectedActions)
	}
	if !fields.Has("role_id") && (fields.Has("role") || fields.Has("role_summary")) {
		p.Require("role_id", &value.RoleID)
	}
	return p
}

func (value User) MarshalJSON() ([]byte, error) {
	type plain User
	return report.MarshalFields(plain(value), value.selectedFields)
}

func (m *Module) listSelected(ctx context.Context, fields report.Fields, where string, args []any, now int64, size int32, offset int64) ([]User, error) {
	placeholder := "0"
	if fields.Has("status") || fields.Has("metadata") {
		args = append(args, now)
		placeholder = "$" + strconv.Itoa(len(args))
	}
	args = append(args, size, offset)
	build := func(value *User, fields report.Fields) report.Projection {
		return listProjection(value, fields, placeholder)
	}
	items, err := report.ReadRecords(ctx, m.store.SQL(ctx), fields, build, " FROM t_user"+where+" ORDER BY id DESC LIMIT $"+strconv.Itoa(len(args)-1)+" OFFSET $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, err
	}
	for i := range items {
		item := &items[i]
		if fields.Has("metadata") {
			if err := json.Unmarshal(item.selectedMetadata, &item.Metadata); err != nil {
				return nil, fmt.Errorf("decode user metadata: %w", err)
			}
			if err := validateMetadataKeys(item.Metadata); err != nil {
				return nil, err
			}
		}
		if fields.Has("action_codes") || fields.Has("actions") {
			item.ActionCodes = m.actions.SplitCodes(item.selectedActions)
		}
	}
	if err := m.enrichList(ctx, items, fields); err != nil {
		return nil, err
	}
	if fields.Has("role_summary") && fields.Has("role") {
		for i := range items {
			if role := items[i].Role; role != nil {
				items[i].RoleSummary = &rolemodule.Summary{ID: role.ID, Name: role.Name, Word: role.Word}
			}
		}
	} else if fields.Has("role_summary") {
		ids := make([]int64, 0, len(items))
		for _, item := range items {
			if item.RoleID != nil {
				ids = append(ids, *item.RoleID)
			}
		}
		summaries, err := m.roles.LookupSummaries(ctx, m.store.SQL(ctx), ids)
		if err != nil {
			return nil, err
		}
		for i := range items {
			if items[i].RoleID != nil {
				summary := summaries[*items[i].RoleID]
				items[i].RoleSummary = &summary
			}
		}
	}
	return items, nil
}

func (m *Module) enrichList(ctx context.Context, items []User, fields report.Fields) error {
	roleIDs := make([]int64, 0, len(items))
	codes := make([]string, 0)
	for _, item := range items {
		if fields.Has("role") && item.RoleID != nil {
			roleIDs = append(roleIDs, *item.RoleID)
		}
		if fields.Has("actions") {
			codes = append(codes, item.ActionCodes...)
		}
	}
	var roles map[int64]rolemodule.Role
	var err error
	if len(roleIDs) > 0 {
		roles, err = m.roles.LookupRecords(ctx, m.store.SQL(ctx), roleIDs)
	}
	if err != nil {
		return err
	}
	// Follow page order so the batch input is deterministic even with shared roles.
	seenRoles := make(map[int64]struct{}, len(roles))
	for _, id := range roleIDs {
		if _, seen := seenRoles[id]; seen {
			continue
		}
		seenRoles[id] = struct{}{}
		codes = append(codes, roles[id].ActionCodes...)
	}
	var actions []actionmodule.Action
	if len(codes) > 0 {
		actions, err = m.actions.Lookup(ctx, m.store.SQL(ctx), codes)
	}
	if err != nil {
		return err
	}
	byCode := make(map[string]ActionView, len(actions))
	for _, item := range actionViews(actions) {
		byCode[item.Code] = item
	}
	for i := range items {
		item := &items[i]
		if fields.Has("actions") {
			item.Actions = orderedActions(item.ActionCodes, byCode)
		}
		if fields.Has("role") && item.RoleID != nil {
			record := roles[*item.RoleID]
			item.Role = roleView(&record)
			item.Role.Actions = orderedActions(record.ActionCodes, byCode)
		}
	}
	return nil
}
