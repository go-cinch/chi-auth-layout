package main

import (
	"net/http"
	"strings"
)

func (g *generator) describeMailTemplateRoute(r *route) {
	if !strings.HasPrefix(r.path, "/mail-template") {
		return
	}
	if strings.HasSuffix(r.path, "/test-send") {
		g.ensureErrorResponse()
		addErrorResponse(r, http.StatusForbidden, "Bind a verified email before test delivery (MAIL_TEMPLATE_EMAIL_REQUIRED), or complete sensitive verification")
		addErrorResponse(r, http.StatusServiceUnavailable, "Email delivery is disabled or in debug mode; no email was sent")
		if r.requestBody != nil {
			r.requestBody.description = "Send the supplied current editor content (subject,html,text) in locale zh-CN or en-US to the authenticated account’s currently verified email. The request cannot specify a recipient. values must explicitly contain every referenced parameter; preview samples are not runtime defaults. Requires template edit permission, sensitive verification and an x-idempotent key. Does not save or publish. Success means the signed multipart text/HTML snapshot was durably queued; SMTP delivery runs asynchronously."
		}
		return
	}
	if r.path == "/mail-template" && r.method == http.MethodGet {
		fallback := int64(1)
		for _, name := range []string{"p", "s"} {
			limit := g.limits.MaxP
			if name == "s" {
				limit = g.limits.MaxS
			}
			r.parameters = append(r.parameters, parameter{name: name, in: "query", schema: schema{typeName: "integer", format: "int32", paginationKey: name, paginationMax: int64(limit), defaultValue: &fallback}})
		}
		r.parameters = append(r.parameters, parameter{name: "q", in: "query", description: "Case-insensitive literal substring match on name OR template_id", schema: schema{typeName: "string"}}, parameter{name: "category", in: "query", description: "verification, confirmation, security or notification", schema: schema{typeName: "string"}})
		response := r.responses[http.StatusOK]
		response.description = "Metadata-only items,t,p,s. Content is loaded by the item endpoint; list queries never load template bodies. Omitted p/s select 1; invalid formats or duplicate parameters return 400."
		r.responses[http.StatusOK] = response
	}
	if r.method == http.MethodPost || r.method == http.MethodPatch {
		g.ensureErrorResponse()
		addErrorResponse(r, http.StatusConflict, "Template ID already exists, the optimistic revision is stale, or there is no unpublished draft")
	}
	if r.requestBody != nil {
		r.requestBody.description = "Template content uses zh-CN and en-US keys. Each language contains subject,html,text,preview_values. Preview values are examples, never runtime defaults. Draft writes preserve incomplete content; publishing requires both subjects and HTML bodies. PATCH may rename template_id; omission preserves it, duplicate IDs return 409. PATCH with revision and discard_draft:true restores the current published name/category/content and clears has_draft under the same row lock; reject other editable fields in that request, unpublished templates (400), and stale revisions (409). The current template_id, numeric row ID and publication history stay unchanged. Publication only stores a version and never sends email."
	}
}
