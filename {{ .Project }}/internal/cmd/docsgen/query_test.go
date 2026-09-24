package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
)

func TestQueryParameters(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "handler.go", `package example
func list(w http.ResponseWriter,r *http.Request){
 name:=r.URL.Query().Get("name")
 if raw:=r.URL.Query().Get("page");raw!="" {page,err:=strconv.ParseInt(raw,10,32)}
 if raw:=r.URL.Query().Get("pageSize");raw!="" {pageSize,err:=strconv.ParseInt(raw,10,32)}
 if active,err:=strconv.ParseBool(r.URL.Query().Get("active"));err!=nil {}
 other.Get("notQuery")
}
func ginList(c *gin.Context){
 name:=c.Query("name")
 page,err:=strconv.Atoi(c.DefaultQuery("page","1"))
}
func remove(w http.ResponseWriter,r *http.Request){
 parts:=strings.Split(chi.URLParam(r,"id"),",")
 for _,part:=range parts {strconv.ParseInt(part,10,64)}
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	function := file.Decls[0].(*ast.FuncDecl)
	parameters := queryParameters(function)
	if len(parameters) != 4 {
		t.Fatalf("parameters: %v", parameters)
	}
	expected := map[string]string{"name": "string", "page": "integer", "pageSize": "integer", "active": "boolean"}
	for _, p := range parameters {
		if p.in != "query" || p.required || p.schema.typeName != expected[p.name] {
			t.Fatalf("parameter: %#v", p)
		}
		if strings.HasPrefix(p.name, "page") && p.schema.format != "int32" {
			t.Fatalf("format: %#v", p)
		}
	}
	parameters = queryParameters(file.Decls[1].(*ast.FuncDecl))
	if len(parameters) != 2 || parameters[1].schema.typeName != "integer" {
		t.Fatalf("gin parameters: %v", parameters)
	}
	value := parameterType(file.Decls[2].(*ast.FuncDecl), "id")
	if value.typeName != "string" || value.example != "1,2,3" {
		t.Fatalf("CSV parameter: %#v", value)
	}
	doc := string(render("test", nil, []route{
		{method: "GET", path: "/example", parameters: parameters},
	}, nil))
	if !strings.Contains(doc, "in: query\n          required: false") {
		t.Fatal(doc)
	}
}

func TestQueryMapDocumentation(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "handler.go", `package example
func list(w http.ResponseWriter,r *http.Request){
 query:=r.URL.Query()
 name:=query.Get("name")
 if values,provided:=query["p"];provided {p,err:=strconv.ParseInt(values[0],10,32)}
 if values,provided:=query["s"];provided {s,err:=strconv.ParseInt(values[0],10,32)}
 headers:=r.Header
 headers.Get("not-query")
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	parameters := queryParameters(file.Decls[0].(*ast.FuncDecl))
	if len(parameters) != 3 {
		t.Fatal(parameters)
	}
	p, s := parameters[1], parameters[2]
	if p.name != "p" || p.schema.paginationMax != 10000 || *p.schema.defaultValue != 1 {
		t.Fatalf("page: %#v", p)
	}
	if s.name != "s" || s.schema.paginationMax != 10000 || *s.schema.defaultValue != 1 {
		t.Fatalf("size: %#v", s)
	}
	doc := string(render("test", nil, []route{
		{method: "GET", path: "/example", parameters: parameters},
	}, nil))
	if strings.Contains(doc, "minimum:") || strings.Contains(doc, "maximum:") {
		t.Fatal("pagination must describe empty windows, not reject their values")
	}
	for _, expected := range []string{"Values outside 1..10000 return an empty list.", "default: 1"} {
		if !strings.Contains(doc, expected) {
			t.Fatal(doc)
		}
	}
}

func TestConfiguredQueryLimits(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "handler.go", `package example
func list(c *gin.Context){
 if value,provided:=c.GetQuery("p");provided {p,err:=strconv.ParseInt(value,10,32)}
 if value,provided:=c.GetQuery("s");provided {s,err:=strconv.ParseInt(value,10,32)}
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	limits, _ := pagination.New(4, 7)
	parameters := queryParameters(file.Decls[0].(*ast.FuncDecl), limits)
	if len(parameters) != 2 || parameters[0].schema.paginationMax != 4 || parameters[1].schema.paginationMax != 7 || *parameters[1].schema.defaultValue != 1 {
		t.Fatal(parameters)
	}
	if parameters[0].schema.paginationKey != "p" || parameters[1].schema.paginationKey != "s" {
		t.Fatal("missing runtime markers")
	}
}
