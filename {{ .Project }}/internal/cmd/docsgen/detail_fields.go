package main

import (
	"net/http"
	"strings"
)

func (g *generator) describeDetailFields(r *route) {
	if r.method != http.MethodGet {
		return
	}
	if r.path != "/action/{id}" {
		fields := ""
		switch {
		case r.path == "/msg/sent/{id}":
			fields = "id,created_at,updated_at,title,content,summary,type,scope,sender_id,published_at,expired_at,read_at,recipient_ids,recipients"
		case r.path == "/user/{id}":
			fields = "id,created_at,updated_at,username,code,role_id,last_logged_in_at,login_count,status,metadata,wrong,action_codes,actions,role,role_summary,permission_sources"
		case r.path == "/role/{id}":
			fields = "id,created_at,updated_at,name,word,action_codes,actions,action_summaries"
		case strings.HasPrefix(r.path, "/audit/") && strings.Contains(r.path, "/detail/"):
			fields = "id,occurred_at,kind,user_id,username,module,method,path,ip,user_agent,trace_id,error_code,status,duration_ms,operation,resource_type,resource_id,header,request_params,response_params"
		case r.pkg != nil && r.pkg.name == "auth" && r.handler != nil && r.handler.Name.Name == "info":
			fields = "id,username,code,role,scope,permission"
		}
		if fields != "" {
			g.describeSelectedDetail(r, fields)
		}
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

func (g *generator) describeSelectedDetail(r *route, fields string) {
	description := "Optional comma-separated f: " + fields + ". Omitted f preserves the complete detail response. Only selected fields and their hidden dependencies are loaded; unselected associations and payloads are not queried. No metadata is always present. Selected zero values, empty arrays and null associations are retained. Empty, unknown, duplicate selectors and repeated f parameters return 400."
	if r.path == "/user/{id}" {
		description += " permission_sources is opt-in and adds one statement snapshot of direct, role and user-group assignments, including owner kind/id/name and action_codes/actions (code,word,config without technical ui/endpoints projections). Empty owners are retained, overlapping codes keep all owners, and no user-list or write contract changes."
	}
	r.parameters = append(r.parameters, parameter{name: "f", in: "query", description: description, schema: schema{typeName: "string", example: "id"}})
	response := r.responses[http.StatusOK]
	response.description = description
	record := response.schema
	if record != nil && record.ref != "" {
		record = g.components[strings.TrimPrefix(record.ref, "#/components/schemas/")]
	}
	if record != nil {
		copyRecord := *record
		copyRecord.required = nil
		response.schema = &copyRecord
	}
	r.responses[http.StatusOK] = response
}
