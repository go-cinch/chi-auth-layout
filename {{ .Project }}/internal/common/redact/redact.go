package redact

import (
	"net/url"
	"strings"
	"unicode"
)

var defaultSensitiveKeys = []string{
	"password",
	"passwd",
	"pwd",
	"secret",
	"token",
	"authorization",
	"credential",
	"cookie",
	"session",
	"dsn",
	"key",
	"apiKey",
	"accessKey",
	"privateKey",
	"clientSecret",
	"email",
	"phone",
	"mobile",
	"card",
}

type Policy struct {
	keywords map[string]struct{}
	names    map[string]struct{}
}

func New(sensitiveKeys ...string) Policy {
	keys := make([]string, 0, len(defaultSensitiveKeys)+len(sensitiveKeys))
	keys = append(keys, defaultSensitiveKeys...)
	keys = append(keys, sensitiveKeys...)
	policy := Policy{
		keywords: make(map[string]struct{}, len(keys)),
		names:    make(map[string]struct{}, len(keys)),
	}
	for _, key := range keys {
		parts := splitName(key)
		switch len(parts) {
		case 0:
			continue
		case 1:
			policy.keywords[parts[0]] = struct{}{}
		default:
			policy.names[strings.Join(parts, ".")] = struct{}{}
		}
	}
	return policy
}

func (p Policy) IsSensitive(name string) bool {
	parts := splitName(name)
	if len(parts) == 0 {
		return false
	}
	if _, exists := p.names[strings.Join(parts, ".")]; exists {
		return true
	}
	for _, part := range parts {
		if _, exists := p.keywords[part]; exists {
			return true
		}
	}
	return false
}

func (p Policy) Value(name, value string) string {
	if !p.IsSensitive(name) {
		return value
	}
	for _, part := range splitName(name) {
		if part == "dsn" {
			return DSN(value)
		}
	}
	return Mask(value)
}

func Mask(value string) string {
	runes := []rune(value)
	if len(runes) <= 6 {
		if len(runes) > 3 {
			runes = runes[:3]
		}
		return string(runes) + "***"
	}
	return string(runes[:3]) + "***" + string(runes[len(runes)-3:])
}

func DSN(value string) string {
	if parsed, err := url.Parse(value); err == nil && parsed.User != nil {
		if password, exists := parsed.User.Password(); exists {
			parsed.User = url.UserPassword(parsed.User.Username(), Mask(password))
			masked := parsed.String()
			masked = strings.ReplaceAll(masked, "%2A", "*")
			return strings.ReplaceAll(masked, "%2a", "*")
		}
		return value
	}

	separator := strings.LastIndex(value, "@")
	if separator < 0 {
		return value
	}
	credentials := value[:separator]
	password := strings.Index(credentials, ":")
	if password < 0 {
		return value
	}
	return credentials[:password+1] + Mask(credentials[password+1:]) + value[separator:]
}

func splitName(value string) []string {
	runes := []rune(strings.TrimSpace(value))
	parts := make([]string, 0, 4)
	current := make([]rune, 0, len(runes))
	flush := func() {
		if len(current) == 0 {
			return
		}
		parts = append(parts, string(current))
		current = current[:0]
	}
	for index, value := range runes {
		if !unicode.IsLetter(value) && !unicode.IsDigit(value) {
			flush()
			continue
		}
		if unicode.IsUpper(value) && len(current) > 0 {
			previous := runes[index-1]
			nextIsLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
			if unicode.IsLower(previous) || unicode.IsDigit(previous) || unicode.IsUpper(previous) && nextIsLower {
				flush()
			}
		}
		current = append(current, unicode.ToLower(value))
	}
	flush()
	return parts
}
