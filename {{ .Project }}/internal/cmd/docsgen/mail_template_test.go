package main

import (
 "net/http"
 "testing"
 "{{ .Computed.module_name_final }}/internal/common/pagination"
)

func TestMailTemplateDocumentation(t *testing.T) {
 g:=&generator{limits:pagination.Limits{MaxP:20,MaxS:50},components:map[string]*schema{}}
 r:=route{path:"/mail-template",method:http.MethodGet,responses:map[int]response{200:{}}}
 g.describeMailTemplateRoute(&r)
 if len(r.parameters)!=4||r.parameters[0].schema.paginationMax!=20||r.parameters[1].schema.paginationMax!=50{t.Fatal(r.parameters)}
 r.method=http.MethodPatch;r.requestBody=&schema{};g.describeMailTemplateRoute(&r)
 if r.responses[409].schema==nil||r.requestBody.description==""{t.Fatal("missing mutation contract")}
 other:=route{path:"/other"};g.describeMailTemplateRoute(&other);if len(other.parameters)!=0{t.Fatal(other)}
}
