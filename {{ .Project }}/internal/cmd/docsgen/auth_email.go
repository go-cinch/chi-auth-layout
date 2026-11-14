package main

import "net/http"

func (g *generator) describeEmailRoute(r *route) {
	if r.tag != "auth" || r.handler == nil {
		return
	}
	handler := r.handler.Name.Name
	switch handler {
	case "sendResetCode", "confirmEmailReset":
		g.ensureErrorResponse()
		r.parameters = append(r.parameters, parameter{in: "header", name: "X-MFA-Ticket", required: true, description: "Unexpired login MFA ticket", schema: schema{typeName: "string"}})
		r.requestBody = &schema{ref: "#/components/schemas/EncryptedLoginInput"}
		if handler == "sendResetCode" {
			r.requestBody.description = "One-use login JWE containing the current password, username and browser_uuid. Sends a reset-only code to the ticket owner's verified email."
			r.responses[200] = response{description: "Reset code queued", schema: &schema{ref: "#/components/schemas/EmailCodeStatus"}}
		} else {
			r.requestBody.description = "One-use login JWE with method=email_code, username, browser_uuid, verification_id and the reset-only verification_code."
			r.responses[200] = response{description: "Enrollment-restricted replacement session", schema: &schema{ref: "#/components/schemas/LoginResult"}}
		}
		for _, code := range []int{400, 401, 429, 500, 503} {
			addErrorResponse(r, code, http.StatusText(code))
		}
		return
	case "unlinkStatus":
		g.ensureErrorResponse()
		addErrorResponse(r, 500, "Internal server error")
		for i := range r.parameters {
			if r.parameters[i].name == "browser_uuid" {
				r.parameters[i].required = true
			}
		}
		addErrorResponse(r, 503, "Email authentication service unavailable")
		return
	case "requestUnlink", "confirmUnlink", "verifyUnlink":
		addErrorResponse(r, 500, "Internal server error")
		g.ensureErrorResponse()
		addErrorResponse(r, 409, "The link or binding has changed, or this method is unavailable")
		addErrorResponse(r, 503, "Email or verification service unavailable")
		if handler != "confirmUnlink" {
			addErrorResponse(r, 429, "The resend or verification limit is active")
		}
		if r.requestBody != nil {
			switch handler {
			case "requestUnlink":
				r.requestBody.description = "Submit the browser UUID for the send budget. The authenticated account determines the recipient."
			case "verifyUnlink":
				r.parameters = append(r.parameters, parameter{in: "header", name: "X-TOTP-Code", description: "Six-digit TOTP only; submit with an empty JSON object", schema: schema{typeName: "string"}})
				r.requestBody.description = "For authenticator verification send {} with X-TOTP-Code. For password verification fetch POST /challenge with purpose=email_unbind and submit challenge_id and credential containing the encrypted password."
			case "confirmUnlink":
				r.requestBody.description = "Submit the one-use token from the email URL fragment. Reopening a completed link returns already_confirmed without sending another notice."
			}
		}
		return
	}
	status := handler == "loginEmailCodeStatus" || handler == "bindEmailCodeStatus"
	send := handler == "sendSensitiveCode" || handler == "sendLoginEmailCode" || handler == "sendBindEmailCode"
	bind := handler == "bindEmail"
	if !status && !send && !bind {
		return
	}
	g.ensureErrorResponse()
	if status {
		identifier := "username"
		if handler == "bindEmailCodeStatus" {
			identifier = "email"
		}
		r.parameters = append(r.parameters,
			parameter{in: "query", name: "browser_uuid", required: true, description: "Persistent browser UUID v4. The server also applies the source IP cooldown.", schema: schema{typeName: "string", format: "uuid"}},
			parameter{in: "query", name: identifier, required: true, schema: schema{typeName: "string"}},
		)
		r.responses[http.StatusOK] = response{description: "Server countdown and this browser's pending verification context; never returns a verification code or resolved recipient.", schema: &schema{ref: "#/components/schemas/EmailCodeStatus"}}
	}
	for _, code := range []int{400, 500, 503} {
		addErrorResponse(r, code, http.StatusText(code))
	}
	if send {
		r.responses[http.StatusOK] = response{description: "Initial send and resend return as soon as the OTP is queued. SMTP runs asynchronously with up to three retries. Unknown syntactically valid login email addresses receive the same acceptance response. server_time and retry_at are Unix milliseconds; browser UUID, IP and recipient budgets are checked atomically.", schema: &schema{ref: "#/components/schemas/EmailCodeStatus"}}
		addErrorResponse(r, 429, "Send cooldown is active. The body includes server_time and retry_at; Retry-After reports remaining seconds.")
		result := r.responses[429]
		result.headers = []string{"Retry-After"}
		r.responses[429] = result
	}
	if handler == "sendLoginEmailCode" && r.requestBody != nil {
		r.requestBody.description = "username must contain a valid email address with a dotted domain, not an account username. Invalid addresses return 400 AUTH_EMAIL_ADDRESS_INVALID before verification or queueing. The server trims surrounding whitespace and resolves the verified email alias."
	}
	if bind || handler == "sendBindEmailCode" {
		addErrorResponse(r, 409, "Email is already bound or unavailable")
	}
	if bind && r.requestBody != nil {
		r.requestBody.description = "Fetch POST /challenge with purpose=email_bind. Submit only challenge_id and credential (email-bind+jwe); email, browser_uuid, verification_id and verification_code belong inside the one-use encrypted payload. Success returns replacement tokens after committing the unique email binding."
	}
}
