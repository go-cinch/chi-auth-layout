package main

import (
	"net/http"
	"strings"
)

func (g *generator) describeListFields(r *route) {
	if r.method != http.MethodGet {
		return
	}
	fields := map[string]string{
		"/user":            "id,created_at,updated_at,username,code,role_id,last_logged_in_at,login_count,status,metadata,wrong,action_codes,actions,role,role_summary",
		"/role":            "id,created_at,updated_at,name,word,action_codes,actions,action_summaries",
		"/action":          "id,created_at,updated_at,code,name,group,word,resource,menu,button",
		"/dictionary":      "id,created_at,updated_at,key,name,description,enabled,value",
		"/whitelist":       "id,created_at,updated_at,category,resource",
		"/user-group":      "id,created_at,updated_at,name,word,action_codes,action_summaries",
		"/msg/inbox":       "id,created_at,updated_at,title,content,summary,type,scope,sender_id,published_at,expired_at,read_at",
		"/msg/sent":        "id,created_at,updated_at,title,content,summary,type,scope,sender_id,published_at,expired_at,read_at",
		"/audit/login":     "id,occurred_at,kind,user_id,username,module,method,path,ip,user_agent,trace_id,error_code,status,duration_ms,operation,resource_type,resource_id",
		"/audit/operation": "id,occurred_at,kind,user_id,username,module,method,path,ip,user_agent,trace_id,error_code,status,duration_ms,operation,resource_type,resource_id",
	}[r.path]
	if fields == "" {
		return
	}
	found := false
	for i := range r.parameters {
		if r.parameters[i].name == "f" {
			found = true
			r.parameters[i].schema.example = strings.Split(fields, ",")[0]
		}
	}
	if !found {
		r.parameters = append(r.parameters, parameter{name: "f", in: "query", schema: schema{typeName: "string", example: strings.Split(fields, ",")[0]}})
	}
	response := r.responses[http.StatusOK]
	response.description = "Optional comma-separated f: " + fields + ". Only selected record fields are loaded and returned; items,t,p,s always remain. Omitted f preserves the legacy response. Empty, unknown, duplicate selectors or repeated f parameters return 400."
	if r.path == "/role" || r.path == "/user-group" {
		response.description += " action_summaries returns ordered code,name,group identities without loading resource/menu/button rules; actions retains complete associations."
	}
	if r.path == "/user" {
		response.description += " role_summary returns only id,name,word (or null), with one batched role query and no permission lookup. role retains the complete role association. Status and metadata use the same request timestamp as the filters to reflect expired temporary locks without writes."
	}
	if r.path == "/user-group" {
		response.description += " The associations actions,users,member_count,member_preview may also be selected. Selections containing only legacy associations (actions,users,member_count,member_preview) preserve existing base metadata; selections containing any listed field, including action_summaries, return exactly the selected fields."
	}
	if strings.HasPrefix(r.path, "/msg/") {
		response.description += " summary is the first 160 Unicode characters of content; it is not included by default. Detail endpoints still return full content."
	}
	// List item schemas must allow sparse records without relaxing detail/write schemas.
	resolve := func(s *schema) *schema {
		if s != nil && s.ref != "" {
			return g.components[strings.TrimPrefix(s.ref, "#/components/schemas/")]
		}
		return s
	}
	if page := resolve(response.schema); page != nil {
		copyPage := *page
		copyPage.properties = map[string]*schema{}
		for key, value := range page.properties {
			copyPage.properties[key] = value
		}
		if items := page.properties["items"]; items != nil {
			copyItems := *items
			if record := resolve(items.items); record != nil {
				copyRecord := *record
				copyRecord.required = nil
				copyItems.items = &copyRecord
			}
			copyPage.properties["items"] = &copyItems
		}
		response.schema = &copyPage
	}
	r.responses[http.StatusOK] = response
}
