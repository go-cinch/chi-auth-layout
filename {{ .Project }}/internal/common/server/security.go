package server

import (
	"log/slog"
	"net/http"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func WriteHTTPSecurityError(w http.ResponseWriter, r *http.Request, err error) {
	SetMFAErrorHeaders(w, err)
	switch apperror.Code(err) {
	case "AUTH_STEP_UP_REQUIRED", "AUTH_MFA_ENROLLMENT_REQUIRED":
		WriteError(w, r, http.StatusForbidden, err)
	case "AUTH_MFA_INVALID":
		WriteError(w, r, http.StatusBadRequest, err)
	case "AUTH_MFA_RATE_LIMITED":
		WriteError(w, r, http.StatusTooManyRequests, err)
	case "AUTH_UNAUTHORIZED":
		WriteError(w, r, http.StatusUnauthorized, err)
	case "AUTH_MFA_UNAVAILABLE":
		WriteError(w, r, http.StatusServiceUnavailable, err)
	default:
		slog.ErrorContext(r.Context(), "http security check failed: "+err.Error(), "operation", "http_security_check", "outcome", "failure", "error", err)
		WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
	}
}

func SetMFAErrorHeaders(w http.ResponseWriter, err error) {
	switch apperror.Code(err) {
	case "AUTH_STEP_UP_REQUIRED", "AUTH_MFA_INVALID", "AUTH_MFA_RATE_LIMITED", "AUTH_MFA_ENROLLMENT_REQUIRED":
		w.Header().Set(authn.MFARequiredHeader, "true")
		w.Header().Set("Cache-Control", "no-store")
		if apperror.Code(err) == "AUTH_MFA_ENROLLMENT_REQUIRED" {
			w.Header().Set(authn.MFAEnrollmentRequiredHeader, "true")
		}
	}
}
