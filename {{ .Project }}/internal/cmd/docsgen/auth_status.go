package main

import (
	"go/ast"
	"net/http"
)

func (g *generator) describeStatusRoute(r *route) {
	if r.tag != "auth" || r.handler == nil {
		return
	}
	handler := r.handler.Name.Name
	if handler == "systemProperties" || handler == "info" {
		r.parameters = append(r.parameters, parameter{name: "ui", in: "query", description: "Optional UI client selector. Currently vben_pc. /pub/opt adds dictionary defaults; /info requires f=ui_preferences and returns defaults merged with only this account/client overrides. Language and timezone stay in metadata.language and metadata.timezone. Unknown, empty or duplicate client selectors return 400.", schema: schema{typeName: "string", example: "vben_pc"}})
	}
	if handler == "systemProperties" {
		response := r.responses[http.StatusOK]
		response.description = "Public deployment options and developer-published language choices. email_enabled and totp_enabled are independent booleans. default_locale and locales (tag, name, enabled) come from I18N_CATALOG; only enabled languages are returned. No user data or translations. With ui=vben_pc, ui_preferences.vben_pc contains UI_PREFERENCES_VBEN_PC defaults. Response is no-store."
		r.responses[http.StatusOK] = response
	}
	if handler == "updateInfo" && r.requestBody != nil {
		r.requestBody.description = "Supply exactly one of metadata.ui_preferences, metadata.language, metadata.timezone or a true onboarding skip flag. ui_preferences accepts only {vben_pc: sparse_patch}; nested null restores a field/group, vben_pc:null resets this client. Values equal to current dictionary defaults remove their stored overrides. PATCH returns effective ui_preferences for a preference write. Arrays replace atomically. Arbitrary metadata, language and timezone nested inside UI preferences are rejected. metadata.language accepts one enabled tag from GET /pub/opt. metadata.timezone accepts a non-empty IANA timezone name, such as Asia/Tokyo or America/New_York; UTC is supported, Local and numeric offsets are rejected. Do not combine language and skip flags. Language updates affect only the authenticated user's language and preserve all other metadata. Restricted onboarding sessions cannot update language or timezone."
	}
	legacy := map[string]string{
		"totpStatus":            "GET /info?f=totp and GET /status?purpose=totp_enrollment&f=requirements",
		"totpChallenge":         "POST /challenge with purpose=totp_setup",
		"loginVerification":    "GET /pub/status?purpose=login&f=requirements, then POST /pub/captcha to issue a required challenge",
		"loginEmailCodeStatus": "GET /pub/status?purpose=login&f=email_code",
		"bindEmailCodeStatus":  "GET /status?purpose=email_bind&f=email_code",
		"unlinkStatus":         "GET /status?purpose=email_unbind&f=methods,email_code",
	}
	if replacement, ok := legacy[handler]; ok {
		response := r.responses[http.StatusOK]
		response.description = "Compatibility endpoint for older clients. New clients use " + replacement + ". " + response.description
		r.responses[http.StatusOK] = response
		return
	}
	if handler == "refreshLoginCaptcha" && r.requestBody != nil {
		r.requestBody.description = "Send username to explicitly issue a required login point CAPTCHA (returns captcha_required and optional captcha), or captcha_id to refresh an existing challenge (returns the challenge). Do not combine the two. GET /pub/status never issues a challenge."
	}
	if handler != "status" && handler != "publicStatus" {
		return
	}
	purposes := "onboarding (requirements), totp_enrollment (requirements), email_bind (email_code), email_unbind (methods,email_code), sensitive (methods,grant,email_code), totp_recovery (methods,email_code)"
	if handler == "publicStatus" {
		r.parameters = append(r.parameters, parameter{name: "X-MFA-Ticket", in: "header", description: "Required only for login_totp_recovery. Unexpired login MFA ticket from successful first-factor verification; read without consumption or lifetime extension. Never send it in the URL.", schema: schema{typeName: "string"}})
		purposes = "login (requirements,email_code), login_totp_recovery (methods,email_code, requires a valid X-MFA-Ticket). Account binding is only disclosed for the verified ticket owner; authenticated purposes are rejected"
	}
	r.parameters = append(r.parameters,
		parameter{name: "purpose", in: "query", required: true, description: "Verification flow: " + purposes, schema: schema{typeName: "string"}},
		parameter{name: "f", in: "query", required: true, description: "Comma-separated response groups allowed for this purpose. Missing, empty, unknown, duplicate selectors and repeated parameters return 400. Only selected dependencies are loaded. requirements never issues a CAPTCHA; explicitly POST /pub/captcha with username to issue one.", schema: schema{typeName: "string"}},
		parameter{name: "browser_uuid", in: "query", description: "Required only when selecting email_code; source IP also contributes to the send cooldown.", schema: schema{typeName: "string", format: "uuid"}},
		parameter{name: "username", in: "query", description: "Required for login requirements. Omit for an email_code-only browser/IP cooldown before entering an identifier. Unknown accounts retain the existing non-disclosing response behavior.", schema: schema{typeName: "string"}},
		parameter{name: "email", in: "query", description: "Optional for email_bind; omit for the browser/IP cooldown before entering an address. When present, reads pending verification for this unverified target, not the account mailbox.", schema: schema{typeName: "string"}},
	)
	r.responses[http.StatusOK] = response{
		description: "Read-only verification-flow snapshot. purpose and server_time (Unix milliseconds) are always returned; unselected groups are omitted, selected false/zero values are retained. Onboarding requirements include totp_skip_allowed, password_reset_skip_allowed and effective totp_onboarding_hidden, password_reset_onboarding_hidden, email_onboarding_hidden flags. Hidden flags only apply to skippable steps; Vben acknowledges hidden Verification/password steps before navigation and suppresses the optional email prompt; submit exactly one true metadata flag (totp_enrollment_skipped or password_reset_skipped) to PATCH /info to skip an optional step, then reread onboarding. Required policy rejects skipping; recovery re-enrollment cannot be skipped. For sensitive, totp_recovery and login_totp_recovery, email_code returns only the browser/IP/verified-recipient cooldown; it never issues mail or consumes Verification attempts. email_code.completed indicates an unlink flow has no remaining binding. Responses are no-store. /pub/opt remains the independent deployment-options endpoint. Enrollment-restricted sessions may also read sensitive methods and grants to verify an already bound mailbox before enrollment. Password-reset-only sessions remain limited to onboarding or totp_enrollment requirements.",
		schema:      g.typeSchema(r.pkg, ast.NewIdent("StatusResult")),
	}
	for _, code := range []int{400, 401, 403, 500, 503} {
		addErrorResponse(r, code, http.StatusText(code))
	}
}

func (g *generator) describeOnboardingResponse(r *route) {
	if r.tag != "auth" || r.handler == nil {
		return
	}
	switch r.handler.Name.Name {
	case "login", "refresh", "totpConfirm", "resetPassword", "bindEmail", "confirmEmailReset", "confirmAccountReset":
		result := r.responses[http.StatusOK]
		result.description += " Accept the replacement tokens, then GET /status?purpose=onboarding&f=requirements before business navigation. Session restrictions are not returned in response headers."
		r.responses[http.StatusOK] = result
	}
}
