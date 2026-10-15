package main

import "testing"

func TestSummaryFieldParameter(t *testing.T) {
	g := &generator{}
	for _, path := range []string{"/audit/login/summary", "/audit/operation/summary", "/user/summary", "/action/summary", "/role/summary", "/user-group/summary", "/msg/sent/summary"} {
		r := route{path: path}
		g.describeSummaryRoute(&r)
		g.describeSummaryRoute(&r)
		if len(r.parameters) != 1 || r.parameters[0].name != "f" || r.parameters[0].required || r.parameters[0].schema.example == "" {
			t.Fatal(r.parameters)
		}
	}
	r := route{path: "/user"}
	g.describeSummaryRoute(&r)
	if len(r.parameters) != 0 {
		t.Fatal(r.parameters)
	}
}
