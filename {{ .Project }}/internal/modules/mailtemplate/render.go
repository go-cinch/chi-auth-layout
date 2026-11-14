package mailtemplate

import (
	"bytes"
	"html/template"
	"strings"
)

func renderTranslation(value Translation, values map[string]string) (string, string, string, error) {
	// Reuse the editor's size and placeholder rules, without requiring the other language.
	value.PreviewValues = values
	if err := validate("test", "notification", Content{"zh-CN": value, "en-US": value}, true); err != nil {
		return "", "", "", err
	}
	for _, name := range Parameters(value.Subject + "\n" + value.HTML + "\n" + value.Text) {
		if strings.TrimSpace(values[name]) == "" {
			return "", "", "", ErrInvalid
		}
	}
	// Apply the same account privacy rule to Auth delivery and editor test sends.
	// Keep caller-owned input and signed queue provenance unchanged.
	if account, ok := values["account"]; ok {
		masked := make(map[string]string, len(values))
		for key, value := range values {
			masked[key] = value
		}
		characters := []rune(account)
		masked["account"] = string(characters[:min(2, len(characters))]) + "***"
		values = masked
	}
	replace := func(source string) string {
		return parameterPattern.ReplaceAllStringFunc(source, func(token string) string {
			return values[parameterPattern.FindStringSubmatch(token)[1]]
		})
	}
	subject := replace(value.Subject)
	if len(subject) > 500 || strings.ContainsAny(subject, "\r\n") {
		return "", "", "", ErrInvalid
	}
	// Contextual escaping protects text, attributes and URL parameters. Values are
	// inserted once and can never become template expressions or raw HTML.
	source := parameterPattern.ReplaceAllStringFunc(value.HTML, func(token string) string {
		return "{" + "{." + parameterPattern.FindStringSubmatch(token)[1] + "}" + "}"
	})
	parsed, err := template.New("mail").Option("missingkey=error").Parse(source)
	if err != nil {
		return "", "", "", ErrInvalid
	}
	var html bytes.Buffer
	if err = parsed.Execute(&html, values); err != nil {
		return "", "", "", ErrInvalid
	}
	return subject, replace(value.Text), html.String(), nil
}
