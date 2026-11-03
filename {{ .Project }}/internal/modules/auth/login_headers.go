package auth

import (
	"net/http"

	"{{ .Computed.module_name_final }}/internal/common/authn"
)

// Report the restrictions carried by every newly issued or refreshed session.
func writeLoginHeaders(w http.ResponseWriter, result *LoginResult) {
	if result == nil {
		return
	}
	if result.PasswordResetRequired {
		w.Header().Set(authn.PasswordResetRequiredHeader, "true")
	}
	if result.MFAEnrollmentRequired {
		w.Header().Set(authn.MFARequiredHeader, "true")
		w.Header().Set(authn.MFAEnrollmentRequiredHeader, "true")
	}
}
