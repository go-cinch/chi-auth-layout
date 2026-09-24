package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONRequestBodies(t *testing.T) {
	for _, tc := range []struct {
		name, signature, body string
		want                  bool
	}{
		{"decoder", "w http.ResponseWriter, r *http.Request", `var request CreateRequest; decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)); decoder.Decode(&request); decoder.Decode(new(any))`, true},
		{"inline", "w http.ResponseWriter, r *http.Request", `var request CreateRequest; json.NewDecoder(r.Body).Decode(&request)`, true},
		{"gin", "c *gin.Context", `var request CreateRequest; c.ShouldBindJSON(&request)`, true},
		{"gin_bind", "c *gin.Context", `var request CreateRequest; c.BindJSON(&request)`, true},
		{"none", "w http.ResponseWriter, r *http.Request", ``, false},
		{"xml", "w http.ResponseWriter, r *http.Request", `var request CreateRequest; decoder := xml.NewDecoder(r.Body); decoder.Decode(&request)`, false},
		{"other_reader", "w http.ResponseWriter, r *http.Request", `var request CreateRequest; decoder := json.NewDecoder(strings.NewReader("{}")); decoder.Decode(&request)`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeSource(t, root, "conf/http.yml", "http:\n  docs:\n    servers: []\n")
			source := `package order
 type Module struct{}
 type CreateRequest struct { Amount int64 ` + "`json:\"amount\" example:\"1990\"`" + ` }
 func (*Module) Name() string { return "/order" }
 func (m *Module) HTTP() http.Handler {r:=chi.NewRouter();r.Post("/",m.create);return r}
 func (m *Module) create(` + tc.signature + `) {` + tc.body + `;server.WriteOK(w)}
 `
			writeSource(t, root, "order/http.go", source)
			output := filepath.Join(root, "openapi.yaml")
			if err := run(root, filepath.Join(root, "conf"), output, "test"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			doc := string(data)
			if strings.Contains(doc, "requestBody:") != tc.want {
				t.Fatal(doc)
			}
			if tc.want {
				for _, expected := range []string{"requestBody:\n        required: true", "#/components/schemas/CreateRequest", "required: [amount]", "example: 1990"} {
					if !strings.Contains(doc, expected) {
						t.Fatalf("missing %s:\n%s", expected, doc)
					}
				}
				if strings.Contains(doc, `example: "1990"`) {
					t.Fatal("numeric example became string")
				}
			}
		})
	}
}

func TestSchemaExamples(t *testing.T) {
	for _, tc := range []struct{ typ, input, want string }{
		{"integer", "1990", "1990"}, {"number", "1.5", "1.5"}, {"boolean", "true", "true"},
		{"string", "hello", `"hello"`}, {"integer", "invalid", `"invalid"`},
	} {
		if got := schemaExample(&schema{typeName: tc.typ, example: tc.input}); got != tc.want {
			t.Fatalf("example: %s", got)
		}
	}
}
