package report

import (
	"net/url"
	"slices"
	"strings"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
)

// Fields is an allowlisted selection. Nil preserves the legacy full response.
type Fields []string

func (f Fields) Has(name string) bool { return f == nil || slices.Contains(f, name) }

func (f Fields) Validate(allowed ...string) error {
	if f != nil && len(f) == 0 {
		return apperror.InvalidQuery
	}
	seen := map[string]bool{}
	for _, name := range f {
		if !slices.Contains(allowed, name) || seen[name] {
			return apperror.InvalidQuery
		}
		seen[name] = true
	}
	return nil
}

// ParseFields rejects empty, duplicate and unknown selectors before database work.
func ParseFields(query url.Values, allowed ...string) (Fields, error) {
	values, ok := query["f"]
	if !ok {
		return nil, nil
	}
	if len(values) != 1 {
		return nil, apperror.InvalidQuery
	}
	fields := Fields(strings.Split(values[0], ","))
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return fields, fields.Validate(allowed...)
}
