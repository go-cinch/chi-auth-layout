package main

import "net/http"

func (g *generator) describePasswordResetHeaders(r *route) {
	handler := ""
	if r.handler != nil && r.handler.Name != nil {
		handler = r.handler.Name.Name
	}
	sessionResponse := r.tag == "auth" && (handler == "login" || handler == "refresh" || handler == "mfaConfirm")
	for status, result := range r.responses {
		if (status == http.StatusForbidden && r.authenticated) || (status == http.StatusOK && sessionResponse) {
			result.headers = append(result.headers, "X-Password-Reset-Required")
			r.responses[status] = result
		}
	}
}
