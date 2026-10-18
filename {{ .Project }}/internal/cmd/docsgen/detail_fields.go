package main

import (
	"net/http"
	"strings"
)

func (g *generator) describeDetailFields(r *route) {
	if r.method != http.MethodGet || r.path != "/action/{id}" {
		return
	}
	const description = "Optional comma-separated f: id,created_at,updated_at,code,name,names,group,word,scope,config,resource,menu,button. Only selected fields are returned, with no always-present metadata. Omitted f preserves the complete detail response. Empty, unknown, duplicate selectors or repeated f parameters return 400. SQL loads only selected columns and hidden dependencies: name needs word/config and localized labels; names needs word and registered-language overrides; config/resource/menu/button need word/config and the resource catalog. Unselected names and mappings are not computed. Selected empty names remain an object and empty resource/menu/button values remain strings."
	r.parameters = append(r.parameters, parameter{name: "f", in: "query", description: description, schema: schema{typeName: "string", example: "id,word,group,names,scope,config"}})
	response := r.responses[http.StatusOK]
	response.description = description
	record := response.schema
	if record != nil && record.ref != "" {
		record = g.components[strings.TrimPrefix(record.ref, "#/components/schemas/")]
	}
	if record != nil {
		// The GET projection is sparse; create/update keep the full Action schema.
		copyRecord := *record
		copyRecord.required = nil
		response.schema = &copyRecord
	}
	r.responses[http.StatusOK] = response
}
