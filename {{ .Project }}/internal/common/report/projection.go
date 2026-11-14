package report

import (
	"strings"
)

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
