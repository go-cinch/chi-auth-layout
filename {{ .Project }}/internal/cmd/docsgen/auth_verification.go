package main

import (
	"fmt"
	"net/http"
	"strings"
)

// writeVerificationError maps errors shared by several handlers. Document only errors
// reachable from each operation; middleware failures are added separately.
func (g *generator) describeVerificationErrors(r *route) {
	if r.tag != "auth" || r.handler == nil || r.handler.Name == nil {
		return
	}
	var statuses []int
	switch r.handler.Name.Name {
	case "login":
		statuses = []int{400, 401, 403, 429, 500, 503}
	case "totpStatus":
		statuses = []int{401, 500, 503}
	case "totpChallenge":
		statuses = []int{500, 503}
	case "totpSetup", "totpConfirm":
		statuses = []int{400, 401, 409, 429, 500, 503}
	case "totpReset":
		statuses = []int{400, 401, 403, 404, 500, 503}
	}
	g.ensureErrorResponse()
	for _, status := range statuses {
		addErrorResponse(r, status, http.StatusText(status))
	}
}

func (g *generator) describeVerificationHeaders(r *route) {
	handler := ""
	if r.handler != nil && r.handler.Name != nil {
		handler = r.handler.Name.Name
	}
	login := handler == "login" && r.tag == "auth"
	gateway := handler == "checkPermission" && r.tag == "auth"
	enrollmentVerification := r.tag == "auth" && (handler == "totpSetup" || handler == "totpConfirm")
	if handler == "totpTest" && r.tag == "auth" {
		result := r.responses[http.StatusOK]
		result.description = "Authenticator code verified and atomically consumed. Every attempt uses the shared account and credential-version rate limit. Does not create, read, refresh, extend, or clear sensitive-operation authorization. The same TOTP step cannot be reused for testing, login, or sensitive operations."
		r.responses[http.StatusOK] = result
	}
	sessionResponse := r.tag == "auth" && (login || handler == "refresh" || handler == "resetPassword" || handler == "totpConfirm")
	if !r.authenticated && !sessionResponse {
		return
	}
	sensitive := r.method == http.MethodDelete
	if r.tag == "auth" {
		sensitive = ((handler == "totpReset" || handler == "totpSetup" || handler == "totpConfirm" || handler == "bindEmail") && r.method == http.MethodPost) ||
			(handler == "changePassword" && r.method == http.MethodPatch)
	} else if r.method == http.MethodPost || r.method == http.MethodPut || r.method == http.MethodPatch {
		for _, root := range []string{"/mail-template", "/user", "/user-group", "/role", "/action", "/dictionary"} {
			sensitive = sensitive || r.path == root || strings.HasPrefix(r.path, root+"/")
		}
	}
	if sensitive || login || gateway {
		r.parameters = append(r.parameters, parameter{in: "header", name: "X-TOTP-Code", description: "Six-digit TOTP only; submit directly on this endpoint", schema: schema{typeName: "string"}})
		if !login {
			r.parameters = append(r.parameters,
				parameter{in: "header", name: "X-Step-Up-Challenge", description: "One-use sensitive challenge ID for email verification", schema: schema{typeName: "string"}},
				parameter{in: "header", name: "X-Step-Up-Credential", description: "Compact sensitive+jwe containing challenge_id, method=email, verification_code, verification_id and browser_uuid. Omit X-TOTP-Code when using email.", schema: schema{typeName: "string"}})
		}
		g.ensureErrorResponse()
		for _, status := range []int{400, 403, 429, 503} {
			if _, ok := r.responses[status]; !ok {
				addErrorResponse(r, status, http.StatusText(status))
			}
		}
	}
	if login {
		if r.requestBody != nil {
			empty := &schema{typeName: "object", closedObject: true}
			encrypted := &schema{ref: "#/components/schemas/EncryptedLoginInput"}
			r.requestBody = &schema{anyOf: []*schema{empty, encrypted}}
			r.requestBody.description = "Without X-MFA-Ticket, send challenge_id and credential for encrypted password or email-code login. With X-MFA-Ticket, send only {} for TOTP or password-verified recovery. Email-code login recovery must include a fresh login JWE containing the current password. Use exactly one of X-TOTP-Code or X-Recovery-Code. JSON null is rejected."
		}
		r.parameters = append(r.parameters,
			parameter{in: "header", name: "X-MFA-Ticket", description: fmt.Sprintf("One-use first-factor continuation returned by this login endpoint; send with exactly one of X-TOTP-Code or X-Recovery-Code and an empty JSON object. Ticket lifetime: %s; X-MFA-Expires-At is the absolute Unix-millisecond deadline.", g.loginTicketTTL), schema: schema{typeName: "string"}},
			parameter{in: "header", name: "X-Recovery-Code", description: "Unused recovery code accepted only with X-MFA-Ticket and an empty JSON object after password verification; do not combine with X-TOTP-Code. Recovery invalidates the previous factor, all recovery codes and existing sessions, then returns an enrollment-restricted session with onboarding requirements available from GET /status. It does not complete login or authorize sensitive operations; bind a new authenticator before continuing", schema: schema{typeName: "string"}},
		)
	}
	// Enrollment endpoints remain available before binding, but require an
	// existing mailbox proof when available. Permission failures have no Verification headers.
	enrollmentRejected := r.authenticated
	if r.tag == "auth" {
		switch handler {
		case "totpStatus", "totpChallenge", "totpSetup", "totpConfirm", "logout":
			enrollmentRejected = false
		}
	}
	for status, result := range r.responses {
		if status == http.StatusForbidden && (sensitive || login || enrollmentRejected) {
			result.headers = append(result.headers, "X-Verification-Required")
		}
		if (sensitive || login || gateway || enrollmentVerification) && (status == http.StatusBadRequest || status == http.StatusTooManyRequests) {
			result.headers = append(result.headers, "X-Verification-Required")
		}
		if login && status >= http.StatusBadRequest {
			result.headers = append(result.headers, "X-MFA-Ticket", "X-MFA-Expires-At")
		}

		r.responses[status] = result
	}
}
