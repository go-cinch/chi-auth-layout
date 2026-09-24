package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/pmezard/go-difflib/difflib"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
)

type sourcePackage struct {
	name      string
	funcs     map[string]*ast.FuncDecl
	types     map[string]ast.Expr
	errors    map[string]string
	receivers map[string]string
}

type route struct {
	method        string
	path          string
	tag           string
	authenticated bool
	idempotent    bool
	handler       *ast.FuncDecl
	pkg           *sourcePackage
	responses     map[int]response
	parameters    []parameter
	requestBody   *schema
}

type response struct {
	description string
	schema      *schema
}

type parameter struct {
	in       string
	required bool
	name     string
	schema   schema
}

type docServer struct {
	URL         string `koanf:"url"`
	Description string `koanf:"description"`
}

type docsFileConfig struct {
	Pagination struct {
		MaxP int `koanf:"maxP"`
		MaxS int `koanf:"maxS"`
	} `koanf:"pagination"`
	HTTP docsHTTPConfig `koanf:"http"`
}

type docsHTTPConfig struct {
	Docs docsConfig `koanf:"docs"`
}

type docsConfig struct {
	Servers []docServer `koanf:"servers"`
}

type schema struct {
	paginationKey        string
	paginationMax        int64
	defaultValue         *int64
	ref                  string
	typeName             string
	format               string
	properties           map[string]*schema
	required             []string
	items                *schema
	additionalProperties *schema
	example              string
}

type generator struct {
	limits     pagination.Limits
	components map[string]*schema
	visiting   map[string]bool
}

var routeMethods = map[string]string{
	"Get": http.MethodGet, "Post": http.MethodPost, "Put": http.MethodPut,
	"Patch": http.MethodPatch, "Delete": http.MethodDelete, "Head": http.MethodHead,
	"Options": http.MethodOptions,
}

var requestMethodConstants = map[string]string{
	"MethodGet": http.MethodGet, "MethodPost": http.MethodPost, "MethodPut": http.MethodPut,
	"MethodPatch": http.MethodPatch, "MethodDelete": http.MethodDelete, "MethodHead": http.MethodHead,
	"MethodOptions": http.MethodOptions,
}

var statusCodes = map[string]int{
	"StatusOK": http.StatusOK, "StatusCreated": http.StatusCreated,
	"StatusAccepted": http.StatusAccepted, "StatusNoContent": http.StatusNoContent,
	"StatusBadRequest": http.StatusBadRequest, "StatusUnauthorized": http.StatusUnauthorized,
	"StatusForbidden": http.StatusForbidden, "StatusNotFound": http.StatusNotFound,
	"StatusMethodNotAllowed": http.StatusMethodNotAllowed, "StatusConflict": http.StatusConflict,
	"StatusUnprocessableEntity": http.StatusUnprocessableEntity,
	"StatusTooManyRequests":     http.StatusTooManyRequests,
	"StatusInternalServerError": http.StatusInternalServerError,
	"StatusBadGateway":          http.StatusBadGateway,
	"StatusServiceUnavailable":  http.StatusServiceUnavailable,
	"StatusGatewayTimeout":      http.StatusGatewayTimeout,
}

func main() {
	source := flag.String("source", "internal", "Go source directory")
	configDir := flag.String("config-dir", "conf", "configuration directory")
	output := flag.String("output", "internal/docs/openapi.yaml", "generated OpenAPI file")
	title := flag.String("title", "HTTP API", "OpenAPI title")
	flag.Parse()
	if err := run(*source, *configDir, *output, *title); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "docsgen: %v\n", err)
		os.Exit(1)
	}
}

func run(source, configDir, output, title string) error {
	servers, limits, err := loadSettings(configDir)
	if err != nil {
		return err
	}
	packages, err := loadPackages(source)
	if err != nil {
		return err
	}
	routes := discoverRoutes(packages)
	gen := &generator{limits: limits, components: make(map[string]*schema), visiting: make(map[string]bool)}
	for index := range routes {
		gen.describeRoute(&routes[index])
		if routes[index].idempotent && routes[index].method == http.MethodPost {
			routes[index].parameters = append(routes[index].parameters, parameter{
				in: "header", name: "X-Idempotent", schema: schema{typeName: "string"},
			})
			gen.ensureErrorResponse()
			addErrorResponse(&routes[index], http.StatusBadRequest, "invalid idempotency key")
			addErrorResponse(&routes[index], http.StatusConflict, "idempotency key has already been used")
			addErrorResponse(&routes[index], http.StatusInternalServerError, "internal server error")
		}

		if routeRequiresAuthentication(routes[index].path) {
			routes[index].authenticated = true
			gen.ensureErrorResponse()
			routes[index].responses[http.StatusUnauthorized] = response{
				description: http.StatusText(http.StatusUnauthorized),
				schema:      &schema{ref: "#/components/schemas/ErrorResponse"},
			}
		}

	}
	data := render(title, servers, routes, gen.components)
	if err := showDiff(output, data); err != nil {
		return err
	}
	return writeAtomic(output, data)
}

func routeRequiresAuthentication(path string) bool {
	return path != "/healthz" && path != "/pub" &&
		!strings.Contains(path, "/pub/") && !strings.HasSuffix(path, "/pub")
}

func loadSettings(configDir string) ([]docServer, pagination.Limits, error) {
	values, _, err := config.LoadValues(configDir)
	if err != nil {
		return nil, pagination.Limits{}, err
	}
	var cfg docsFileConfig
	if err := values.Unmarshal("", &cfg); err != nil {
		return nil, pagination.Limits{}, fmt.Errorf("decode docs configuration: %w", err)
	}
	limits, err := pagination.New(cfg.Pagination.MaxP, cfg.Pagination.MaxS)
	return cfg.HTTP.Docs.Servers, limits, err
}

func loadPackages(root string) ([]*sourcePackage, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("read source directory: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("source path is not a directory")
	}
	var packages []*sourcePackage
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() || path == root {
			return nil
		}
		if filepath.Clean(path) == filepath.Join(filepath.Clean(root), "docs") {
			return filepath.SkipDir
		}
		set := token.NewFileSet()
		parsed, parseErr := parser.ParseDir(set, path, func(info os.FileInfo) bool {
			return strings.HasSuffix(info.Name(), ".go") &&
				!strings.HasSuffix(info.Name(), "_test.go") && !strings.HasSuffix(info.Name(), ".gen.go")
		}, parser.SkipObjectResolution)
		if parseErr != nil {
			return fmt.Errorf("parse %q: %w", path, parseErr)
		}
		for _, parsedPackage := range parsed {
			pkg := &sourcePackage{
				name: parsedPackage.Name, funcs: make(map[string]*ast.FuncDecl),
				types: make(map[string]ast.Expr), errors: make(map[string]string),
				receivers: make(map[string]string),
			}
			for _, file := range parsedPackage.Files {
				pkg.collect(file)
			}
			packages = append(packages, pkg)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return packages, nil
}

func (p *sourcePackage) collect(file *ast.File) {
	for _, declaration := range file.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			receiver := receiverName(value.Recv)
			p.funcs[receiver+"."+value.Name.Name] = value
			if receiver != "" && value.Name.Name == "Name" {
				if name := returnedString(value); name != "" {
					p.receivers[receiver] = name
				}
			}
		case *ast.GenDecl:
			for _, item := range value.Specs {
				switch specification := item.(type) {
				case *ast.TypeSpec:
					p.types[specification.Name.Name] = specification.Type
				case *ast.ValueSpec:
					p.collectError(specification)
				}
			}
		}
	}
}

func (p *sourcePackage) collectError(value *ast.ValueSpec) {
	for index, name := range value.Names {
		if index >= len(value.Values) {
			continue
		}
		call, ok := value.Values[index].(*ast.CallExpr)
		if !ok || (len(call.Args) != 1 && len(call.Args) != 2) {
			continue
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		message, messageOK := stringLiteral(call.Args[len(call.Args)-1])
		if ok && selector.Sel.Name == "New" && messageOK {
			p.errors[name.Name] = message
		}
	}
}

func discoverRoutes(packages []*sourcePackage) []route {
	var routes []route
	for _, pkg := range packages {
		for receiver, base := range pkg.receivers {
			httpMethod := pkg.funcs[receiver+".HTTP"]
			if httpMethod == nil || httpMethod.Body == nil {
				continue
			}
			found := httpRoutes(pkg, receiver, base, httpMethod)
			idempotent := returnedBool(pkg.funcs[receiver+".Idempotent"])
			if len(found) == 0 && (httpMethod.Type.Params == nil || len(httpMethod.Type.Params.List) == 0) {
				for _, method := range requestMethods(httpMethod) {
					found = append(found, route{method: method, path: base, tag: pkg.name, handler: httpMethod, pkg: pkg})
				}
			}
			for index := range found {
				found[index].idempotent = idempotent
			}
			routes = append(routes, found...)
		}
	}
	sort.Slice(routes, func(left, right int) bool {
		if routes[left].path == routes[right].path {
			return routes[left].method < routes[right].method
		}
		return routes[left].path < routes[right].path
	})
	return routes
}

func addErrorResponse(route *route, status int, description string) {
	if previous := route.responses[status].description; previous != "" && previous != description {
		description = previous + "; " + description
	}
	route.responses[status] = response{
		description: description,
		schema:      &schema{ref: "#/components/schemas/ErrorResponse"},
	}
}

func requestMethods(function *ast.FuncDecl) []string {
	seen := make(map[string]bool)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if method := requestMethodConstants[selector.Sel.Name]; method != "" {
			seen[method] = true
		}
		return true
	})
	if len(seen) == 0 {
		return []string{http.MethodGet}
	}
	methods := make([]string, 0, len(seen))
	for method := range seen {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return methods
}

func (g *generator) describeRoute(route *route) {
	route.responses = make(map[int]response)
	route.requestBody = g.requestSchema(route.pkg, route.handler)
	ast.Inspect(route.handler.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch functionName(call.Fun) {
		case "WriteOK":
			result := response{description: http.StatusText(http.StatusOK)}
			if len(call.Args) >= 2 {
				result.schema = g.expressionSchema(route.pkg, route.handler, call.Args[1])
			}
			route.responses[http.StatusOK] = result
		case "WriteError":
			if len(call.Args) >= 3 {
				offset := 0
				if len(call.Args) == 4 {
					offset = 1
				}
				status := statusCode(call.Args[1+offset])
				if status != 0 {
					g.ensureErrorResponse()
					description := errorDescription(route.pkg, call.Args[2+offset], status)
					if previous := route.responses[status].description; previous != "" && previous != description {
						if strings.Contains("; "+previous+"; ", "; "+description+"; ") {
							description = previous
						} else {
							description = previous + "; " + description
						}
					}
					route.responses[status] = response{description: description, schema: &schema{ref: "#/components/schemas/ErrorResponse"}}
				}
			}
		case "WriteNoContent":
			route.responses[http.StatusNoContent] = response{description: http.StatusText(http.StatusNoContent)}
		case "JSON", "PureJSON", "IndentedJSON", "AbortWithStatusJSON":
			if len(call.Args) >= 2 {
				status := statusCode(call.Args[0])
				if status != 0 {
					route.responses[status] = response{description: http.StatusText(status), schema: g.expressionSchema(route.pkg, route.handler, call.Args[1])}
				}
			}
		case "Status", "AbortWithStatus":
			if len(call.Args) == 1 {
				status := statusCode(call.Args[0])
				if status != 0 {
					route.responses[status] = response{description: http.StatusText(status)}
				}
			}
		case "WriteJSON":
			if len(call.Args) >= 3 {
				status := statusCode(call.Args[1])
				if status != 0 {
					route.responses[status] = response{description: http.StatusText(status), schema: g.expressionSchema(route.pkg, route.handler, call.Args[2])}
				}
			}
		}
		return true
	})
	for _, name := range pathParameters(route.path) {
		route.parameters = append(route.parameters, parameter{name: name, in: "path", required: true, schema: parameterType(route.handler, name)})
	}
	route.parameters = append(route.parameters, queryParameters(route.handler, g.limits)...)
}

func (g *generator) expressionSchema(pkg *sourcePackage, function *ast.FuncDecl, expression ast.Expr) *schema {
	if identifier, ok := expression.(*ast.Ident); ok {
		if inferred := inferredIdentifierType(pkg, function, identifier.Name); inferred != nil {
			return g.typeSchema(pkg, inferred)
		}
	}
	if literal, ok := expression.(*ast.CompositeLit); ok {
		_, mapValue := literal.Type.(*ast.MapType)
		if named, ok := literal.Type.(*ast.SelectorExpr); ok && named.Sel.Name == "H" {
			mapValue = true
		}
		if mapValue {
			properties := make(map[string]*schema)
			for _, element := range literal.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				name, ok := stringLiteral(pair.Key)
				if ok {
					properties[name] = g.literalSchema(pkg, pair.Value)
				}
			}
			return &schema{typeName: "object", properties: properties, required: sortedKeys(properties)}
		}
		return g.typeSchema(pkg, literal.Type)
	}
	return g.literalSchema(pkg, expression)
}

func inferredIdentifierType(pkg *sourcePackage, function *ast.FuncDecl, name string) ast.Expr {
	var result ast.Expr
	if function == nil {
		return nil
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if result != nil {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for index, left := range value.Lhs {
				identifier, ok := left.(*ast.Ident)
				if !ok || identifier.Name != name {
					continue
				}
				if len(value.Rhs) == len(value.Lhs) {
					if literal, ok := value.Rhs[index].(*ast.CompositeLit); ok {
						result = literal.Type
					}
				} else if len(value.Rhs) == 1 {
					result = callResultType(pkg, value.Rhs[0], index)
				}
			}
		case *ast.ValueSpec:
			for _, identifier := range value.Names {
				if identifier.Name == name {
					result = value.Type
				}
			}
		}
		return true
	})
	return result
}

func callResultType(pkg *sourcePackage, expression ast.Expr, index int) ast.Expr {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return nil
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	for key, function := range pkg.funcs {
		if !strings.HasSuffix(key, "."+selector.Sel.Name) || function.Type.Results == nil {
			continue
		}
		results := flattenFields(function.Type.Results.List)
		if index < len(results) {
			return results[index]
		}
	}
	return nil
}

func flattenFields(fields []*ast.Field) []ast.Expr {
	var result []ast.Expr
	for _, field := range fields {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			result = append(result, field.Type)
		}
	}
	return result
}

func (g *generator) literalSchema(pkg *sourcePackage, expression ast.Expr) *schema {
	switch value := expression.(type) {
	case *ast.BasicLit:
		switch value.Kind {
		case token.STRING:
			text, _ := strconv.Unquote(value.Value)
			return &schema{typeName: "string", example: text}
		case token.INT:
			return &schema{typeName: "integer", example: value.Value}
		case token.FLOAT:
			return &schema{typeName: "number", example: value.Value}
		}
	case *ast.CompositeLit:
		return g.expressionSchema(pkg, nil, value)
	case *ast.Ident:
		if value.Name == "true" || value.Name == "false" {
			return &schema{typeName: "boolean", example: value.Name}
		}
	}
	return &schema{typeName: "object"}
}

func (g *generator) typeSchema(pkg *sourcePackage, expression ast.Expr) *schema {
	switch value := expression.(type) {
	case *ast.StarExpr:
		return g.typeSchema(pkg, value.X)
	case *ast.ArrayType:
		return &schema{typeName: "array", items: g.typeSchema(pkg, value.Elt)}
	case *ast.MapType:
		return &schema{typeName: "object", additionalProperties: g.typeSchema(pkg, value.Value)}
	case *ast.SelectorExpr:
		if identifier, ok := value.X.(*ast.Ident); ok && identifier.Name == "time" && value.Sel.Name == "Time" {
			return &schema{typeName: "string", format: "date-time"}
		}
		return &schema{typeName: "object"}
	case *ast.StructType:
		return g.structSchema(pkg, value)
	case *ast.InterfaceType:
		return &schema{typeName: "object"}
	case *ast.Ident:
		if primitive := primitiveSchema(value.Name); primitive != nil {
			return primitive
		}
		declaration := pkg.types[value.Name]
		if declaration == nil {
			return &schema{typeName: "object"}
		}
		if _, ok := declaration.(*ast.StructType); !ok {
			return g.typeSchema(pkg, declaration)
		}
		if !g.visiting[value.Name] {
			g.visiting[value.Name] = true
			g.components[value.Name] = g.typeSchema(pkg, declaration)
			delete(g.visiting, value.Name)
		}
		return &schema{ref: "#/components/schemas/" + value.Name}
	default:
		return &schema{typeName: "object"}
	}
}

func (g *generator) structSchema(pkg *sourcePackage, value *ast.StructType) *schema {
	properties := make(map[string]*schema)
	var required []string
	for _, field := range value.Fields.List {
		if len(field.Names) == 0 || !field.Names[0].IsExported() {
			continue
		}
		name := lowerFirst(field.Names[0].Name)
		optional := false
		if field.Tag != nil {
			tag, _ := strconv.Unquote(field.Tag.Value)
			jsonName := strings.Split(reflect.StructTag(tag).Get("json"), ",")
			if len(jsonName) > 0 && jsonName[0] == "-" {
				continue
			}
			if len(jsonName) > 0 && jsonName[0] != "" {
				name = jsonName[0]
			}
			optional = strings.Contains(reflect.StructTag(tag).Get("json"), "omitempty")
		}
		properties[name] = g.typeSchema(pkg, field.Type)
		if field.Tag != nil {
			tag, _ := strconv.Unquote(field.Tag.Value)
			properties[name].example = reflect.StructTag(tag).Get("example")
		}
		if !optional {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	return &schema{typeName: "object", properties: properties, required: required}
}

func primitiveSchema(name string) *schema {
	switch name {
	case "string":
		return &schema{typeName: "string"}
	case "bool":
		return &schema{typeName: "boolean"}
	case "int", "int8", "int16", "int32", "uint", "uint8", "uint16", "uint32":
		return &schema{typeName: "integer", format: "int32"}
	case "int64", "uint64":
		return &schema{typeName: "integer", format: "int64"}
	case "float32":
		return &schema{typeName: "number", format: "float"}
	case "float64":
		return &schema{typeName: "number", format: "double"}
	case "any":
		return &schema{typeName: "object"}
	default:
		return nil
	}
}

func (g *generator) ensureErrorResponse() {
	if g.components["ErrorResponse"] != nil {
		return
	}
	g.components["ErrorResponse"] = &schema{
		typeName:   "object",
		properties: map[string]*schema{"msg": {typeName: "string"}, "error_code": {typeName: "string"}},
		required:   []string{"msg", "error_code"},
	}
}

func parameterType(function *ast.FuncDecl, name string) schema {
	result := schema{typeName: "string"}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if functionName(call.Fun) == "Split" && len(call.Args) == 2 && urlParamName(call.Args[0]) == name {
			if delimiter, ok := stringLiteral(call.Args[1]); ok && delimiter == "," {
				result = schema{typeName: "string", example: "1,2,3"}
			}
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (selector.Sel.Name != "ParseInt" && selector.Sel.Name != "Atoi") || len(call.Args) == 0 {
			return true
		}
		if urlParamName(call.Args[0]) == name {
			result = schema{typeName: "integer", format: "int64"}
		}
		return true
	})
	return result
}

func urlParamName(expression ast.Expr) string {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return ""
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	index := 1
	switch selector.Sel.Name {
	case "URLParam":
	case "Param":
		index = 0
	default:
		return ""
	}
	if len(call.Args) <= index {
		return ""
	}
	value, _ := stringLiteral(call.Args[index])
	return value
}

func statusCode(expression ast.Expr) int {
	switch value := expression.(type) {
	case *ast.BasicLit:
		parsed, _ := strconv.Atoi(value.Value)
		return parsed
	case *ast.SelectorExpr:
		return statusCodes[value.Sel.Name]
	default:
		return 0
	}
}

func functionName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.Ident:
		return value.Name
	default:
		return ""
	}
}

func errorDescription(pkg *sourcePackage, expression ast.Expr, status int) string {
	if identifier, ok := expression.(*ast.Ident); ok && pkg.errors[identifier.Name] != "" {
		return pkg.errors[identifier.Name]
	}
	if value, ok := stringLiteral(expression); ok {
		return value
	}
	if call, ok := expression.(*ast.CallExpr); ok {
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
			if selector.Sel.Name == "Error" {
				if identifier, ok := selector.X.(*ast.Ident); ok && pkg.errors[identifier.Name] != "" {
					return pkg.errors[identifier.Name]
				}
			}
			if selector.Sel.Name == "StatusText" {
				return http.StatusText(status)
			}
		}
	}
	if description := http.StatusText(status); description != "" {
		return description
	}
	return "Error"
}

func render(title string, servers []docServer, routes []route, components map[string]*schema) []byte {
	var output strings.Builder
	output.WriteString("# Code generated by docsgen; DO NOT EDIT.\n\nopenapi: 3.0.3\ninfo:\n  title: ")
	output.WriteString(strconv.Quote(title))
	output.WriteString("\n  version: 1.0.0\n")
	if len(servers) > 0 {
		output.WriteString("servers:\n")
		for _, server := range servers {
			output.WriteString("  - url: ")
			output.WriteString(strconv.Quote(server.URL))
			output.WriteByte('\n')
			if server.Description != "" {
				output.WriteString("    description: ")
				output.WriteString(strconv.Quote(server.Description))
				output.WriteByte('\n')
			}
		}
	}
	output.WriteString("paths:")
	if len(routes) == 0 {
		output.WriteString(" {}\n")
	} else {
		output.WriteByte('\n')
	}
	for index, item := range routes {
		if index == 0 || routes[index-1].path != item.path {
			output.WriteString("  ")
			output.WriteString(item.path)
			output.WriteString(":\n")
		}
		output.WriteString("    ")
		output.WriteString(strings.ToLower(item.method))
		output.WriteString(":\n      summary: ")
		output.WriteString(strconv.Quote(item.method + " " + item.path))
		output.WriteString("\n      operationId: ")
		output.WriteString(operationID(item.method, item.path))
		output.WriteString("\n      tags: [")
		output.WriteString(strconv.Quote(upperFirst(item.tag)))
		output.WriteString("]\n")
		if item.authenticated {
			output.WriteString("      security:\n        - bearerAuth: []\n")
		}
		if len(item.parameters) > 0 {
			output.WriteString("      parameters:\n")
			for _, parameter := range item.parameters {
				output.WriteString("        - name: ")
				output.WriteString(parameter.name)
				location := parameter.in
				if location == "" {
					location = "path"
				}
				fmt.Fprintf(&output, "\n          in: %s\n          required: %t\n          schema:\n", location, parameter.required || location == "path")
				renderSchema(&output, &parameter.schema, 12)
			}
		}
		if item.requestBody != nil {
			output.WriteString("      requestBody:\n        required: true\n        content:\n          application/json:\n            schema:\n")
			renderSchema(&output, item.requestBody, 14)
		}
		output.WriteString("      responses:")
		statuses := sortedStatuses(item.responses)
		if len(statuses) == 0 {
			output.WriteString(" {}\n")
			continue
		}
		output.WriteByte('\n')
		for _, status := range statuses {
			value := item.responses[status]
			fmt.Fprintf(&output, "        %q:\n          description: %s\n", strconv.Itoa(status), strconv.Quote(value.description))
			if value.schema != nil {
				output.WriteString("          content:\n            application/json:\n              schema:\n")
				renderSchema(&output, value.schema, 16)
			}
		}
	}
	hasAuthentication := false
	for _, item := range routes {
		hasAuthentication = hasAuthentication || item.authenticated
	}
	if len(components) > 0 || hasAuthentication {
		output.WriteString("components:\n")
		if hasAuthentication {
			output.WriteString("  securitySchemes:\n    bearerAuth:\n      type: http\n      scheme: bearer\n      bearerFormat: JWT\n")
		}
		if len(components) > 0 {
			output.WriteString("  schemas:\n")
		}
		names := sortedKeys(components)
		for _, name := range names {
			output.WriteString("    ")
			output.WriteString(name)
			output.WriteString(":\n")
			renderSchema(&output, components[name], 6)
		}
	}
	return []byte(output.String())
}

func renderSchema(output *strings.Builder, value *schema, indent int) {
	spaces := strings.Repeat(" ", indent)
	if value.ref != "" {
		output.WriteString(spaces + "$ref: " + strconv.Quote(value.ref) + "\n")
		return
	}
	output.WriteString(spaces + "type: " + value.typeName + "\n")
	if value.format != "" {
		output.WriteString(spaces + "format: " + value.format + "\n")
	}
	if value.paginationKey != "" {
		fmt.Fprintf(output, "%sx-pagination: %s\n", spaces, value.paginationKey)
		fmt.Fprintf(output, "%sdescription: %q\n", spaces, fmt.Sprintf("Values outside 1..%d return an empty list.", value.paginationMax))
	}
	if value.defaultValue != nil {
		fmt.Fprintf(output, "%sdefault: %d\n", spaces, *value.defaultValue)
	}
	if value.example != "" {
		output.WriteString(spaces + "example: " + schemaExample(value) + "\n")
	}
	if value.items != nil {
		output.WriteString(spaces + "items:\n")
		renderSchema(output, value.items, indent+2)
	}
	if value.additionalProperties != nil {
		output.WriteString(spaces + "additionalProperties:\n")
		renderSchema(output, value.additionalProperties, indent+2)
	}
	if len(value.required) > 0 {
		output.WriteString(spaces + "required: [")
		for index, name := range value.required {
			if index > 0 {
				output.WriteString(", ")
			}
			output.WriteString(name)
		}
		output.WriteString("]\n")
	}
	if len(value.properties) > 0 {
		output.WriteString(spaces + "properties:\n")
		for _, name := range sortedKeys(value.properties) {
			output.WriteString(spaces + "  " + name + ":\n")
			renderSchema(output, value.properties[name], indent+4)
		}
	}
}

func writeAtomic(output string, data []byte) error {
	current, err := os.ReadFile(output)
	if err == nil && bytes.Equal(current, data) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(output), ".openapi-*.yaml")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write output: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set output permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	if err := os.Rename(name, output); err != nil {
		return fmt.Errorf("replace output: %w", err)
	}
	return nil
}

func showDiff(output string, data []byte) error {
	current, err := os.ReadFile(output)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Printf("create %s\n", output)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read OpenAPI document: %w", err)
	}
	if bytes.Equal(current, data) {
		fmt.Printf("%s unchanged\n", output)
		return nil
	}
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(current)),
		B:        difflib.SplitLines(string(data)),
		FromFile: output,
		ToFile:   output + " (generated)",
		Context:  3,
	})
	if err != nil {
		return fmt.Errorf("render OpenAPI diff: %w", err)
	}
	fmt.Print(diff)
	return nil
}

func receiverName(list *ast.FieldList) string {
	if list == nil || len(list.List) == 0 {
		return ""
	}
	expression := list.List[0].Type
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	identifier, _ := expression.(*ast.Ident)
	if identifier == nil {
		return ""
	}
	return identifier.Name
}

func returnedString(function *ast.FuncDecl) string {
	if function.Body == nil {
		return ""
	}
	for _, statement := range function.Body.List {
		if returned, ok := statement.(*ast.ReturnStmt); ok && len(returned.Results) == 1 {
			value, _ := stringLiteral(returned.Results[0])
			return value
		}
	}
	return ""
}

func returnedBool(function *ast.FuncDecl) bool {
	if function == nil || function.Body == nil {
		return false
	}
	for _, statement := range function.Body.List {
		if returned, ok := statement.(*ast.ReturnStmt); ok && len(returned.Results) == 1 {
			value, ok := returned.Results[0].(*ast.Ident)
			return ok && value.Name == "true"
		}
	}
	return false
}

func stringLiteral(expression ast.Expr) (string, bool) {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func joinPath(base, child string) string {
	if child == "" || child == "/" {
		return base
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(child, "/")
}

func pathParameters(path string) []string {
	var result []string
	for {
		start := strings.IndexByte(path, '{')
		if start < 0 {
			return result
		}
		path = path[start+1:]
		end := strings.IndexByte(path, '}')
		if end < 0 {
			return result
		}
		result = append(result, path[:end])
		path = path[end+1:]
	}
}

func operationID(method, path string) string {
	parts := strings.FieldsFunc(path, func(value rune) bool { return !unicode.IsLetter(value) && !unicode.IsDigit(value) })
	result := strings.ToLower(method)
	for _, part := range parts {
		result += upperFirst(part)
	}
	return result
}

func sortedStatuses(values map[int]response) []int {
	result := make([]int, 0, len(values))
	for status := range values {
		result = append(result, status)
	}
	sort.Ints(result)
	return result
}

func sortedKeys[T any](values map[string]T) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func upperFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
