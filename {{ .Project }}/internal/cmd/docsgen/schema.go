package main

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type parameter struct {
	description string
	in          string
	required    bool
	name        string
	schema      schema
}

type schema struct {
	description          string
	anyOf                []*schema
	closedObject         bool
	nullable             bool
	anyJSON              bool
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
		result := *g.typeSchema(pkg, value.X)
		// Scalar and inline field pointers serialize as null when absent. References
		// describe successful resource responses whose missing case is an HTTP error.
		if result.ref == "" {
			result.nullable = true
		}
		return &result
	case *ast.ArrayType:
		return &schema{typeName: "array", items: g.typeSchema(pkg, value.Elt)}
	case *ast.MapType:
		return &schema{typeName: "object", additionalProperties: g.typeSchema(pkg, value.Value)}
	case *ast.SelectorExpr:
		if identifier, ok := value.X.(*ast.Ident); ok && identifier.Name == "json" && value.Sel.Name == "RawMessage" {
			return &schema{anyJSON: true}
		}
		if identifier, ok := value.X.(*ast.Ident); ok && identifier.Name == "time" && value.Sel.Name == "Time" {
			return &schema{typeName: "string", format: "date-time"}
		}
		if identifier, ok := value.X.(*ast.Ident); ok && pkg != nil {
			if dependency := pkg.dependencies[identifier.Name]; dependency != nil {
				return g.typeSchema(dependency, ast.NewIdent(value.Sel.Name))
			}
		}
		return &schema{typeName: "object"}
	case *ast.StructType:
		return g.structSchema(pkg, value)
	case *ast.InterfaceType:
		return &schema{anyJSON: true}
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
		name := value.Name
		if qualified := g.schemaNames[pkg][name]; qualified != "" {
			name = qualified
		}
		if !g.visiting[name] {
			g.visiting[name] = true
			g.components[name] = g.typeSchema(pkg, declaration)
			delete(g.visiting, name)
		}
		return &schema{ref: "#/components/schemas/" + name}
	default:
		return &schema{typeName: "object"}
	}
}

func (g *generator) structSchema(pkg *sourcePackage, value *ast.StructType) *schema {
	properties := make(map[string]*schema)
	var required []string
	for _, field := range value.Fields.List {
		if len(field.Names) == 0 {
			// Untagged exported embeddings flatten into the JSON object. The
			// existing type visitor bounds recursive references.
			typ := field.Type
			if pointer, ok := typ.(*ast.StarExpr); ok {
				typ = pointer.X
			}
			name, ok := typ.(*ast.Ident)
			if !ok || !name.IsExported() || field.Tag != nil {
				continue
			}
			embedded := g.typeSchema(pkg, field.Type)
			if embedded.ref != "" {
				embedded = g.components[strings.TrimPrefix(embedded.ref, "#/components/schemas/")]
			}
			if embedded != nil {
				for name, property := range embedded.properties {
					properties[name] = property
				}
				required = append(required, embedded.required...)
			}
			continue
		}
		if !field.Names[0].IsExported() {
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
		if _, pointer := field.Type.(*ast.StarExpr); pointer {
			properties[name].nullable = true
		}
		if field.Tag != nil {
			tag, _ := strconv.Unquote(field.Tag.Value)
			properties[name].example = reflect.StructTag(tag).Get("example")
			if reflect.StructTag(tag).Get("nullable") == "true" {
				properties[name].nullable = true
			}
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
		return &schema{anyJSON: true}
	default:
		return nil
	}
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

// registerSchemaNames disambiguates package-owned contracts before routes are
// visited, so discovery order cannot change a previously emitted reference.
func (g *generator) registerSchemaNames(packages []*sourcePackage, root string) {
	owners := map[string][]*sourcePackage{}
	for _, pkg := range packages {
		for name, declaration := range pkg.types {
			if _, object := declaration.(*ast.StructType); object {
				owners[name] = append(owners[name], pkg)
			}
		}
	}
	g.schemaNames = map[*sourcePackage]map[string]string{}
	for name, packages := range owners {
		if len(packages) < 2 {
			continue
		}
		for _, pkg := range packages {
			relative, err := filepath.Rel(root, pkg.directory)
			if err != nil {
				relative = pkg.name
			}
			if g.schemaNames[pkg] == nil {
				g.schemaNames[pkg] = map[string]string{}
			}
			g.schemaNames[pkg][name] = strings.ReplaceAll(filepath.ToSlash(relative), "/", ".") + "." + name
		}
	}
}
