package redact

import (
	"encoding/json"
	"strings"
)

// Structured hides complete sensitive values, including nested transport secrets.
// Unlike display masking, audit storage must not preserve any secret prefix/suffix.
func (p Policy) Structured(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			if p.IsSensitive(key) {
				result[key] = "[redacted]"
			} else {
				result[key] = p.Structured(item)
			}
		}
		return result
	case []any:
		result := make([]any, len(value))
		for i, item := range value {
			result[i] = p.Structured(item)
		}
		return result
	case string:
		// JSON encoded inside a string still needs recursive secret filtering.
		decoder := json.NewDecoder(strings.NewReader(value))
		decoder.UseNumber()
		var nested any
		if decoder.Decode(&nested) == nil {
			switch nested.(type) {
			case map[string]any, []any:
				return p.Structured(nested)
			}
		}
	}
	return value
}
