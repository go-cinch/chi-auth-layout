package server

import (
	"encoding/json"
	"io"
	"net/http"

	"{{ .Computed.module_name_final }}/internal/common/apperror"
)

// DecodeJSON reads one bounded JSON value and rejects unknown fields and
// trailing data. Callers own response headers and business-field validation.
func DecodeJSON(w http.ResponseWriter, r *http.Request, input any, category BodyLimit) error {
	return decodeJSON(w, r, input, requestBodyLimit(r, category))
}

func decodeJSON(w http.ResponseWriter, r *http.Request, input any, maxBytes int64) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		return apperror.InvalidBody
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return apperror.InvalidBody
	}
	return nil
}
