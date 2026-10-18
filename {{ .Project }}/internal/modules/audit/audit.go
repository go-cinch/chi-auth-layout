package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var (
	ErrFilter      = apperror.New("AUDIT_FILTER_INVALID", "invalid audit filter or time range (maximum 90 days)")
	ErrNotFound    = apperror.New("AUDIT_NOT_FOUND", "audit event not found")
	ErrExportLimit = apperror.New("AUDIT_EXPORT_LIMIT", "more than 10000 events; narrow the filters before exporting")
)

type Module struct {
	authHTTPPath string
	store        *db.Store
	limits       pagination.Limits
}

func New(store *db.Store, limits pagination.Limits) *Module {
	return &Module{store: store, limits: limits}
}

type Event struct {
	selectedFields report.Fields
	Header         json.RawMessage `json:"header"`
	Operation      string          `json:"operation"`
	ResourceType   string          `json:"resource_type"`
	ResourceID     string          `json:"resource_id"`
	RequestParams  json.RawMessage `json:"request_params"`
	ResponseParams json.RawMessage `json:"response_params"`
	ID             int64           `json:"id"`

	Kind       string `json:"kind"`
	UserID     int64  `json:"user_id"`
	Username   string `json:"username"`
	Module     string `json:"module"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	IP         string `json:"ip"`
	UserAgent  string `json:"user_agent"`
	TraceID    string `json:"trace_id"`
	ErrorCode  string `json:"error_code"`
	Status     int    `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	OccurredAt int64  `json:"occurred_at"`
}
type Filter struct {
	Fields   report.Fields
	Username string
	IP       string
	Path     string
	Sort     string
	Weekday  *int32
	Hour     *int32
	Kind     string
	Query    string
	Module   string
	Result   string
	From     int64
	To       int64
	Timezone string
	Page     *int32
	Size     *int32
}

type Page struct {
	Items []Event `json:"items"`
	Total int64   `json:"t"`
	Page  int32   `json:"p"`
	Size  int32   `json:"s"`
}

func clean(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}

func (m *Module) RecordHTTP(ctx context.Context, event auditctx.Event) error {
	if (event.Kind != "login" && event.Kind != "operation") || event.UserID < 0 {
		return ErrFilter
	}
	if event.ResourceID == "" && event.Kind == "operation" {
		if event.Module == "auth" && event.ResourceType == "user" && event.UserID > 0 {
			event.ResourceID = strconv.FormatInt(event.UserID, 10)
		}
		if event.Operation == "create" && event.Status >= 200 && event.Status < 300 {
			var body struct {
				ID json.Number `json:"id"`
			}
			if json.Unmarshal(event.ResponseParams, &body) == nil {
				if id, err := body.ID.Int64(); err == nil && id > 0 {
					event.ResourceID = strconv.FormatInt(id, 10)
				}
			}
		}
	}
	_, err := m.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_audit_event
 (occurred_at,kind,user_id,username,module,method,path,ip,user_agent,trace_id,error_code,status,duration_ms,operation,resource_type,resource_id,request_params,response_params,header)
 VALUES ($1,$2,NULLIF($3,0),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		event.OccurredAt, event.Kind, event.UserID, clean(event.Username, 256), clean(event.Module, 64), clean(event.Method, 16), clean(event.Path, 1024), clean(event.IP, 128), clean(event.UserAgent, 512), clean(event.TraceID, 128), clean(event.ErrorCode, 128), event.Status, max(int64(0), event.DurationMS), clean(event.Operation, 32), clean(event.ResourceType, 64), clean(event.ResourceID, 1024), jsonValue(event.RequestParams), jsonValue(event.ResponseParams), jsonValue(event.Header))
	return err
}

func normalize(f Filter) (Filter, error) {
	if f.Kind != "login" && f.Kind != "operation" {
		return f, ErrFilter
	}
	if f.To == 0 {
		f.To = time.Now().UnixMilli()
	}
	if f.From == 0 {
		f.From = f.To - int64(7*24*time.Hour/time.Millisecond)
	}
	if f.From < 0 || f.To > 253402300799999 || f.To <= f.From || f.To-f.From > int64(90*24*time.Hour/time.Millisecond) {
		return f, ErrFilter
	}
	if f.Result != "" && f.Result != "success" && f.Result != "failure" {
		return f, ErrFilter
	}
	if len([]rune(f.Query)) > 256 || len(f.Module) > 64 || len([]rune(f.Username)) > 256 || len(f.IP) > 128 || len([]rune(f.Path)) > 1024 {
		return f, ErrFilter
	}
	if f.Sort != "" && f.Sort != "newest" && f.Sort != "failure" && f.Sort != "duration" {
		return f, ErrFilter
	}
	if (f.Weekday == nil) != (f.Hour == nil) {
		return f, ErrFilter
	}
	if f.Weekday != nil && (*f.Weekday < 0 || *f.Weekday > 6 || *f.Hour < 0 || *f.Hour > 23) {
		return f, ErrFilter
	}
	if f.Timezone == "" {
		f.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(f.Timezone); err != nil || f.Timezone == "Local" {
		return f, ErrFilter
	}
	return f, nil
}

func where(f Filter) (string, []any) {
	args := []any{f.Kind, f.From, f.To}
	s := " WHERE kind=$1 AND occurred_at >= $2 AND occurred_at < $3"
	if f.Query != "" {
		args = append(args, "%"+strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(f.Query)+"%")
		s += " AND (username ILIKE $4 OR ip ILIKE $4 OR path ILIKE $4 OR trace_id ILIKE $4)"
	}
	if f.Module != "" {
		args = append(args, f.Module)
		s += fmt.Sprintf(" AND module=$%d", len(args))
	}
	if f.Result == "success" {
		s += " AND status >= 200 AND status < 400"
	}
	if f.Result == "failure" {
		s += " AND (status < 200 OR status >= 400)"
	}

	for _, item := range []struct{ field, value string }{
		{"username", f.Username}, {"ip", f.IP}, {"path", f.Path},
	} {
		if item.value != "" {
			args = append(args, item.value)
			s += fmt.Sprintf(" AND %s=$%d", item.field, len(args))
		}
	}
	if f.Weekday != nil {
		args = append(args, f.Timezone, *f.Weekday, *f.Hour)
		local := fmt.Sprintf("(to_timestamp(occurred_at / 1000.0) AT TIME ZONE $%d)", len(args)-2)
		s += fmt.Sprintf(" AND EXTRACT(ISODOW FROM %s)::int-1=$%d AND EXTRACT(HOUR FROM %s)::int=$%d", local, len(args)-1, local, len(args))
	}
	return s, args
}

func order(f Filter) string {
	switch f.Sort {
	case "failure":
		return " ORDER BY (status < 200 OR status >= 400) DESC,occurred_at DESC,id DESC"
	case "duration":
		return " ORDER BY duration_ms DESC,occurred_at DESC,id DESC"
	default:
		return " ORDER BY occurred_at DESC,id DESC"
	}
}

const columns = "id,occurred_at,kind,COALESCE(user_id,0),username,module,method,path,ip,user_agent,trace_id,error_code,status,duration_ms,operation,resource_type,resource_id,request_params,response_params,header"

// List and export carry metadata only; bounded payloads are loaded by detail.
const listColumns = "id,occurred_at,kind,COALESCE(user_id,0),username,module,method,path,ip,user_agent,trace_id,error_code,status,duration_ms,operation,resource_type,resource_id,'null'::jsonb,'null'::jsonb,'null'::jsonb"

func scan(row interface{ Scan(...any) error }) (Event, error) {
	var e Event
	var at int64
	err := row.Scan(&e.ID, &at, &e.Kind, &e.UserID, &e.Username, &e.Module, &e.Method, &e.Path, &e.IP, &e.UserAgent, &e.TraceID, &e.ErrorCode, &e.Status, &e.DurationMS, &e.Operation, &e.ResourceType, &e.ResourceID, &e.RequestParams, &e.ResponseParams, &e.Header)
	e.OccurredAt = at
	return e, err
}

func (m *Module) List(ctx context.Context, f Filter) (*Page, error) {
	if err := f.Fields.Validate(listFields...); err != nil {
		return nil, err
	}
	f, err := normalize(f)
	if err != nil {
		return nil, err
	}
	p, s, valid := m.limits.Normalize(f.Page, f.Size)
	result := &Page{Items: []Event{}, Page: p, Size: s}
	if !valid {
		return result, nil
	}
	clause, args := where(f)
	if err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT COUNT(*) FROM t_audit_event"+clause, args...).Scan(&result.Total); err != nil {
		return nil, err
	}
	args = append(args, s, int64(p-1)*int64(s))
	if f.Fields != nil {
		items, err := report.ReadRecords(ctx, m.store.SQL(ctx), f.Fields, listProjection, " FROM t_audit_event"+clause+order(f)+fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
		result.Items = items
		return result, err
	}
	rows, err := m.store.SQL(ctx).QueryContext(ctx, "SELECT "+listColumns+" FROM t_audit_event"+clause+order(f)+fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, e)
	}
	return result, rows.Err()
}

func (m *Module) Get(ctx context.Context, kind string, id int64) (*Event, error) {
	if id <= 0 || (kind != "login" && kind != "operation") {
		return nil, ErrFilter
	}
	e, err := scan(m.store.SQL(ctx).QueryRowContext(ctx, "SELECT "+columns+" FROM t_audit_event WHERE kind=$1 AND id=$2", kind, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (m *Module) Options(ctx context.Context, f Filter) ([]string, error) {
	f, err := normalize(f)
	if err != nil {
		return nil, err
	}
	clause, args := where(f)
	rows, err := m.store.SQL(ctx).QueryContext(ctx, "SELECT DISTINCT username FROM t_audit_event"+clause+" AND username <> '' ORDER BY username LIMIT 10", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

func (m *Module) Export(ctx context.Context, f Filter) ([]Event, error) {
	f, err := normalize(f)
	if err != nil {
		return nil, err
	}
	clause, args := where(f)
	rows, err := m.store.SQL(ctx).QueryContext(ctx, "SELECT "+listColumns+" FROM t_audit_event"+clause+order(f)+" LIMIT 10001", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Event{}
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, e)
		if len(items) > 10000 {
			return nil, ErrExportLimit
		}
	}
	return items, rows.Err()
}

func jsonValue(value json.RawMessage) string {
	if !json.Valid(value) {
		return "null"
	}
	return string(value)
}
