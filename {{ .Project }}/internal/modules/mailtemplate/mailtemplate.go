package mailtemplate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var (
	ErrInvalid   = apperror.New("MAIL_TEMPLATE_INVALID", "invalid email template")
	ErrNotFound  = apperror.New("MAIL_TEMPLATE_NOT_FOUND", "email template not found")
	ErrDuplicate = apperror.New("MAIL_TEMPLATE_DUPLICATE", "template ID already exists")
	ErrConflict  = apperror.New("MAIL_TEMPLATE_CONFLICT", "template changed; reload before saving")
)

var templateIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,99}$`)
var parameterPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)
var parameterNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

type Translation struct {
	Subject       string            `json:"subject"`
	HTML          string            `json:"html"`
	Text          string            `json:"text"`
	PreviewValues map[string]string `json:"preview_values"`
}

type Content map[string]Translation

type Record struct {
	ID               int64   `json:"id"`
	CreatedAt        int64   `json:"created_at"`
	UpdatedAt        int64   `json:"updated_at"`
	TemplateID       string  `json:"template_id"`
	Name             string  `json:"name"`
	Category         string  `json:"category"`
	Revision         int64   `json:"revision"`
	PublishedVersion int64   `json:"published_version"`
	HasDraft         bool    `json:"has_draft"`
	Content          Content `json:"content,omitempty"`
}

type Version struct {
	ID          int64   `json:"id"`
	CreatedAt   int64   `json:"created_at"`
	Version     int64   `json:"version"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	PublishedBy int64   `json:"published_by"`
	Content     Content `json:"content,omitempty"`
}

type CreateInput struct {
	TemplateID string  `json:"template_id"`
	Name       string  `json:"name"`
	Category   string  `json:"category"`
	Content    Content `json:"content"`
}

type UpdateInput struct {
	DiscardDraft *bool    `json:"discard_draft"`
	TemplateID   *string  `json:"template_id"`
	Revision     int64    `json:"revision"`
	Name         *string  `json:"name"`
	Category     *string  `json:"category"`
	Content      *Content `json:"content"`
}

type PublishInput struct {
	Revision int64 `json:"revision"`
}

type ListInput struct {
	P        *int32
	S        *int32
	Q        string
	Category string
}

type Page struct {
	Items []Record `json:"items"`
	T     int64    `json:"t"`
	P     int32    `json:"p"`
	S     int32    `json:"s"`
}

type Module struct {
	store  *db.Store
	limits pagination.Limits
	sender TestSender
}

func New(store *db.Store, limits pagination.Limits) *Module {
	return &Module{store: store, limits: limits}
}

// Parameters are data placeholders, never executable template expressions.
func Parameters(source string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, match := range parameterPattern.FindAllStringSubmatch(source, -1) {
		if !seen[match[1]] {
			result = append(result, match[1])
			seen[match[1]] = true
		}
	}
	return result
}

func normalizePreviewValues(content Content) {
	for locale, value := range content {
		if value.PreviewValues == nil {
			value.PreviewValues = map[string]string{}
			content[locale] = value
		}
	}
}

func validCategory(value string) bool {
	switch value {
	case "verification", "confirmation", "security", "notification":
		return true
	}
	return false
}

func validate(name, category string, content Content, publishing bool) error {
	if strings.TrimSpace(name) == "" || len([]rune(name)) > 100 || !validCategory(category) || len(content) != 2 {
		return ErrInvalid
	}
	for _, locale := range []string{"zh-CN", "en-US"} {
		value, ok := content[locale]
		if !ok || len(value.Subject) > 500 || strings.ContainsAny(value.Subject, "\r\n") || len(value.HTML) > 200000 || len(value.Text) > 100000 || len(value.PreviewValues) > 100 {
			return ErrInvalid
		}
		if publishing && (strings.TrimSpace(value.Subject) == "" || strings.TrimSpace(value.HTML) == "") {
			return ErrInvalid
		}
		for _, source := range []string{value.Subject, value.HTML, value.Text} {
			rest := parameterPattern.ReplaceAllString(source, "")
			if strings.Contains(rest, "{"+"{") {
				return ErrInvalid
			}
		}
		for key, example := range value.PreviewValues {
			if !parameterNamePattern.MatchString(key) || len(key) > 100 || len(example) > 10000 {
				return ErrInvalid
			}
		}
	}
	return nil
}

const recordColumns = "id, created_at, updated_at, template_id, name, category, revision, published_version, has_draft"

type scanner interface{ Scan(...any) error }

func readRecord(row scanner, content bool) (Record, error) {
	var result Record
	var raw []byte
	targets := []any{&result.ID, &result.CreatedAt, &result.UpdatedAt, &result.TemplateID, &result.Name, &result.Category, &result.Revision, &result.PublishedVersion, &result.HasDraft}
	if content {
		targets = append(targets, &raw)
	}
	err := row.Scan(targets...)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrNotFound
	}
	if err == nil && content {
		err = json.Unmarshal(raw, &result.Content)
		normalizePreviewValues(result.Content)
	}
	return result, err
}

func (m *Module) Get(ctx context.Context, id int64) (Record, error) {
	if id <= 0 {
		return Record{}, ErrInvalid
	}
	return readRecord(m.store.SQL(ctx).QueryRowContext(ctx, "SELECT "+recordColumns+", content FROM t_mail_template WHERE id=$1", id), true)
}

func (m *Module) List(ctx context.Context, input ListInput) (Page, error) {
	p, s, valid := m.limits.Normalize(input.P, input.S)
	result := Page{Items: []Record{}, P: p, S: s}
	if !valid {
		return result, nil
	}
	if input.Category != "" && !validCategory(input.Category) {
		return result, ErrInvalid
	}
	q := strings.TrimSpace(input.Q)
	q = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q)
	where := ` FROM t_mail_template WHERE ($1='' OR name ILIKE '%'||$1||'%' OR template_id ILIKE '%'||$1||'%') AND ($2='' OR category=$2)`
	if err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT count(*)"+where, q, input.Category).Scan(&result.T); err != nil {
		return result, err
	}
	rows, err := m.store.SQL(ctx).QueryContext(ctx, "SELECT "+recordColumns+where+" ORDER BY id DESC LIMIT $3 OFFSET $4", q, input.Category, s, int64(p-1)*int64(s))
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		record, err := readRecord(rows, false)
		if err != nil {
			return result, err
		}
		result.Items = append(result.Items, record)
	}
	return result, rows.Err()
}

func (m *Module) Create(ctx context.Context, input CreateInput) (Record, error) {
	input.TemplateID = strings.TrimSpace(input.TemplateID)
	input.Name = strings.TrimSpace(input.Name)
	if !templateIDPattern.MatchString(input.TemplateID) {
		return Record{}, ErrInvalid
	}
	if err := validate(input.Name, input.Category, input.Content, false); err != nil {
		return Record{}, err
	}
	normalizePreviewValues(input.Content)
	raw, err := json.Marshal(input.Content)
	if err != nil {
		return Record{}, err
	}
	var result Record
	err = m.store.MutationTx(ctx, func(tx context.Context) error {
		var insertErr error
		result, insertErr = readRecord(m.store.SQL(tx).QueryRowContext(tx, "INSERT INTO t_mail_template(template_id,name,category,content) VALUES($1,$2,$3,$4) RETURNING "+recordColumns+",content", input.TemplateID, input.Name, input.Category, raw), true)
		return insertErr
	})
	var duplicate *pq.Error
	if errors.As(err, &duplicate) && duplicate.Code == "23505" {
		return Record{}, ErrDuplicate
	}
	return result, err
}

func (m *Module) Update(ctx context.Context, id int64, input UpdateInput) (Record, error) {
	hasFields := input.TemplateID != nil || input.Name != nil || input.Category != nil || input.Content != nil
	if id <= 0 || input.Revision <= 0 || (!hasFields && input.DiscardDraft == nil) {
		return Record{}, ErrInvalid
	}
	if input.DiscardDraft != nil && (!*input.DiscardDraft || hasFields) {
		return Record{}, ErrInvalid
	}
	if input.TemplateID != nil {
		value := strings.TrimSpace(*input.TemplateID)
		if !templateIDPattern.MatchString(value) {
			return Record{}, ErrInvalid
		}
		input.TemplateID = &value
	}
	var result Record
	err := m.store.MutationTx(ctx, func(tx context.Context) error {
		current, err := readRecord(m.store.SQL(tx).QueryRowContext(tx, "SELECT "+recordColumns+",content FROM t_mail_template WHERE id=$1 FOR UPDATE", id), true)
		if err != nil {
			return err
		}
		if current.Revision != input.Revision {
			return ErrConflict
		}
		if input.DiscardDraft != nil {
			if current.PublishedVersion == 0 {
				return ErrInvalid
			}
			published, err := m.GetVersion(tx, id, current.PublishedVersion)
			if err != nil {
				return err
			}
			raw, err := json.Marshal(published.Content)
			if err != nil {
				return err
			}
			// Keep the current template identity and immutable publication history.
			result, err = readRecord(m.store.SQL(tx).QueryRowContext(tx, "UPDATE t_mail_template SET name=$2,category=$3,content=$4,revision=revision+1,has_draft=FALSE,updated_at=FLOOR(EXTRACT(EPOCH FROM statement_timestamp())*1000)::bigint WHERE id=$1 RETURNING "+recordColumns+",content", id, published.Name, published.Category, raw), true)
			return err
		}
		if input.TemplateID != nil {
			current.TemplateID = *input.TemplateID
		}
		if input.Name != nil {
			current.Name = strings.TrimSpace(*input.Name)
		}
		if input.Category != nil {
			current.Category = *input.Category
		}
		if input.Content != nil {
			current.Content = *input.Content
		}
		if err := validate(current.Name, current.Category, current.Content, false); err != nil {
			return err
		}
		normalizePreviewValues(current.Content)
		raw, err := json.Marshal(current.Content)
		if err != nil {
			return err
		}
		result, err = readRecord(m.store.SQL(tx).QueryRowContext(tx, "UPDATE t_mail_template SET name=$2,category=$3,content=$4,template_id=$5,revision=revision+1,has_draft=TRUE,updated_at=FLOOR(EXTRACT(EPOCH FROM statement_timestamp())*1000)::bigint WHERE id=$1 RETURNING "+recordColumns+",content", id, current.Name, current.Category, raw, current.TemplateID), true)
		return err
	})
	var duplicate *pq.Error
	if errors.As(err, &duplicate) && duplicate.Code == "23505" {
		return Record{}, ErrDuplicate
	}
	return result, err
}

// Publish only creates an immutable content version. It never sends or queues mail.
func (m *Module) Publish(ctx context.Context, id int64, input PublishInput) (Record, error) {
	if id <= 0 || input.Revision <= 0 {
		return Record{}, ErrInvalid
	}
	identity, _ := authn.FromContext(ctx)
	var result Record
	err := m.store.MutationTx(ctx, func(tx context.Context) error {
		current, err := readRecord(m.store.SQL(tx).QueryRowContext(tx, "SELECT "+recordColumns+",content FROM t_mail_template WHERE id=$1 FOR UPDATE", id), true)
		if err != nil {
			return err
		}
		if current.Revision != input.Revision {
			return ErrConflict
		}
		if !current.HasDraft {
			return ErrConflict
		}
		if err := validate(current.Name, current.Category, current.Content, true); err != nil {
			return err
		}
		normalizePreviewValues(current.Content)
		raw, err := json.Marshal(current.Content)
		if err != nil {
			return err
		}
		if _, err = m.store.SQL(tx).ExecContext(tx, "INSERT INTO t_mail_template_version(template_pk,version,name,category,content,published_by) VALUES($1,$2,$3,$4,$5,$6)", id, current.PublishedVersion+1, current.Name, current.Category, raw, identity.UserID); err != nil {
			return err
		}
		result, err = readRecord(m.store.SQL(tx).QueryRowContext(tx, "UPDATE t_mail_template SET published_version=published_version+1,revision=revision+1,has_draft=FALSE,updated_at=FLOOR(EXTRACT(EPOCH FROM statement_timestamp())*1000)::bigint WHERE id=$1 RETURNING "+recordColumns+",content", id), true)
		return err
	})
	return result, err
}

func (m *Module) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrInvalid
	}
	return m.store.MutationTx(ctx, func(tx context.Context) error {
		result, err := m.store.SQL(tx).ExecContext(tx, "DELETE FROM t_mail_template WHERE id=$1", id)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err == nil && count == 0 {
			return ErrNotFound
		}
		return err
	})
}

func (m *Module) Versions(ctx context.Context, id int64) ([]Version, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	var exists bool
	if err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM t_mail_template WHERE id=$1)", id).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := m.store.SQL(ctx).QueryContext(ctx, "SELECT id,created_at,version,name,category,published_by FROM t_mail_template_version WHERE template_pk=$1 ORDER BY version DESC", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Version{}
	for rows.Next() {
		var v Version
		if err := rows.Scan(&v.ID, &v.CreatedAt, &v.Version, &v.Name, &v.Category, &v.PublishedBy); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (m *Module) GetVersion(ctx context.Context, id, version int64) (Version, error) {
	if id <= 0 || version <= 0 {
		return Version{}, ErrInvalid
	}
	var result Version
	var raw []byte
	err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT id,created_at,version,name,category,published_by,content FROM t_mail_template_version WHERE template_pk=$1 AND version=$2", id, version).Scan(&result.ID, &result.CreatedAt, &result.Version, &result.Name, &result.Category, &result.PublishedBy, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &result.Content)
		normalizePreviewValues(result.Content)
	}
	return result, err
}
