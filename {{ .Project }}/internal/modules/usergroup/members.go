package usergroup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

func groupFields(fields report.Fields) (report.Fields, error) {
	if err := fields.Validate("actions", "users", "member_count", "member_preview"); err != nil {
		return nil, err
	}
	if fields == nil {
		return report.Fields{"actions", "users"}, nil
	}
	return fields, nil
}

func (m *Module) enrich(ctx context.Context, executor db.SQLExecutor, items []UserGroup, fields report.Fields) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(items))
	codes := []string{}
	for i := range items {
		items[i].fields = fields
		ids = append(ids, items[i].ID)
		codes = append(codes, items[i].ActionCodes...)
	}
	if fields.Has("actions") {
		actions, err := m.actions.Lookup(ctx, executor, codes)
		if err != nil {
			return err
		}
		byCode := map[string]ActionView{}
		for _, item := range actionViews(actions) {
			byCode[item.Code] = item
		}
		for i := range items {
			items[i].Actions = orderedActions(items[i].ActionCodes, byCode)
		}
	}
	if fields.Has("users") {
		members, err := m.users.SummariesByGroups(ctx, executor, ids)
		if err != nil {
			return err
		}
		for i := range items {
			items[i].Users = userSummaries(members[items[i].ID])
			previewFromUsers(&items[i], fields)
		}
		return nil
	}
	if !fields.Has("member_count") && !fields.Has("member_preview") {
		return nil
	}
	expressions := []string{"g.id"}
	if fields.Has("member_count") {
		expressions = append(expressions, `(SELECT COUNT(*) FROM t_user_user_group_relation r WHERE r.user_group_id = g.id)`)
	}
	if fields.Has("member_preview") {
		expressions = append(expressions, `COALESCE((SELECT jsonb_agg(p ORDER BY p.id) FROM (
   SELECT u.id, u.username, u.code FROM t_user_user_group_relation r
   JOIN t_user u ON u.id = r.user_id WHERE r.user_group_id = g.id
   ORDER BY r.user_id LIMIT 5
  ) p), '[]'::jsonb)`)
	}
	// One statement for the whole page; count and preview share a database snapshot.
	rows, err := executor.QueryContext(ctx, "SELECT "+strings.Join(expressions, ", ")+" FROM unnest($1::bigint[]) AS g(id)", pq.Array(ids))
	if err != nil {
		return fmt.Errorf("query member previews: %w", err)
	}
	defer rows.Close()
	byID := make(map[int64]*UserGroup, len(items))
	for i := range items {
		byID[items[i].ID] = &items[i]
	}
	for rows.Next() {
		var id, count int64
		var data []byte
		targets := []any{&id}
		if fields.Has("member_count") {
			targets = append(targets, &count)
		}
		if fields.Has("member_preview") {
			targets = append(targets, &data)
		}
		if err := rows.Scan(targets...); err != nil {
			return err
		}
		item := byID[id]
		if item == nil {
			return fmt.Errorf("unexpected user group %d", id)
		}
		if fields.Has("member_count") {
			item.MemberCount = &count
		}
		if fields.Has("member_preview") {
			preview := []UserSummary{}
			if err := json.Unmarshal(data, &preview); err != nil {
				return err
			}
			item.MemberPreview = &preview
		}
	}
	return rows.Err()
}

type ListMembersInput struct {
	Page     *int32
	PageSize *int32
	Q        string
}

type ListMembersResult struct {
	Items    []UserSummary `json:"items"`
	Total    int64         `json:"t"`
	Page     int32         `json:"p"`
	PageSize int32         `json:"s"`
}

// ListMembers uses the same filters for the count and page, in ascending user ID order.
func (m *Module) ListMembers(ctx context.Context, id int64, input ListMembersInput) (*ListMembersResult, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	page, size, valid := m.limits.Normalize(input.Page, input.PageSize)
	result := &ListMembersResult{Items: []UserSummary{}, Page: page, PageSize: size}
	if !valid {
		return result, nil
	}
	var found int64
	if err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT id FROM t_user_group WHERE id = $1", id).Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	where := " WHERE EXISTS" + " (SELECT 1 FROM t_user_user_group_relation r WHERE r.user_group_id = $1 AND r.user_id = u.id)"
	args := []any{id}
	if q := strings.TrimSpace(input.Q); q != "" {
		where += " AND (u.username ILIKE $2 ESCAPE '!' OR u.code ILIKE $2 ESCAPE '!')"
		args = append(args, like(q))
	}
	if err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT COUNT(*) FROM t_user u"+where, args...).Scan(&result.Total); err != nil {
		return nil, err
	}
	offset := int64(page-1) * int64(size)
	if offset >= result.Total {
		return result, nil
	}
	args = append(args, size, offset)
	rows, err := m.store.SQL(ctx).QueryContext(ctx, fmt.Sprintf("SELECT u.id, u.username, u.code FROM t_user u%s ORDER BY u.id LIMIT $%d OFFSET $%d", where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item UserSummary
		if err := rows.Scan(&item.ID, &item.Username, &item.Code); err != nil {
			return nil, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

func memberDelta(input UpdateUserGroupInput) ([]int64, []int64, error) {
	if input.UserIDs != nil && (input.AddUserIDs != nil || input.RemoveUserIDs != nil) {
		return nil, nil, ErrInvalid
	}
	normalize := func(values *[]int64) ([]int64, error) {
		result := []int64{}
		if values == nil {
			return result, nil
		}
		seen := map[int64]bool{}
		for _, id := range *values {
			if id <= 0 {
				return nil, ErrInvalid
			}
			if !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
		sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
		return result, nil
	}
	added, err := normalize(input.AddUserIDs)
	if err != nil {
		return nil, nil, err
	}
	removed, err := normalize(input.RemoveUserIDs)
	if err != nil {
		return nil, nil, err
	}
	seen := map[int64]bool{}
	for _, id := range added {
		seen[id] = true
	}
	for _, id := range removed {
		if seen[id] {
			return nil, nil, ErrInvalid
		}
	}
	return added, removed, nil
}

func applyMemberDelta(ctx context.Context, executor db.SQLExecutor, id int64, added, removed []int64) error {
	if len(removed) > 0 {
		if _, err := executor.ExecContext(ctx, "DELETE FROM t_user_user_group_relation WHERE user_group_id = $1 AND user_id = ANY($2)", id, pq.Array(removed)); err != nil {
			return err
		}
	}
	if len(added) > 0 {
		_, err := executor.ExecContext(ctx, `INSERT INTO t_user_user_group_relation (user_group_id, user_id)
   SELECT $1, user_id FROM unnest($2::bigint[]) AS users(user_id)
   ON CONFLICT (user_id, user_group_id) DO NOTHING`, id, pq.Array(added))
		if err != nil {
			var pgError *pq.Error
			if errors.As(err, &pgError) && pgError.Code == "23503" {
				return ErrUserNotFound
			}
			return err
		}
	}
	return nil
}

// When full members are explicitly selected, derive previews without a second member query.
func previewFromUsers(item *UserGroup, fields report.Fields) {
	if fields.Has("member_count") {
		count := int64(len(item.Users))
		item.MemberCount = &count
	}
	if fields.Has("member_preview") {
		preview := item.Users[:min(5, len(item.Users))]
		item.MemberPreview = &preview
	}
}
