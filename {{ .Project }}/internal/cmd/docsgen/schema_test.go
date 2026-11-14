package main

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRawJSONResponseSchema(t *testing.T) {
	g := &generator{}
	value := g.typeSchema(nil, &ast.SelectorExpr{X: ast.NewIdent("json"), Sel: ast.NewIdent("RawMessage")})
	var output strings.Builder
	renderSchema(&output, value, 0)
	for _, part := range []string{"oneOf:", "type: object", "type: array", "nullable: true", "type: string", "type: number", "type: boolean"} {
		if !strings.Contains(output.String(), part) {
			t.Fatal(output.String())
		}
	}
}

func TestNullableTimestampSchema(t *testing.T) {
	g := &generator{}
	value := g.typeSchema(nil, &ast.StarExpr{X: ast.NewIdent("int64")})
	var output strings.Builder
	renderSchema(&output, value, 0)
	for _, part := range []string{"type: integer", "format: int64", "nullable: true"} {
		if !strings.Contains(output.String(), part) {
			t.Fatal(output.String())
		}
	}
}

func TestImportedResponseSchema(t *testing.T) {
	root := t.TempDir()
	writeSource(t, root, "conf/http.yml", "http:\n  docs:\n    servers: []\n")
	writeSource(t, root, "common/report/summary.go", `package report
 type ManagementPoint struct { Count int64 `+"`json:\"count\"`"+` }
 type ManagementSummary struct { Total int64 `+"`json:\"total\"`"+`; Trend []ManagementPoint `+"`json:\"trend\"`"+` }
 `)
	writeSource(t, root, "modules/example/http.go", `package example
 import (
  "net/http"
  chart "example.com/service/internal/common/report"
 )
 type Module struct{}
 func (*Module) Name() string {return "/example"}
 func (m *Module) Summary() (*chart.ManagementSummary,error) {return nil,nil}
 func (m *Module) HTTP() http.Handler {r:=chi.NewRouter();r.Get("/summary",m.summary);return r}
 func (m *Module) summary(w http.ResponseWriter,r *http.Request) {value,err:=m.Summary();if err!=nil{return};server.WriteOK(w,value)}
 `)
	output := filepath.Join(root, "docs/openapi.yaml")
	if err := run(root, filepath.Join(root, "conf"), output, "Summary"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`$ref: "#/components/schemas/ManagementSummary"`, `ManagementPoint:`, `total:`, `trend:`, `count:`} {
		if !strings.Contains(string(content), expected) {
			t.Fatalf("missing %s: %s", expected, content)
		}
	}
}

func TestPackageQualifiedSchemasAndAnyJSON(t *testing.T) {
	left := &sourcePackage{name: "action", directory: "internal/modules/action", types: map[string]ast.Expr{"Summary": &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{ {Names: []*ast.Ident{ast.NewIdent("Code")}, Type: ast.NewIdent("string"), Tag: &ast.BasicLit{Kind: token.STRING, Value: "`json:\"code\"`"}}}}}}}
	right := &sourcePackage{name: "role", directory: "internal/modules/role", types: map[string]ast.Expr{"Summary": &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{ {Names: []*ast.Ident{ast.NewIdent("ID")}, Type: ast.NewIdent("int64"), Tag: &ast.BasicLit{Kind: token.STRING, Value: "`json:\"id\"`"}}}}}}}
	g := &generator{components: map[string]*schema{}, visiting: map[string]bool{}}
	g.registerSchemaNames([]*sourcePackage{left, right}, "internal")
	a, b := g.typeSchema(left, ast.NewIdent("Summary")), g.typeSchema(right, ast.NewIdent("Summary"))
	if a.ref == b.ref || g.components["modules.action.Summary"].properties["code"] == nil || g.components["modules.role.Summary"].properties["id"] == nil {
		t.Fatal(a, b, g.components)
	}
	for _, expression := range []ast.Expr{ast.NewIdent("any"), &ast.InterfaceType{Methods: &ast.FieldList{}}} {
		if value := g.typeSchema(left, expression); !value.anyJSON {
			t.Fatal(value)
		}
	}
	named := g.structSchema(right, &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{ {Names: []*ast.Ident{ast.NewIdent("Role")}, Type: &ast.StarExpr{X: ast.NewIdent("Summary")}}}}})
	var output strings.Builder
	renderSchema(&output, named.properties["role"], 0)
	if !strings.Contains(output.String(), "anyOf:") || !strings.Contains(output.String(), "enum: [null]") || !strings.Contains(output.String(), b.ref) {
		t.Fatal(output.String())
	}
}

func TestEmbeddedLoginResultSchema(t *testing.T) {
	base := &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
		{Names: []*ast.Ident{ast.NewIdent("AccessToken")}, Type: ast.NewIdent("string"), Tag: &ast.BasicLit{Kind: token.STRING, Value: "`json:\"access_token\"`"}},
	}}}
	pkg := &sourcePackage{name: "auth", types: map[string]ast.Expr{"LoginResult": base}}
	g := &generator{components: map[string]*schema{}, visiting: map[string]bool{}}
	result := g.structSchema(pkg, &ast.StructType{Fields: &ast.FieldList{List: []*ast.Field{
		{Type: &ast.StarExpr{X: ast.NewIdent("LoginResult")}},
		{Names: []*ast.Ident{ast.NewIdent("RecoveryCodes")}, Type: &ast.ArrayType{Elt: ast.NewIdent("string")}, Tag: &ast.BasicLit{Kind: token.STRING, Value: "`json:\"recovery_codes\"`"}},
	}}})
	if result.properties["access_token"] == nil || result.properties["recovery_codes"] == nil {
		t.Fatal("embedded enrollment session fields missing")
	}
}

func TestPrimitiveSchema(t *testing.T) {
 if primitiveSchema("float64").format != "double" || primitiveSchema("unknown") != nil { t.Fatal("unexpected primitive schema") }
}
