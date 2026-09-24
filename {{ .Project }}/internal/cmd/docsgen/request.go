package main

import (
	"go/ast"
	"go/token"
	"strconv"
)

// Use the first JSON decode; a trailing-data check does not define the request schema.
func (g *generator) requestSchema(pkg *sourcePackage, handler *ast.FuncDecl) *schema {
	decoders := map[string]bool{}
	ast.Inspect(handler.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assignment.Lhs {
			name, ok := lhs.(*ast.Ident)
			if ok && i < len(assignment.Rhs) && jsonBodyDecoder(assignment.Rhs[i]) {
				decoders[name.Name] = true
			}
		}
		return true
	})
	var result *schema
	ast.Inspect(handler.Body, func(node ast.Node) bool {
		if result != nil {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		body := false
		switch selector.Sel.Name {
		case "Decode":
			body = jsonBodyDecoder(selector.X)
			if receiver, ok := selector.X.(*ast.Ident); ok {
				body = decoders[receiver.Name]
			}
		case "BindJSON", "ShouldBindJSON":
			if receiver, ok := selector.X.(*ast.Ident); ok {
				for _, param := range handler.Type.Params.List {
					pointer, ok := param.Type.(*ast.StarExpr)
					if !ok {
						continue
					}
					typ, ok := pointer.X.(*ast.SelectorExpr)
					if !ok || typ.Sel.Name != "Context" {
						continue
					}
					for _, name := range param.Names {
						if name.Name == receiver.Name {
							body = true
						}
					}
				}
			}
		}
		if body {
			expression := call.Args[0]
			if address, ok := expression.(*ast.UnaryExpr); ok && address.Op == token.AND {
				expression = address.X
			}
			result = g.expressionSchema(pkg, handler, expression)
		}
		return true
	})
	return result
}

func jsonBodyDecoder(expression ast.Expr) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "NewDecoder" {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "json" {
		return false
	}
	body := false
	ast.Inspect(call.Args[0], func(node ast.Node) bool {
		if field, ok := node.(*ast.SelectorExpr); ok && field.Sel.Name == "Body" {
			body = true
		}
		return true
	})
	return body
}

func schemaExample(value *schema) string {
	switch value.typeName {
	case "integer":
		if n, err := strconv.ParseInt(value.example, 10, 64); err == nil {
			return strconv.FormatInt(n, 10)
		}
	case "number":
		if n, err := strconv.ParseFloat(value.example, 64); err == nil {
			return strconv.FormatFloat(n, 'g', -1, 64)
		}
	case "boolean":
		if b, err := strconv.ParseBool(value.example); err == nil {
			return strconv.FormatBool(b)
		}
	}
	return strconv.Quote(value.example)
}
