package mailtemplate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// RenderPublished reads one committed publication snapshot on every attempt.
// Drafts and editor preview values are never delivery inputs. Missing or
// unsupported account languages use Chinese.
func (m *Module) RenderPublished(ctx context.Context, id, language string, values map[string]string) (string, string, string, error) {
	if !templateIDPattern.MatchString(id) {
		return "", "", "", ErrInvalid
	}
	var raw []byte
	err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT v.content
        FROM t_mail_template t JOIN t_mail_template_version v
          ON v.template_pk=t.id AND v.version=t.published_version
        WHERE t.template_id=$1 AND t.published_version>0`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	if err != nil {
		return "", "", "", err
	}
	var content Content
	if err = json.Unmarshal(raw, &content); err != nil {
		return "", "", "", err
	}
	locale := "zh-CN"
	if language == "en-US" {
		locale = language
	}
	translation, exists := content[locale]
	if !exists {
		return "", "", "", ErrInvalid
	}
	subject, text, body, err := renderTranslation(translation, values)
	if err != nil {
		return "", "", "", err
	}
	if len(subject) > 500 {
		return "", "", "", ErrInvalid
	}
	return subject, text, `<div lang="`+locale+`">`+body+`</div>`, nil
}
