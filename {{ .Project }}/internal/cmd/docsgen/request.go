package main

import (
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
)

// Keep distinct decoded inputs from alternative branches. Anonymous trailing-data
// checks must not widen a typed request into arbitrary JSON.
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
	var alternatives []*schema
	var trailingOnly *schema
	add := func(expression ast.Expr, strict bool) {
		if address, ok := expression.(*ast.UnaryExpr); ok && address.Op == token.AND {
			expression = address.X
		}
		if call, ok := expression.(*ast.CallExpr); ok {
			if function, ok := call.Fun.(*ast.Ident); ok && function.Name == "new" && len(call.Args) == 1 {
				value := g.typeSchema(pkg, call.Args[0])
				if value.anyJSON {
					trailingOnly = value
					return
				}
			}
		}
		value := g.expressionSchema(pkg, handler, expression)
		if name, ok := expression.(*ast.Ident); ok && rejectsNilRequest(handler, name.Name) {
			value.nullable = false
		}
		// A strict decoder accepts exactly {} for an inline empty struct. Preserve
		// this constraint so it cannot swallow the other request alternatives.
		if strict && value.typeName == "object" && value.properties != nil && len(value.properties) == 0 {
			value.closedObject = true
		}
		for _, previous := range alternatives {
			if reflect.DeepEqual(previous, value) {
				return
			}
		}
		alternatives = append(alternatives, value)
	}
	ast.Inspect(handler.Body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		// Business boundaries share bounded, strict JSON decoders.
		if functionName(call.Fun) == "DecodeJSON" && len(call.Args) == 4 {
			add(call.Args[2], true)
			return false
		}
		if len(call.Args) != 1 {
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
			add(call.Args[0], false)
		}
		return true
	})
	switch len(alternatives) {
	case 0:
		return trailingOnly
	case 1:
		return alternatives[0]
	default:
		return &schema{anyOf: alternatives}
	}
}

// A pointer is a decoding detail when the handler explicitly rejects JSON null.
// Recognize the direct nil guard without changing nullability of nested fields.
func rejectsNilRequest(handler *ast.FuncDecl, name string) bool {
	rejected := false
	ast.Inspect(handler.Body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		guard, ok := node.(*ast.IfStmt)
		if !ok || len(guard.Body.List) == 0 {
			return true
		}
		condition, ok := guard.Cond.(*ast.BinaryExpr)
		if !ok || condition.Op != token.EQL {
			return true
		}
		left, leftOK := condition.X.(*ast.Ident)
		right, rightOK := condition.Y.(*ast.Ident)
		if !leftOK || !rightOK || !((left.Name == name && right.Name == "nil") || (left.Name == "nil" && right.Name == name)) {
			return true
		}
		if _, returns := guard.Body.List[len(guard.Body.List)-1].(*ast.ReturnStmt); returns {
			rejected = true
		}
		return true
	})
	return rejected
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
