package main

import "net/http"

func (g *generator) describeCatalogRoute(r *route) {
	if r.method != http.MethodGet {
		return
	}
	var description, example string
	switch r.path {
	case "/dictionary/pub/i18n":
		example = "default_locale,locales,group_order,frontend_messages"
		description = "Select default_locale,locales,scopes,operations,group_order,messages,frontend_messages. Omitted f returns default_locale and locales. schema_version is always present. frontend_messages returns only messages.<locale>.frontend, including projected menu labels; messages returns all namespaces. These two selectors are mutually exclusive. Only selected fields are present; selected empty collections remain arrays or objects."
		if component := g.components["PublicCatalog"]; component != nil {
			component.required = []string{"schema_version"}
		}
	case "/action/catalog":
		example = "scopes,operations,resource_catalog,defined_actions"
		description = "Select scopes,operations,resource_catalog,defined_actions. Omitted f returns scopes and operations. Only selected fields are present; empty selected collections remain arrays."
	case "/action/effective":
		example = "btn,http,grpc"
		description = "Select btn,http,grpc. Omitted f returns btn. scope is always present; selected empty collections are arrays."
		for i := range r.parameters {
			if r.parameters[i].in == "query" && r.parameters[i].name == "scope" {
				r.parameters[i].required = true
			}
		}
	default:
		return
	}
	for _, name := range []string{"CatalogResult", "EffectiveResult"} {
		if component := g.components[name]; component != nil {
			component.required = nil
			if name == "EffectiveResult" {
				component.required = []string{"scope"}
			}
		}
	}
	r.parameters = append(r.parameters, parameter{name: "f", in: "query", schema: schema{typeName: "string", example: example}, description: description})
}
