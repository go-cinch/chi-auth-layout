package main

import (
	"fmt"
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
	for _, name := range []string{"q", "module", "result", "time_zone", "from", "to", "weekday", "hour", "username", "user_id", "ip", "path", "sort"} {
		s := schema{typeName: "string"}
		if name == "from" || name == "to" || name == "user_id" {
			s = schema{typeName: "integer", format: "int64"}
		} else if name == "weekday" || name == "hour" {
			s = schema{typeName: "integer", format: "int32"}
		}
		description := ""
		if name == "from" || name == "to" {
			description = fmt.Sprintf("Optional Unix-millisecond bound: from is inclusive, to exclusive. A single bound constrains only that side. Omit both for the last %s. Explicit paired ranges and allocated charts may span at most %s. Missing chart bounds are derived from matching event extents and remain null in response range metadata.", g.audit.DefaultWindow, g.audit.MaxWindow)
		}
		if name == "user_id" {
			description = "Exact positive actor ID recorded on the event, retained across account renames and deletion."
		}
		if name == "q" {
			description = "Case-insensitive literal substring across username, ip, path and trace_id. Trimmed; blank keywords are ignored."
		}
		r.parameters = append(r.parameters, parameter{name: name, in: "query", description: description, schema: s})
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
		r.responses[200] = response{description: fmt.Sprintf("CSV audit metadata; at most %d records", g.audit.ExportMaxRows), contentType: "text/csv", schema: &schema{typeName: "string"}}
	}
}
