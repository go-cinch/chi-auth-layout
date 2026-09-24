package main

import (
	"go/ast"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
)

// Infer query types and defaults; describe paging ranges from configuration.
func queryParameters(handler *ast.FuncDecl, configured ...pagination.Limits) []parameter {
	limits, _ := pagination.New(0, 0)
	if len(configured) > 0 {
		limits, _ = pagination.New(int(configured[0].MaxP), int(configured[0].MaxS))
	}
	aliases := map[string]bool{}
	ast.Inspect(handler.Body, func(node ast.Node) bool {
		if assignment, ok := node.(*ast.AssignStmt); ok {
			for i, rhs := range assignment.Rhs {
				if i < len(assignment.Lhs) && isQueryValues(rhs) {
					if variable, ok := assignment.Lhs[i].(*ast.Ident); ok {
						aliases[variable.Name] = true
					}
				}
			}
		}
		return true
	})
	nameOf := func(expression ast.Expr) string { return queryName(expression, aliases) }
	found := map[string]schema{}
	variables := map[string]string{}
	ast.Inspect(handler.Body, func(node ast.Node) bool {
		if expression, ok := node.(ast.Expr); ok {
			if name := nameOf(expression); name != "" {
				found[name] = schema{typeName: "string"}
			}
		}
		if assignment, ok := node.(*ast.AssignStmt); ok {
			for i, rhs := range assignment.Rhs {
				if i >= len(assignment.Lhs) {
					continue
				}
				if name := nameOf(rhs); name != "" {
					if variable, ok := assignment.Lhs[i].(*ast.Ident); ok {
						variables[variable.Name] = name
					}
				}
			}
		}
		return true
	})
	// Track query variables within each conditional scope.
	var inspect func(ast.Node, map[string]string)
	inspect = func(root ast.Node, vars map[string]string) {
		ast.Inspect(root, func(node ast.Node) bool {
			if conditional, ok := node.(*ast.IfStmt); ok && node != root {
				scoped := make(map[string]string, len(vars))
				for key, value := range vars {
					scoped[key] = value
				}
				if init, ok := conditional.Init.(*ast.AssignStmt); ok {
					for i, rhs := range init.Rhs {
						if i < len(init.Lhs) {
							if key := nameOf(rhs); key != "" {
								if variable, ok := init.Lhs[i].(*ast.Ident); ok {
									scoped[variable.Name] = key
								}
							}
						}
					}
				}
				if conditional.Init != nil {
					inspect(conditional.Init, scoped)
				}
				inspect(conditional.Cond, scoped)
				inspect(conditional.Body, scoped)
				if conditional.Else != nil {
					inspect(conditional.Else, scoped)
				}
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			method := functionName(call.Fun)
			if method != "ParseInt" && method != "Atoi" && method != "ParseBool" {
				return true
			}
			name := nameOf(call.Args[0])
			if name == "" {
				name = queryVariable(call.Args[0], vars)
			}
			if name != "" {
				value := schema{typeName: "integer", format: "int64"}
				if method == "ParseBool" {
					value = schema{typeName: "boolean"}
				}
				if method == "ParseInt" && len(call.Args) == 3 {
					if bits, ok := call.Args[2].(*ast.BasicLit); ok && bits.Value == "32" {
						value.format = "int32"
					}
				}
				found[name] = value
			}
			return true
		})
	}
	inspect(handler.Body, variables)
	for name, value := range found {
		if (name == "p" || name == "s") && value.typeName == "integer" {
			max, fallback := int64(limits.MaxP), int64(1)
			if name == "s" {
				max = int64(limits.MaxS)
				fallback = int64(limits.DefaultSize())
			}
			value.paginationMax = max
			value.defaultValue = &fallback
			value.paginationKey = name
			found[name] = value
		}
	}
	var result []parameter
	for _, name := range sortedKeys(found) {
		result = append(result, parameter{name: name, in: "query", schema: found[name]})
	}
	return result
}

func queryName(expression ast.Expr, aliases map[string]bool) string {
	if index, ok := expression.(*ast.IndexExpr); ok {
		if variable, ok := index.X.(*ast.Ident); ok && aliases[variable.Name] {
			name, _ := stringLiteral(index.Index)
			return name
		}
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return ""
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	switch selector.Sel.Name {
	case "Query", "DefaultQuery", "GetQuery", "GetQueryArray":
	case "Get":
		if variable, ok := selector.X.(*ast.Ident); !ok || !aliases[variable.Name] {
			if !isQueryValues(selector.X) {
				return ""
			}
		}
	default:
		return ""
	}
	name, _ := stringLiteral(call.Args[0])
	return name
}

func isQueryValues(expression ast.Expr) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if selector.Sel.Name == "Query" && len(call.Args) == 0 {
		field, ok := selector.X.(*ast.SelectorExpr)
		return ok && field.Sel.Name == "URL"
	}
	if selector.Sel.Name == "ParseQuery" {
		pkg, ok := selector.X.(*ast.Ident)
		return ok && pkg.Name == "url"
	}
	return false
}

func queryVariable(expression ast.Expr, variables map[string]string) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return variables[value.Name]
	case *ast.IndexExpr:
		return queryVariable(value.X, variables)
	}
	return ""
}
