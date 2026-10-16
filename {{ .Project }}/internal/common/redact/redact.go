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
	"authentication",
	"jwt",
	"csrf",
	"xsrf",
	"signature",
	"baggage",
	"credential",
	"challenge",
	"jwk",
	"proof",
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
	if len(runes) <= 3 {
		return "***"
	}
	if len(runes) <= 6 {
		return string(runes[:3]) + "***"
	}
	return string(runes[:3]) + "***" + string(runes[len(runes)-3:])
}

func DSN(value string) string {
	policy := New("sslpassword")
	if hasDSNOptions(value) {
		return maskDSNOptions(value, policy)
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return "***"
		}
		if parsed.User != nil {
			if password, exists := parsed.User.Password(); exists {
				parsed.User = url.UserPassword(parsed.User.Username(), Mask(password))
			}
		}
		query, err := url.ParseQuery(parsed.RawQuery)
		if err != nil {
			// A partial query must never expose a malformed credential value.
			parsed.RawQuery = "***"
		} else {
			changed := false
			for key, values := range query {
				if !policy.IsSensitive(key) {
					continue
				}
				for index, value := range values {
					values[index] = Mask(value)
				}
				changed = true
			}
			if changed {
				parsed.RawQuery = query.Encode()
			}
		}
		masked := strings.ReplaceAll(parsed.String(), "%2A", "*")
		return strings.ReplaceAll(masked, "%2a", "*")
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

// Detect keyword DSNs from their first assignment, never from credential text.
func hasDSNOptions(value string) bool {
	key, _, found := strings.Cut(strings.TrimSpace(value), "=")
	key = strings.TrimSpace(key)
	if !found || key == "" {
		return false
	}
	for index, char := range key {
		if char != '_' && !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(index > 0 && char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}

// maskDSNOptions follows PostgreSQL's quoted and backslash-escaped option syntax.
// Preserve connection context and formatting, but fail closed on malformed input.
func maskDSNOptions(value string, policy Policy) string {
	runes := []rune(value)
	var output strings.Builder
	last := 0
	for index := 0; index < len(runes); {
		for index < len(runes) && unicode.IsSpace(runes[index]) {
			index++
		}
		if index == len(runes) {
			break
		}
		keyStart := index
		for index < len(runes) && !unicode.IsSpace(runes[index]) && runes[index] != '=' {
			index++
		}
		key := string(runes[keyStart:index])
		for index < len(runes) && unicode.IsSpace(runes[index]) {
			index++
		}
		if key == "" || index == len(runes) || runes[index] != '=' {
			return "***"
		}
		index++
		for index < len(runes) && unicode.IsSpace(runes[index]) {
			index++
		}
		start := index
		quoted := index < len(runes) && runes[index] == '\''
		if quoted {
			index++
		}
		var decoded []rune
		closed := !quoted
		for index < len(runes) {
			current := runes[index]
			if quoted && current == '\'' {
				index++
				closed = true
				break
			}
			if !quoted && unicode.IsSpace(current) {
				break
			}
			if current == '\\' {
				index++
				if index == len(runes) {
					return "***"
				}
				current = runes[index]
			}
			decoded = append(decoded, current)
			index++
		}
		if !closed {
			return "***"
		}
		if policy.IsSensitive(key) {
			output.WriteString(string(runes[last:start]))
			masked := Mask(string(decoded))
			masked = strings.ReplaceAll(strings.ReplaceAll(masked, `\`, `\\`), `'`, `\'`)
			output.WriteString("'" + masked + "'")
			last = index
		}
	}
	output.WriteString(string(runes[last:]))
	return output.String()
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
