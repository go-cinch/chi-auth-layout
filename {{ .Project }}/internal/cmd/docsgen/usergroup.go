package main

import "net/http"

func (g *generator) describeUserGroupRoute(r *route) {
	if (r.path != "/user-group" && r.path != "/user-group/{id}") || r.method == http.MethodDelete {
		return
	}
	for _, parameter := range r.parameters {
		if parameter.name == "f" {
			return
		}
	}
	r.parameters = append(r.parameters, parameter{name: "f", in: "query", schema: schema{typeName: "string", example: "actions,member_count,member_preview"}})
	response := r.responses[http.StatusOK]
	response.description = "Base fields always returned: id, created_at, updated_at, name, word, action_codes; lists also return items,t,p,s. f selects actions, users (all members), member_count, member_preview (at most five, ordered by user id). Omitted f preserves legacy actions and users. Unselected associations are omitted and not queried."
	r.responses[http.StatusOK] = response
}
