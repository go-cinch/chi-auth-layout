package main

import (
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
)

func TestAuditContract(t *testing.T) {
	g := &generator{limits: pagination.Limits{MaxP: 20, MaxS: 50}, components: make(map[string]*schema)}
	for _, path := range []string{"/other", "/audit/login", "/audit/operation", "/audit/login/detail/{id}", "/audit/login/export", "/audit/operation/summary"} {
		r := route{path: path, responses: map[int]response{}}
		g.describeAuditRoute(&r)
		if path == "/other" {
			if len(r.parameters) != 0 {
				t.Fatal(r)
			}
			continue
		}
		if _, ok := r.responses[403]; !ok {
			t.Fatal(r)
		}
		if path == "/audit/login/export" && r.responses[200].contentType != "text/csv" {
			t.Fatal(r)
		}
		if path == "/audit/login" && len(r.parameters) != 14 {
			t.Fatal(r.parameters)
		}
	}
}
