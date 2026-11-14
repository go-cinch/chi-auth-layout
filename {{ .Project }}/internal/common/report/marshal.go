package report

import (
	"encoding/json"
)

// MarshalFields preserves explicitly selected zero values and nulls.
// The caller passes an alias to avoid recursively invoking MarshalJSON.
func MarshalFields(value any, fields Fields) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil || fields == nil {
		return data, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	selected := make(map[string]json.RawMessage, len(fields))
	for _, field := range fields {
		selected[field] = all[field]
	}
	return json.Marshal(selected)
}
