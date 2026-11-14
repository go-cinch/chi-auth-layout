package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestErrorDescriptionsAreUnique(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "handler.go", `package example
func handler(w http.ResponseWriter,r *http.Request){
 server.WriteError(w,400,"invalid query")
 server.WriteError(w,400,"invalid p")
 server.WriteError(w,400,"invalid p")
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	g := &generator{components: map[string]*schema{}, visiting: map[string]bool{}}
	value := route{handler: file.Decls[0].(*ast.FuncDecl), pkg: &sourcePackage{}}
	g.describeRoute(&value)
	if got := value.responses[400].description; got != "invalid query; invalid p" {
		t.Fatal(got)
	}
}

func TestStatusCode(t *testing.T) {
	expression, err := parser.ParseExpr("http.StatusServiceUnavailable")
	if err != nil || statusCode(expression) != 503 {
		t.Fatalf("status = %d, %v", statusCode(expression), err)
	}
}
