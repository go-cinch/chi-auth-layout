package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/i18n"
)

type ErrorResponse struct {
	ErrorCode string `json:"error_code"`
	Msg       string `json:"msg"`
}

func WriteJSON(w http.ResponseWriter, status int, body any) {
	data, err := json.Marshal(body)
	if err != nil {
		slog.Error("encode json response failed: " + err.Error())
		status = http.StatusInternalServerError
		data, _ = json.Marshal(ErrorResponse{ErrorCode: apperror.Code(apperror.Internal), Msg: i18n.Translate(w.Header().Get("Content-Language"), apperror.Code(apperror.Internal), apperror.Internal.Error())})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		slog.Debug("write json response failed: " + err.Error())
	}
}

func WriteOK(w http.ResponseWriter, body ...any) {
	if len(body) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}
	WriteJSON(w, http.StatusOK, body[0])
}

func ErrorMessage(r *http.Request, err error) string {
	public := apperror.Public(err)
	locale := i18n.Locale(r.Context(), r.Header.Get("Accept-Language"))
	return i18n.Translate(locale, apperror.Code(public), public.Error())
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, err error) {
	w.Header().Set("Content-Language", i18n.Locale(r.Context(), r.Header.Get("Accept-Language")))
	WriteJSON(w, status, ErrorResponse{ErrorCode: apperror.Code(err), Msg: ErrorMessage(r, err)})
}

func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
