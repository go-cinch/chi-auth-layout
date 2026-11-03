package main

import (
	"fmt"
	"go/ast"
	"strings"
)

func (g *generator) describeBodyLimit(r *route) {
	if r.requestBody == nil || r.handler == nil {
		return
	}
	limit := 0
	ast.Inspect(r.handler.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || functionName(call.Fun) != "DecodeJSON" || len(call.Args) != 4 {
			return true
		}
		category, ok := call.Args[3].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		var size int
		switch category.Sel.Name {
		case "BodyAuthChallenge":
			size = g.bodyLimits.AuthChallenge
		case "BodySliderChallenge":
			size = g.bodyLimits.SliderChallenge
		case "BodyAuthVerification":
			size = g.bodyLimits.AuthVerification
		case "BodyAuth":
			size = g.bodyLimits.Auth
		case "BodyBusiness":
			size = g.bodyLimits.Business
		case "BodyDictionary":
			size = g.bodyLimits.Dictionary
		}
		limit = max(limit, size)
		return true
	})
	if limit > 0 {
		r.requestBody.description = strings.TrimSpace(r.requestBody.description + fmt.Sprintf(" Maximum JSON request body: %d bytes.", limit))
	}
}
