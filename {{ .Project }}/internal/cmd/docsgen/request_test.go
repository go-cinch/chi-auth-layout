package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
		{"shared_decoder", "w http.ResponseWriter, r *http.Request", `var request CreateRequest; server.DecodeJSON(w,r,&request, 4096)`, true},
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

type requestSchemaDoc struct {
	Ref                  string `yaml:"$ref"`
	Type                 string
	Description          string
	Nullable             bool
	AdditionalProperties any                `yaml:"additionalProperties"`
	AnyOf                []requestSchemaDoc `yaml:"anyOf"`
	Required             []string
	Properties           map[string]requestSchemaDoc
}

func readRequestSchema(t *testing.T, output, path string) (requestSchemaDoc, map[string]requestSchemaDoc) {
	t.Helper()
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]struct{ Schema requestSchemaDoc }
			} `yaml:"requestBody"`
		}
		Components struct{ Schemas map[string]requestSchemaDoc }
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Paths[path]["post"].RequestBody.Content["application/json"].Schema, doc.Components.Schemas
}

func TestRequestBodyAlternatives(t *testing.T) {
	for _, tc := range []struct {
		name, signature, body string
		alternatives          int
		nullable              bool
	}{
		{"chi", "w http.ResponseWriter, r *http.Request", `
if r.Header.Get("X-Continuation") != "" {
    var continuation *struct{}
    if server.DecodeJSON(w, r, &continuation, 4096) != nil { return }
    if continuation == nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
    return
}
var input CreateRequest
decoder := json.NewDecoder(r.Body)
decoder.Decode(&input)
decoder.Decode(new(any))`, 2, false},
		{"gin", "c *gin.Context", `
if c.Request.Header.Get("X-Continuation") != "" {
    var continuation *struct{}
    if server.DecodeJSON(c.Writer, c.Request, &continuation, 4096) != nil { return }
    if nil == continuation { return }
    return
}
var input CreateRequest
c.ShouldBindJSON(&input)`, 2, false},
		{"duplicate", "w http.ResponseWriter, r *http.Request", `
if r.Header.Get("X-Mode") != "" {
    var first CreateRequest
    server.DecodeJSON(w, r, &first, 4096)
    return
}
var second CreateRequest
json.NewDecoder(r.Body).Decode(&second)`, 0, false},
		{"nullable", "w http.ResponseWriter, r *http.Request", `
var input *struct{}
server.DecodeJSON(w, r, &input, 4096)`, 0, true},
		{"nil_check_without_return", "w http.ResponseWriter, r *http.Request", `
var input *struct{}
server.DecodeJSON(w, r, &input, 4096)
if input == nil { log.Print("null input") }`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeSource(t, root, "conf/http.yml", "http:\n  docs:\n    servers: []\n")
			writeSource(t, root, "order/http.go", `package order
type Module struct{}
type CreateRequest struct { Amount int64 `+"`json:\"amount\"`"+` }
func (*Module) Name() string { return "/order" }
func (m *Module) HTTP() http.Handler { r := chi.NewRouter(); r.Post("/", m.create); return r }
func (m *Module) create(`+tc.signature+`) { `+tc.body+`; server.WriteOK(w) }
`)
			output := filepath.Join(root, "openapi.yaml")
			if err := run(root, filepath.Join(root, "conf"), output, "alternatives"); err != nil {
				t.Fatal(err)
			}
			body, _ := readRequestSchema(t, output, "/order")
			if len(body.AnyOf) != tc.alternatives || body.Nullable != tc.nullable {
				t.Fatalf("wrong request alternatives/nullability: %+v", body)
			}
			if tc.alternatives == 2 {
				empty, typed := body.AnyOf[0], body.AnyOf[1]
				if empty.Type != "object" || empty.Nullable || empty.AdditionalProperties != false || len(empty.Properties) != 0 || typed.Ref != "#/components/schemas/CreateRequest" {
					t.Fatalf("empty or typed branch was widened/lost: %+v", body)
				}
			}
			if tc.name == "duplicate" && body.Ref != "#/components/schemas/CreateRequest" {
				t.Fatal("duplicate decoding lost the original schema", body)
			}
		})
	}
}
