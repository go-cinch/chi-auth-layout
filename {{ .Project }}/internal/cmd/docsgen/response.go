package main

import (
	"go/ast"
	"net/http"
	"strconv"
	"strings"
)

type response struct {
	headers     []string
	contentType string
	description string
	schema      *schema
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

func addErrorResponse(route *route, status int, description string) {
	if previous := route.responses[status].description; previous != "" && previous != description {
		description = previous + "; " + description
	}
	route.responses[status] = response{
		description: description,
		schema:      &schema{ref: "#/components/schemas/ErrorResponse"},
	}
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
		case "writeVerificationError":
			g.describeVerificationErrors(route)
		case "WriteHTTPSecurityError":
			g.ensureErrorResponse()
			for _, status := range []int{400, 401, 403, 429, 500, 503} {
				addErrorResponse(route, status, http.StatusText(status))
			}
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
