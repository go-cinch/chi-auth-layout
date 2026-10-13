package main

import (
	"net/http"
	"strings"
)

// Audit handlers share parsing and error mapping; describe this explicit HTTP contract.
func (g *generator) describeAuditRoute(r *route) {
	if !strings.HasPrefix(r.path, "/audit/") {
		return
	}
	g.ensureErrorResponse()
	for _, status := range []int{400, 403, 500} {
		addErrorResponse(r, status, http.StatusText(status))
	}
	if strings.Contains(r.path, "/detail/") {
		addErrorResponse(r, 404, "audit event not found")
		return
	}
	for _, name := range []string{"q", "module", "result", "time_zone", "from", "to", "weekday", "hour", "username", "ip", "path", "sort"} {
		s := schema{typeName: "string"}
		if name == "from" || name == "to" {
			s = schema{typeName: "integer", format: "int64"}
		} else if name == "weekday" || name == "hour" {
			s = schema{typeName: "integer", format: "int32"}
		}
		r.parameters = append(r.parameters, parameter{name: name, in: "query", schema: s})
	}
	if strings.HasSuffix(r.path, "/login") || strings.HasSuffix(r.path, "/operation") {
		for _, name := range []string{"p", "s"} {
			limit := g.limits.MaxP
			if name == "s" {
				limit = g.limits.MaxS
			}
			fallback := int64(1)
			r.parameters = append(r.parameters, parameter{name: name, in: "query", schema: schema{typeName: "integer", format: "int32", paginationKey: name, paginationMax: int64(limit), defaultValue: &fallback}})
		}
	}
	if strings.HasSuffix(r.path, "/export") {
		r.responses[200] = response{description: "CSV audit metadata", contentType: "text/csv", schema: &schema{typeName: "string"}}
	}
}
