{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
	"net/http"
	"net/http/pprof"
)

func NewProfilerHandler() http.Handler {
	r := http.NewServeMux()
	r.HandleFunc("/debug/pprof/", pprof.Index)
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	r.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return r
}
{{ else -}}
package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func NewProfilerHandler() http.Handler {
	r := chi.NewRouter()
	r.Mount("/debug", chimiddleware.Profiler())
	return r
}
{{ end -}}
