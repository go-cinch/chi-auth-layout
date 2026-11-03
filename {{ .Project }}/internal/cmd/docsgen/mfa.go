package main

import (
	"fmt"
	"net/http"
	"strings"
)

// writeMFAError maps errors shared by several handlers. Document only errors
// reachable from each operation; middleware failures are added separately.
func (g *generator) describeMFAErrors(r *route) {
	if r.tag != "auth" || r.handler == nil || r.handler.Name == nil {
		return
	}
	var statuses []int
	switch r.handler.Name.Name {
	case "login":
		statuses = []int{400, 401, 403, 429, 500, 503}
	case "mfaStatus":
		statuses = []int{401, 500, 503}
	case "mfaChallenge":
		statuses = []int{500, 503}
	case "mfaSetup", "mfaConfirm":
		statuses = []int{400, 401, 409, 429, 500, 503}
	case "mfaReset":
		statuses = []int{400, 401, 403, 404, 500, 503}
	}
	g.ensureErrorResponse()
	for _, status := range statuses {
		addErrorResponse(r, status, http.StatusText(status))
	}
}

func (g *generator) describeMFAHeaders(r *route) {
	handler := ""
	if r.handler != nil && r.handler.Name != nil {
		handler = r.handler.Name.Name
	}
	login := handler == "login" && r.tag == "auth"
	gateway := handler == "checkPermission" && r.tag == "auth"
	enrollmentVerification := r.tag == "auth" && (handler == "mfaSetup" || handler == "mfaConfirm")
	if handler == "mfaTest" && r.tag == "auth" {
		result := r.responses[http.StatusOK]
		result.description = "Authenticator code verified and atomically consumed. Every attempt uses the shared account and credential-version rate limit. Does not create, read, refresh, extend, or clear sensitive-operation authorization. The same TOTP step cannot be reused for testing, login, or sensitive operations."
		r.responses[http.StatusOK] = result
	}
	sessionResponse := r.tag == "auth" && (login || handler == "refresh" || handler == "resetPassword" || handler == "mfaConfirm")
	if !r.authenticated && !sessionResponse {
		return
	}
	sensitive := r.method == http.MethodDelete
	if r.tag == "auth" {
		sensitive = (handler == "mfaReset" && r.method == http.MethodPost) ||
			(handler == "changePassword" && r.method == http.MethodPatch)
	} else if r.method == http.MethodPost || r.method == http.MethodPut || r.method == http.MethodPatch {
		for _, root := range []string{"/user", "/user-group", "/role", "/action", "/dictionary"} {
			sensitive = sensitive || r.path == root || strings.HasPrefix(r.path, root+"/")
		}
	}
	if sensitive || login || gateway {
		r.parameters = append(r.parameters, parameter{in: "header", name: "X-MFA-Code", description: "Six-digit TOTP only; retry this same endpoint when X-MFA-Required is true", schema: schema{typeName: "string"}})
		g.ensureErrorResponse()
		for _, status := range []int{400, 403, 429, 503} {
			if _, ok := r.responses[status]; !ok {
				addErrorResponse(r, status, http.StatusText(status))
			}
		}
	}
	if login {
		if r.requestBody != nil {
			r.requestBody.description = "Without X-MFA-Ticket, send challenge_id and credential for encrypted password login. With X-MFA-Ticket, send only {} and exactly one of X-MFA-Code or X-MFA-Recovery-Code. JSON null is rejected."
		}
		r.parameters = append(r.parameters,
			parameter{in: "header", name: "X-MFA-Ticket", description: fmt.Sprintf("One-use password-verified continuation returned by this login endpoint; send with exactly one of X-MFA-Code or X-MFA-Recovery-Code and an empty JSON object. Ticket lifetime: %s; X-MFA-Expires-At is the absolute Unix-millisecond deadline.", g.loginTicketTTL), schema: schema{typeName: "string"}},
			parameter{in: "header", name: "X-MFA-Recovery-Code", description: "Unused recovery code accepted only with X-MFA-Ticket and an empty JSON object after password verification; do not combine with X-MFA-Code. Recovery invalidates the previous factor, all recovery codes and existing sessions, then returns an enrollment-restricted session with X-MFA-Enrollment-Required: true. It does not complete login or authorize sensitive operations; bind a new authenticator before continuing", schema: schema{typeName: "string"}},
		)
	}
	// These Auth endpoints remain available before enrollment and do not
	// request sensitive authorization. Permission failures have no MFA headers.
	enrollmentRejected := r.authenticated
	if r.tag == "auth" {
		switch handler {
		case "mfaStatus", "mfaChallenge", "mfaSetup", "mfaConfirm", "logout":
			enrollmentRejected = false
		}
	}
	for status, result := range r.responses {
		if status == http.StatusForbidden && (login || enrollmentRejected) {
			result.headers = append(result.headers, "X-MFA-Required", "X-MFA-Enrollment-Required")
		}
		if (sensitive || login || gateway || enrollmentVerification) && (status == http.StatusBadRequest || status == http.StatusTooManyRequests) {
			result.headers = append(result.headers, "X-MFA-Required")
		}
		if login && status >= http.StatusBadRequest {
			result.headers = append(result.headers, "X-MFA-Ticket", "X-MFA-Expires-At")
		}
		if status == http.StatusOK && r.tag == "auth" && (login || handler == "refresh") {
			result.headers = append(result.headers, "X-MFA-Required", "X-MFA-Enrollment-Required")
		}
		r.responses[status] = result
	}
}
