package action

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/modules/localization"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/code"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var (
	ErrNotFound = apperror.New("ACTION_NOT_FOUND", "action not found")
	ErrIDs      = apperror.New("ACTION_INVALID_IDS", "provide 1-100 positive action ids")
	ErrInvalid  = apperror.New("ACTION_INVALID", "invalid action")
	ErrConflict = apperror.New("ACTION_CONFLICT", "action code or word already exists")
)

const AllPermissionsActionID int64 = 1

type Action struct {
	selectedFields report.Fields
	ID             int64             `json:"id"`
	CreatedAt      int64             `json:"created_at"`
	UpdatedAt      int64             `json:"updated_at"`
	Code           string            `json:"code"`
	Name           string            `json:"name"`
	Names          map[string]string `json:"names,omitempty"`
	Group          string            `json:"group"`
	Word           string            `json:"word"`
	Scope          string            `json:"scope"`
	Config         Config            `json:"config"`
	// These display-only projections keep older list consumers working. They are never accepted on writes.
	Resource string `json:"resource"`
	Menu     string `json:"menu"`
	Button   string `json:"button"`
}

type CreateActionInput struct {
	Names  map[string]string `json:"names,omitempty"`
	Group  string            `json:"group,omitempty"`
	Word   string            `json:"word" example:"admin:read:system:user"`
	Config Config            `json:"config"`
}

type UpdateActionInput struct {
	Names  map[string]string `json:"names,omitempty"`
	Group  *string           `json:"group,omitempty"`
	Word   *string           `json:"word,omitempty"`
	Config *Config           `json:"config,omitempty"`
}

type ListActionsInput struct {
	Fields      report.Fields
	Q           string
	Scope       string
	GroupsExact []string
	GroupExact  *string
	Code        string
	Name        string
	Group       string
	Word        string
	Resource    string
	Menu        string
	Button      string
	Page        *int32
	PageSize    *int32
}

type ListActionsResult struct {
	Items    []Action `json:"items"`
	Total    int64    `json:"t"`
	Page     int32    `json:"p"`
	PageSize int32    `json:"s"`
}

type ListActionGroupsResult struct {
	Items []string `json:"items"`
}

type CatalogSource interface {
	Current() localization.Catalog
	Select([]string) localization.Catalog
}

type Module struct {
	definitions      DefinitionCatalog
	catalogSource    CatalogSource
	nameLocalization NameLocalization
	administration   PermissionAdministration
	store            *db.Store
	limits           pagination.Limits
	protectSuper     bool
}

func New(store *db.Store, limits pagination.Limits, protectSuper bool, catalog CatalogSource) *Module {
	return &Module{catalogSource: catalog, store: store, limits: limits, protectSuper: protectSuper}
}

func (m *Module) Get(ctx context.Context, id int64) (*Action, error) {
	return m.LookupByID(ctx, m.store.SQL(ctx), id)
}

func (m *Module) LookupByID(ctx context.Context, executor db.SQLExecutor, id int64) (*Action, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	return m.loadAction(ctx, executor, id)
}

const actionColumns = "id, created_at, updated_at, code, action_group, word, scope, config"

func (m *Module) loadAction(ctx context.Context, executor db.SQLExecutor, id int64) (*Action, error) {
	value, err := scan(executor.QueryRowContext(ctx, "SELECT "+actionColumns+" FROM t_action WHERE id = $1", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err == nil {
		catalog := m.catalogSource.Current()
		value.deriveDisplay(catalog)
		value.localizeName(catalog, i18n.Locale(ctx, ""))
		value.loadNames(catalog)
	}
	return value, err
}

func (m *Module) Create(ctx context.Context, input CreateActionInput) (*Action, error) {
	input.Word = strings.TrimSpace(input.Word)
	input.Group = strings.TrimSpace(input.Group)
	if input.Group == "" {
		input.Group = GroupForConfig(input.Word, input.Config)
	}
	if !validIdentity(input.Word, input.Group) || input.Config.Builtin != "" {
		return nil, ErrInvalid
	}
	if err := input.Config.Validate(input.Word); err != nil {
		return nil, err
	}
	var value *Action
	var dictionaryID int64
	err := m.store.Tx(ctx, func(ctx context.Context) error {
		if err := authn.RequirePermissionOperation(ctx, m.administration, "action", "create"); err != nil {
			return err
		}
		catalog, err := m.requireRegisteredDefinition(ctx, input.Word, input.Group, input.Config)
		if err != nil {
			return err
		}
		if err := resolveStoredConfig(catalog, input.Word, &input.Config); err != nil {
			return err
		}
		if input.Config.PermissionScope == "*" {
			if err := authn.RequireCatalogAdministration(ctx, m.administration); err != nil {
				return err
			}
		}

		encoded, err := json.Marshal(input.Config)
		if err != nil {
			return err
		}
		var id int64
		if err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT nextval('t_action_id_seq')`).Scan(&id); err != nil {
			return err
		}
		now := time.Now().UnixMilli()
		inserted := false
		for range 5 {
			actionCode, err := code.Generate()
			if err != nil {
				return err
			}
			result, err := m.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_action (id, created_at, updated_at, code, action_group, word, config) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT ON CONSTRAINT uk_action_code DO NOTHING`, id, now, now, actionCode, input.Group, input.Word, string(encoded))
			if err != nil {
				return mapWriteError(err)
			}
			count, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if count == 1 {
				inserted = true
				break
			}
		}
		if !inserted {
			return errors.New("generate unique action code")
		}
		dictionaryID, err = m.writeNames(ctx, "", input.Word, input.Names)
		if err != nil {
			return err
		}
		value, err = m.loadAction(ctx, m.store.SQL(ctx), id)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("create action: %w", err)
	}
	m.refreshNames(ctx, dictionaryID, value)
	return value, nil
}

func (m *Module) Update(ctx context.Context, id int64, input UpdateActionInput) (*Action, error) {
	if id <= 0 || allNil(input) {
		return nil, ErrInvalid
	}
	if m.protectSuper && id == AllPermissionsActionID {
		return nil, apperror.FeatureDisabled
	}
	if input.Word != nil && !ValidWord(strings.TrimSpace(*input.Word)) {
		return nil, ErrInvalid
	}
	var value *Action
	var dictionaryID int64
	err := m.store.Tx(ctx, func(ctx context.Context) error {
		if err := authn.RequirePermissionOperation(ctx, m.administration, "action", "update"); err != nil {
			return err
		}
		current, err := scan(m.store.SQL(ctx).QueryRowContext(ctx, "SELECT "+actionColumns+" FROM t_action WHERE id = $1 FOR UPDATE", id))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if current.Config.Builtin == "all" || current.Config.Builtin == "default" || current.Config.PermissionScope == "*" {
			if err := authn.RequireCatalogAdministration(ctx, m.administration); err != nil {
				return err
			}
		}
		word, group, config := current.Word, current.Group, current.Config
		if input.Word != nil {
			word = strings.TrimSpace(*input.Word)
		}
		if input.Config != nil {
			config = *input.Config
		}
		if input.Group != nil {
			group = strings.TrimSpace(*input.Group)
		}
		if !validIdentity(word, group) || config.Builtin != current.Config.Builtin {
			return ErrInvalid
		}
		catalog, err := m.requireRegisteredDefinition(ctx, word, group, config)
		if err != nil {
			return err
		}
		if err := config.Validate(word); err != nil {
			return err
		}
		if err := resolveStoredConfig(catalog, word, &config); err != nil {
			return err
		}
		if config.PermissionScope == "*" {
			if err := authn.RequireCatalogAdministration(ctx, m.administration); err != nil {
				return err
			}
		}

		encoded, err := json.Marshal(config)
		if err != nil {
			return err
		}
		result, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_action SET action_group=$1, word=$2, config=$3, updated_at=$4 WHERE id=$5`, group, word, string(encoded), time.Now().UnixMilli(), id)
		if err != nil {
			return mapWriteError(err)
		}
		if _, err = result.RowsAffected(); err != nil {
			return err
		}
		dictionaryID, err = m.writeNames(ctx, current.Word, word, input.Names)
		if err != nil {
			return err
		}
		value, err = m.loadAction(ctx, m.store.SQL(ctx), id)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("update action: %w", err)
	}
	m.refreshNames(ctx, dictionaryID, value)
	return value, nil
}

func (m *Module) Delete(ctx context.Context, ids ...int64) error {
	ids, err := normalizeIDs(ids)
	if err != nil {
		return err
	}
	if m.protectSuper {
		for _, id := range ids {
			if id == AllPermissionsActionID {
				return apperror.FeatureDisabled
			}
		}
	}

	operation := func(ctx context.Context) error {
		if err := authn.RequirePermissionOperation(ctx, m.administration, "action", "delete"); err != nil {
			return err
		}
		if m.administration != nil {
			if err := authn.RequireAllPermissions(ctx, m.administration); err != nil {
				if !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
					return err
				}
				var all, defaults bool
				if queryErr := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT COALESCE(BOOL_OR(config->>'builtin'='all' OR config->>'permission_scope'='*'),false),COALESCE(BOOL_OR(config->>'builtin'='default'),false) FROM t_action WHERE id = ANY($1)`, pq.Array(ids)).Scan(&all, &defaults); queryErr != nil {
					return queryErr
				}
				if all || defaults {
					if err := authn.RequireCatalogAdministration(ctx, m.administration); err != nil {
						return err
					}
				}
			}
		}
		var count int64
		if err := m.store.SQL(ctx).QueryRowContext(ctx, deleteAssignedActionsSQL, pq.Array(ids), time.Now().UnixMilli()).Scan(&count); err != nil {
			return fmt.Errorf("delete action and assignments: %w", err)
		}
		if count == 0 {
			return ErrNotFound
		}
		return nil
	}
	if m.administration != nil {
		return m.store.Tx(ctx, operation)
	}
	return operation(ctx)
}

func (m *Module) List(ctx context.Context, input ListActionsInput) (*ListActionsResult, error) {
	if err := input.Fields.Validate(listFields...); err != nil {
		return nil, err
	}
	page, size, inRange := m.limits.Normalize(input.Page, input.PageSize)
	value := &ListActionsResult{Items: make([]Action, 0), Page: page, PageSize: size}
	if !inRange {
		return value, nil
	}
	mappingFilter := strings.TrimSpace(input.Resource) != "" || strings.TrimSpace(input.Menu) != "" || strings.TrimSpace(input.Button) != ""
	needsMappings := mappingFilter || input.Fields.Has("config") || input.Fields.Has("resource") || input.Fields.Has("menu") || input.Fields.Has("button")
	needsNames := input.Fields.Has("name") || strings.TrimSpace(input.Q) != "" || strings.TrimSpace(input.Name) != ""
	needsGroups := strings.TrimSpace(input.Group) != ""
	var catalog localization.Catalog
	var labels actionNameLabels
	var filterLabels, groupLabels map[string]string
	if needsMappings || needsNames || needsGroups {
		selected := report.Fields{}
		if needsMappings {
			selected = append(selected, "resource_catalog")
		}
		if needsNames || needsGroups {
			selected = append(selected, "messages", "default_locale")
		}
		catalog = m.catalogSource.Select(selected)
	}
	if needsNames {
		labels = newActionNameLabels(catalog, i18n.Locale(ctx, ""))
	}
	if needsGroups {
		groupLabels = catalog.Labels(i18n.Locale(ctx, ""), "groups")
	}
	if strings.TrimSpace(input.Q) != "" || strings.TrimSpace(input.Name) != "" {
		var err error
		filterLabels, err = m.nameFilterLabels(ctx, m.store.SQL(ctx), input, labels, groupLabels)
		if err != nil {
			return nil, fmt.Errorf("load action filter names: %w", err)
		}
	}
	where, args := actionFilters(input, filterLabels, groupLabels)
	var resolved map[string]Config
	if mappingFilter {
		var err error
		resolved, err = m.matchingMappings(ctx, input, catalog, where, args)
		if err != nil {
			return nil, err
		}
		if len(resolved) == 0 {
			return value, nil
		}
		words := make([]string, 0, len(resolved))
		for word := range resolved {
			words = append(words, word)
		}
		sort.Strings(words)
		args = append(args, pq.Array(words))
		if where == "" {
			where = " WHERE "
		} else {
			where += " AND "
		}
		where += "word = ANY($" + strconv.Itoa(len(args)) + ")"
	}
	if err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT COUNT(*) FROM t_action"+where, args...).Scan(&value.Total); err != nil {
		return nil, fmt.Errorf("count actions: %w", err)
	}
	offset := int64(page-1) * int64(size)
	if offset >= value.Total {
		return value, nil
	}
	if input.Fields != nil {
		selectedArgs := append(args, size, offset)
		items, err := report.ReadRecords(ctx, m.store.SQL(ctx), input.Fields, listProjection, " FROM t_action"+where+" ORDER BY id DESC LIMIT $"+strconv.Itoa(len(selectedArgs)-1)+" OFFSET $"+strconv.Itoa(len(selectedArgs)), selectedArgs...)
		if err == nil {
			for index := range items {
				if input.Fields.Has("config") || input.Fields.Has("resource") || input.Fields.Has("menu") || input.Fields.Has("button") {
					items[index].deriveSelectedDisplay(catalog, input.Fields, resolved)
				}
				if input.Fields.Has("name") {
					items[index].Name = labels.name(items[index].Word, items[index].Config)
				}
			}
		}
		value.Items = items
		return value, err
	}
	args = append(args, size, offset)
	query := "SELECT " + actionColumns + " FROM t_action" + where +
		" ORDER BY id DESC LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list actions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		item.deriveSelectedDisplay(catalog, input.Fields, resolved)
		item.Name = labels.name(item.Word, item.Config)
		value.Items = append(value.Items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list actions: %w", err)
	}
	return value, nil
}

func (m *Module) ListGroups(ctx context.Context, keyword string) (*ListActionGroupsResult, error) {
	keyword = strings.TrimSpace(keyword)
	if utf8.RuneCountInString(keyword) > 50 {
		return nil, ErrInvalid
	}
	value := &ListActionGroupsResult{Items: make([]string, 0)}
	query := `SELECT MIN(action_group) FROM t_action WHERE BTRIM(action_group) <> ''`
	args := make([]any, 0, 1)
	if keyword != "" {
		args = append(args, like(keyword))
		clause := `LOWER(action_group) LIKE LOWER($1) ESCAPE '!'`
		if matched := matchingLabelWords(m.catalogSource.Current().Labels(i18n.Locale(ctx, ""), "groups"), keyword); len(matched) > 0 {
			args = append(args, pq.Array(matched))
			clause += " OR action_group = ANY($2)"
		}
		query += " AND (" + clause + ")"
	}
	query += ` GROUP BY LOWER(action_group) ORDER BY LOWER(action_group) LIMIT 20`
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list action groups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var group string
		if err := rows.Scan(&group); err != nil {
			return nil, fmt.Errorf("scan action group: %w", err)
		}
		value.Items = append(value.Items, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list action groups: %w", err)
	}
	return value, nil
}

type scanner interface{ Scan(...any) error }

func scan(row scanner) (*Action, error) {
	var value Action
	if err := row.Scan(&value.ID, &value.CreatedAt, &value.UpdatedAt, &value.Code, &value.Group, &value.Word, &value.Scope, &value.Config); err != nil {
		return nil, fmt.Errorf("scan action: %w", err)
	}
	return &value, nil
}

func (value *Action) deriveDisplay(catalog localization.Catalog) {
	value.deriveSelectedDisplay(catalog, nil, nil)
}

func (m *Module) Lookup(ctx context.Context, executor db.SQLExecutor, codes []string) ([]Action, error) {
	codes = NormalizeCodes(codes)
	result := make([]Action, 0, len(codes))
	if len(codes) == 0 {
		return result, nil
	}
	rows, err := executor.QueryContext(ctx, "SELECT "+actionColumns+" FROM t_action WHERE code = ANY($1)", pq.Array(codes))
	if err != nil {
		return nil, fmt.Errorf("lookup actions: %w", err)
	}
	defer rows.Close()
	catalog := m.catalogSource.Current()
	labels := newActionNameLabels(catalog, i18n.Locale(ctx, ""))
	byCode := make(map[string]Action, len(codes))
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		item.deriveDisplay(catalog)
		item.Name = labels.name(item.Word, item.Config)
		byCode[item.Code] = *item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lookup actions: %w", err)
	}
	for _, code := range codes {
		if item, ok := byCode[code]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func (m *Module) ValidateCodes(ctx context.Context, executor db.SQLExecutor, codes []string) ([]string, error) {
	codes = NormalizeCodes(codes)
	items, err := m.Lookup(ctx, executor, codes)
	if err != nil {
		return nil, err
	}
	if len(items) != len(codes) {
		return nil, ErrNotFound
	}
	return codes, nil
}

func NormalizeCodes(codes []string) []string {
	result := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	return result
}

func (m *Module) SplitCodes(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	return NormalizeCodes(strings.Split(value, ","))
}

func actionFilters(input ListActionsInput, labels, groupLabels map[string]string) (string, []any) {
	parts := make([]string, 0, 7)
	args := make([]any, 0, 7)
	if strings.TrimSpace(input.Q) != "" {
		args = append(args, like(input.Q))
		clause := "LOWER(word) LIKE LOWER($1) ESCAPE '!' OR LOWER(code) LIKE LOWER($1) ESCAPE '!'"
		if matched := matchingLabelWords(labels, input.Q); len(matched) > 0 {
			args = append(args, pq.Array(matched))
			clause += " OR word = ANY($" + strconv.Itoa(len(args)) + ")"
		}
		parts = append(parts, "("+clause+")")
	}
	code := strings.TrimSpace(input.Code)
	if code != "" {
		if strings.Contains(code, ",") {
			args = append(args, pq.Array(NormalizeCodes(strings.Split(code, ","))))
			parts = append(parts, "code = ANY($"+strconv.Itoa(len(args))+")")
		} else {
			args = append(args, like(code))
			parts = append(parts, "LOWER(code) LIKE LOWER($"+strconv.Itoa(len(args))+") ESCAPE '!'")
		}
	}
	if strings.TrimSpace(input.Name) != "" {
		args = append(args, like(input.Name))
		clause := "LOWER(word) LIKE LOWER($" + strconv.Itoa(len(args)) + ") ESCAPE '!'"
		known := make([]string, 0, len(labels))
		for word := range labels {
			known = append(known, word)
		}
		sort.Strings(known)
		if len(known) > 0 {
			args = append(args, pq.Array(known))
			clause = "(" + clause + " AND NOT (word = ANY($" + strconv.Itoa(len(args)) + ")))"
		}
		if matched := matchingLabelWords(labels, input.Name); len(matched) > 0 {
			args = append(args, pq.Array(matched))
			clause += " OR word = ANY($" + strconv.Itoa(len(args)) + ")"
		}
		parts = append(parts, "("+clause+")")
	}
	fields := []struct {
		column, value string
		multiple      bool
	}{
		{"action_group", input.Group, true}, {"word", input.Word, true},
	}
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		if field.multiple {
			addMultiLikeFilter(&parts, &args, field.column, field.value)
			if field.column == "action_group" {
				matched := []string{}
				for _, keyword := range splitFilterValues(field.value) {
					matched = append(matched, matchingLabelWords(groupLabels, keyword)...)
				}
				matched = NormalizeCodes(matched)
				if len(matched) > 0 {
					args = append(args, pq.Array(matched))
					parts[len(parts)-1] = "(" + parts[len(parts)-1] + " OR action_group = ANY($" + strconv.Itoa(len(args)) + "))"
				}
			}
			continue
		}
		args = append(args, like(field.value))
		parts = append(parts, "LOWER("+field.column+") LIKE LOWER($"+strconv.Itoa(len(args))+") ESCAPE '!'")
	}
	if input.Scope != "" {
		args = append(args, input.Scope)
		parts = append(parts, "scope = $"+strconv.Itoa(len(args)))
	}
	if input.GroupExact != nil {
		args = append(args, *input.GroupExact)
		parts = append(parts, "action_group = $"+strconv.Itoa(len(args)))
	}
	if len(input.GroupsExact) > 0 {
		args = append(args, pq.Array(input.GroupsExact))
		parts = append(parts, "action_group = ANY($"+strconv.Itoa(len(args))+")")
	}

	if len(parts) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

func like(value string) string {
	return "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.TrimSpace(value)) + "%"
}

func addMultiLikeFilter(parts *[]string, args *[]any, column, value string) {
	values := splitFilterValues(value)
	if len(values) == 0 {
		return
	}
	if len(values) == 1 {
		*args = append(*args, like(values[0]))
		*parts = append(*parts, "LOWER("+column+") LIKE LOWER($"+strconv.Itoa(len(*args))+") ESCAPE '!'")
		return
	}
	patterns := make([]string, 0, len(values))
	for _, item := range values {
		patterns = append(patterns, like(item))
	}
	*args = append(*args, pq.Array(patterns))
	*parts = append(*parts, "EXISTS (SELECT 1 FROM unnest($"+strconv.Itoa(len(*args))+"::text[]) AS candidate(value) WHERE LOWER("+column+") LIKE LOWER(candidate.value) ESCAPE '!')")
}

func splitFilterValues(value string) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func allNil(input UpdateActionInput) bool {
	return input.Group == nil && input.Word == nil && input.Config == nil && input.Names == nil
}

func normalizeIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, ErrIDs
	}
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrIDs
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func mapWriteError(err error) error {
	var target *pq.Error
	if errors.As(err, &target) {
		if target.Code == "23505" {
			return ErrConflict
		}
		if target.Code == "23514" {
			return ErrInvalid
		}
	}
	return err
}

func matchingLabelWords(labels map[string]string, keyword string) []string {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	matched := []string{}
	for word, label := range labels {
		if strings.Contains(strings.ToLower(label), keyword) {
			matched = append(matched, word)
		}
	}
	sort.Strings(matched)
	return matched
}

type PermissionAdministration interface {
	RequireAllPermissions(context.Context) error
}

func (m *Module) SetPermissionAdministration(policy PermissionAdministration) {
	m.administration = policy
}
