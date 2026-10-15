package main

import "strings"

func (g *generator) describeSummaryRoute(r *route) {
	if !strings.HasSuffix(r.path, "/summary") {
		return
	}
	example := "total,counts"
	switch r.path {
	case "/audit/login/summary":
		example = "metrics,trend,heatmap,accounts,sources"
	case "/audit/operation/summary":
		example = "metrics,trend,modules"
	case "/user/summary":
		example = "total,counts,trend"
	case "/action/summary", "/role/summary", "/user-group/summary":
		example = "total,counts,groups"
	}
	for i := range r.parameters {
		if r.parameters[i].name == "f" {
			r.parameters[i].schema.example = example
			return
		}
	}
	r.parameters = append(r.parameters, parameter{name: "f", in: "query", schema: schema{typeName: "string", example: example}})
}
