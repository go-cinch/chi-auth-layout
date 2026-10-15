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

// Projection contains only server-owned SQL fragments and matching scan targets.
// Never pass user input as an expression or a FROM clause.
type Projection struct {
	Fields      Fields
	expressions []string
	Targets     []any
}

func (p *Projection) Add(field, expression string, target any) {
	if p.Fields.Has(field) {
		p.expressions = append(p.expressions, expression)
		p.Targets = append(p.Targets, target)
	}
}

func (p *Projection) SQL(from string) string {
	query := "SELECT " + strings.Join(p.expressions, ", ")
	if from != "" {
		query += " FROM " + from
	}
	return query
}

// Replace changes a server-owned expression while preserving its scan target.
func (p *Projection) Replace(old, replacement string) {
	for i, expression := range p.expressions {
		if expression == old {
			p.expressions[i] = replacement
		}
	}
}

// Require loads a server-owned dependency without adding it to the response selection.
func (p *Projection) Require(expression string, target any) {
	p.expressions = append(p.expressions, expression)
	p.Targets = append(p.Targets, target)
}
