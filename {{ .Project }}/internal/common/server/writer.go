package server

import (
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func wrapResponseWriter(w http.ResponseWriter, protoMajor int) chimiddleware.WrapResponseWriter {
	if writer, ok := w.(chimiddleware.WrapResponseWriter); ok {
		return writer
	}
	return chimiddleware.NewWrapResponseWriter(w, protoMajor)
}

func responseWritten(w chimiddleware.WrapResponseWriter) bool {
	return w.Status() != 0
}
