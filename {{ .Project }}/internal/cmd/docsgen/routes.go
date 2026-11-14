package main

import (
	"go/ast"
	"net/http"
	"sort"
	"strings"
)

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

// httpRoutes follows literal route groups and inline chi Route/Group callbacks.
// Router scopes are tracked so unrelated Get/POST method calls are not routes.
func httpRoutes(pkg *sourcePackage, receiver, base string, function *ast.FuncDecl) []route {
	var routes []route
	initial := map[string]string{}
	if function.Type.Params != nil {
		for _, field := range function.Type.Params.List {
			for _, name := range field.Names {
				initial[name.Name] = base
			}
		}
	}
	var routerPath func(ast.Expr, map[string]string) (string, bool)
	routerPath = func(expression ast.Expr, scopes map[string]string) (string, bool) {
		if name, ok := expression.(*ast.Ident); ok {
			path, found := scopes[name.Name]
			return path, found
		}
		call, ok := expression.(*ast.CallExpr)
		if !ok {
			return "", false
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		if selector.Sel.Name == "NewRouter" {
			return base, true
		}
		parent, ok := routerPath(selector.X, scopes)
		if !ok {
			return "", false
		}
		switch selector.Sel.Name {
		case "With":
			return parent, true
		case "Group":
			if len(call.Args) > 0 {
				if child, ok := stringLiteral(call.Args[0]); ok {
					return joinPath(parent, child), true
				}
			}
		}
		return "", false
	}
	var walk func(*ast.BlockStmt, map[string]string)
	walk = func(block *ast.BlockStmt, parent map[string]string) {
		scopes := make(map[string]string, len(parent))
		for key, value := range parent {
			scopes[key] = value
		}
		for _, statement := range block.List {
			switch node := statement.(type) {
			case *ast.BlockStmt:
				walk(node, scopes)
			case *ast.AssignStmt:
				for index, lhs := range node.Lhs {
					name, ok := lhs.(*ast.Ident)
					if !ok || index >= len(node.Rhs) {
						continue
					}
					if path, ok := routerPath(node.Rhs[index], scopes); ok {
						scopes[name.Name] = path
					} else {
						delete(scopes, name.Name)
					}
				}
			case *ast.ExprStmt:
				call, ok := node.X.(*ast.CallExpr)
				if !ok {
					continue
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				prefix, ok := routerPath(selector.X, scopes)
				if !ok {
					continue
				}
				name := selector.Sel.Name
				if name == "Route" || name == "Group" {
					childBase := prefix
					for _, argument := range call.Args {
						if child, ok := stringLiteral(argument); ok {
							childBase = joinPath(prefix, child)
						}
						if callback, ok := argument.(*ast.FuncLit); ok {
							nested := make(map[string]string, len(scopes))
							for key, value := range scopes {
								nested[key] = value
							}
							for _, field := range callback.Type.Params.List {
								for _, arg := range field.Names {
									nested[arg.Name] = childBase
								}
							}
							walk(callback.Body, nested)
						}
					}
					continue
				}
				method := routeMethods[name]
				if method == "" {
					method = routeMethods[strings.Title(strings.ToLower(name))]
				}
				pathIndex := 0
				if name == "Handle" || name == "Method" || name == "MethodFunc" {
					if len(call.Args) < 3 {
						continue
					}
					method, _ = stringLiteral(call.Args[0])
					if constant, ok := call.Args[0].(*ast.SelectorExpr); ok {
						method = requestMethodConstants[constant.Sel.Name]
					}
					pathIndex = 1
				}
				if method == "" || len(call.Args) < pathIndex+2 {
					continue
				}
				path, ok := stringLiteral(call.Args[pathIndex])
				if !ok {
					continue
				}
				handler := resolveHandler(pkg, receiver, call.Args[len(call.Args)-1])
				if handler == nil {
					continue
				}
				routes = append(routes, route{method: method, path: openAPIPath(joinPath(prefix, path)), tag: pkg.name, handler: handler, pkg: pkg})
			}
		}
	}
	walk(function.Body, initial)
	return routes
}

func openAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for index, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			name := part[1:]
			if name == "" {
				name = "wildcard"
			}
			parts[index] = "{" + name + "}"
		}
	}
	return strings.Join(parts, "/")
}

func resolveHandler(pkg *sourcePackage, receiver string, expression ast.Expr) *ast.FuncDecl {
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		return pkg.funcs[receiver+"."+value.Sel.Name]
	case *ast.Ident:
		return pkg.funcs["."+value.Name]
	case *ast.FuncLit:
		return &ast.FuncDecl{Type: value.Type, Body: value.Body}
	case *ast.CallExpr:
		name := functionName(value.Fun)
		if (name == "WrapH" || name == "WrapF") && len(value.Args) == 1 {
			return resolveHandler(pkg, receiver, value.Args[0])
		}
		return resolveHandler(pkg, receiver, value.Fun)
	default:
		return nil
	}
}
