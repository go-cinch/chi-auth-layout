package msg

import (
 "encoding/json"
 "errors"
 "io"
 "log/slog"
 "net/http"
 "net/url"
 "strconv"

{{- if eq .Computed.http_router_final "gin" }}
 "github.com/gin-gonic/gin"
{{- else }}
 "github.com/go-chi/chi/v5"
{{- end }}
 "{{ .Computed.module_name_final }}/internal/common/apperror"
 "{{ .Computed.module_name_final }}/internal/common/authn"
 "{{ .Computed.module_name_final }}/internal/common/server"
 "{{ .Computed.module_name_final }}/internal/modules"
)

var _ modules.HTTPModule = (*Module)(nil)
var _ modules.IdempotentHTTPModule = (*Module)(nil)

func (*Module) Name() string { return "/msg" }

func (*Module) Idempotent() bool { return true }

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) HTTP(r *gin.RouterGroup) {
 r.GET("/inbox", m.listInbox)
 r.GET("/inbox/:id", m.getInbox)
 r.PATCH("/inbox/:id", m.read)
 r.DELETE("/inbox/:id", m.deleteInbox)
 r.POST("/inbox/read-all", m.readAll)
 r.GET("/unread-count", m.unreadCount)
 r.POST("", m.create)
 r.GET("/sent", m.listSent)
 r.GET("/sent/:id", m.getSent)
 r.DELETE("/sent/:id", m.deleteSent)
 r.GET("/recipient-option", m.recipientOptions)
}
{{ else -}}
func (m *Module) HTTP() http.Handler {
 r := chi.NewRouter()
 r.Get("/inbox", m.listInbox)
 r.Get("/inbox/{id}", m.getInbox)
 r.Patch("/inbox/{id}", m.read)
 r.Delete("/inbox/{id}", m.deleteInbox)
 r.Post("/inbox/read-all", m.readAll)
 r.Get("/unread-count", m.unreadCount)
 r.Post("/", m.create)
 r.Get("/sent", m.listSent)
 r.Get("/sent/{id}", m.getSent)
 r.Delete("/sent/{id}", m.deleteSent)
 r.Get("/recipient-option", m.recipientOptions)
 return r
}
{{ end }}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) create(c *gin.Context) {
 w,r := c.Writer,c.Request
{{ else -}}
func (m *Module) create(w http.ResponseWriter,r *http.Request) {
{{ end }}
 identity, ok := authn.FromContext(r.Context())
 if !ok { server.WriteError(w,r,http.StatusUnauthorized,apperror.Unauthorized); return }
 var input CreateInput
 decoder := json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20))
 decoder.DisallowUnknownFields()
 if err := decoder.Decode(&input); err != nil { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidBody); return }
 if err := decoder.Decode(new(any)); err != io.EOF { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidBody); return }
 value, err := m.Create(r.Context(),identity.UserID,input)
 if err != nil {
  switch {
  case errors.Is(err,ErrInvalid), errors.Is(err,ErrRecipient): server.WriteError(w,r,http.StatusBadRequest,err)
  case errors.Is(err,ErrNotFound): server.WriteError(w,r,http.StatusNotFound,err)
  default:
   slog.ErrorContext(r.Context(),r.Method+" "+r.URL.Path+" failed: "+err.Error())
   server.WriteError(w,r,http.StatusInternalServerError,apperror.Internal)
  }
  return
 }
 server.WriteOK(w,value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) getInbox(c *gin.Context) {
 w,r := c.Writer,c.Request
{{ else -}}
func (m *Module) getInbox(w http.ResponseWriter,r *http.Request) {
{{ end }}
 identity, ok := authn.FromContext(r.Context())
 if !ok { server.WriteError(w,r,http.StatusUnauthorized,apperror.Unauthorized); return }
{{ if eq .Computed.http_router_final "gin" -}}
 id, err := strconv.ParseInt(c.Param("id"),10,64)
{{ else -}}
 id, err := strconv.ParseInt(chi.URLParam(r,"id"),10,64)
{{ end }}
 if err != nil || id <= 0 { server.WriteError(w,r,http.StatusBadRequest,ErrInvalid); return }
 value, err := m.Get(r.Context(),identity.UserID,id)
 if err != nil {
  switch {
  case errors.Is(err,ErrInvalid), errors.Is(err,ErrRecipient): server.WriteError(w,r,http.StatusBadRequest,err)
  case errors.Is(err,ErrNotFound): server.WriteError(w,r,http.StatusNotFound,err)
  default:
   slog.ErrorContext(r.Context(),r.Method+" "+r.URL.Path+" failed: "+err.Error())
   server.WriteError(w,r,http.StatusInternalServerError,apperror.Internal)
  }
  return
 }
 server.WriteOK(w,value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) getSent(c *gin.Context) {
 w,r := c.Writer,c.Request
{{ else -}}
func (m *Module) getSent(w http.ResponseWriter,r *http.Request) {
{{ end }}
{{ if eq .Computed.http_router_final "gin" -}}
 id, err := strconv.ParseInt(c.Param("id"),10,64)
{{ else -}}
 id, err := strconv.ParseInt(chi.URLParam(r,"id"),10,64)
{{ end }}
 if err != nil || id <= 0 { server.WriteError(w,r,http.StatusBadRequest,ErrInvalid); return }
 value, err := m.GetSent(r.Context(),id)
 if err != nil {
  switch {
  case errors.Is(err,ErrInvalid), errors.Is(err,ErrRecipient): server.WriteError(w,r,http.StatusBadRequest,err)
  case errors.Is(err,ErrNotFound): server.WriteError(w,r,http.StatusNotFound,err)
  default:
   slog.ErrorContext(r.Context(),r.Method+" "+r.URL.Path+" failed: "+err.Error())
   server.WriteError(w,r,http.StatusInternalServerError,apperror.Internal)
  }
  return
 }
 server.WriteOK(w,value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) read(c *gin.Context) {
 w,r := c.Writer,c.Request
{{ else -}}
func (m *Module) read(w http.ResponseWriter,r *http.Request) {
{{ end }}
 identity, ok := authn.FromContext(r.Context())
 if !ok { server.WriteError(w,r,http.StatusUnauthorized,apperror.Unauthorized); return }
{{ if eq .Computed.http_router_final "gin" -}}
 id, err := strconv.ParseInt(c.Param("id"),10,64)
{{ else -}}
 id, err := strconv.ParseInt(chi.URLParam(r,"id"),10,64)
{{ end }}
 if err != nil || id <= 0 { server.WriteError(w,r,http.StatusBadRequest,ErrInvalid); return }
 var input struct { Read *bool `json:"read"` }
 decoder := json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20))
 decoder.DisallowUnknownFields()
 if err := decoder.Decode(&input); err != nil { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidBody); return }
 if err := decoder.Decode(new(any)); err != io.EOF { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidBody); return }
 if input.Read == nil || !*input.Read { server.WriteError(w,r,http.StatusBadRequest,ErrInvalid); return }
 err = m.Read(r.Context(),identity.UserID,id)
 if err != nil {
  switch {
  case errors.Is(err,ErrInvalid), errors.Is(err,ErrRecipient): server.WriteError(w,r,http.StatusBadRequest,err)
  case errors.Is(err,ErrNotFound): server.WriteError(w,r,http.StatusNotFound,err)
  default:
   slog.ErrorContext(r.Context(),r.Method+" "+r.URL.Path+" failed: "+err.Error())
   server.WriteError(w,r,http.StatusInternalServerError,apperror.Internal)
  }
  return
 }
 server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) deleteInbox(c *gin.Context) {
 w,r := c.Writer,c.Request
{{ else -}}
func (m *Module) deleteInbox(w http.ResponseWriter,r *http.Request) {
{{ end }}
 identity, ok := authn.FromContext(r.Context())
 if !ok { server.WriteError(w,r,http.StatusUnauthorized,apperror.Unauthorized); return }
{{ if eq .Computed.http_router_final "gin" -}}
 id, err := strconv.ParseInt(c.Param("id"),10,64)
{{ else -}}
 id, err := strconv.ParseInt(chi.URLParam(r,"id"),10,64)
{{ end }}
 if err != nil || id <= 0 { server.WriteError(w,r,http.StatusBadRequest,ErrInvalid); return }
 err = m.Delete(r.Context(),identity.UserID,id)
 if err != nil {
  switch {
  case errors.Is(err,ErrInvalid), errors.Is(err,ErrRecipient): server.WriteError(w,r,http.StatusBadRequest,err)
  case errors.Is(err,ErrNotFound): server.WriteError(w,r,http.StatusNotFound,err)
  default:
   slog.ErrorContext(r.Context(),r.Method+" "+r.URL.Path+" failed: "+err.Error())
   server.WriteError(w,r,http.StatusInternalServerError,apperror.Internal)
  }
  return
 }
 server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) deleteSent(c *gin.Context) {
 w,r := c.Writer,c.Request
{{ else -}}
func (m *Module) deleteSent(w http.ResponseWriter,r *http.Request) {
{{ end }}
{{ if eq .Computed.http_router_final "gin" -}}
 id, err := strconv.ParseInt(c.Param("id"),10,64)
{{ else -}}
 id, err := strconv.ParseInt(chi.URLParam(r,"id"),10,64)
{{ end }}
 if err != nil || id <= 0 { server.WriteError(w,r,http.StatusBadRequest,ErrInvalid); return }
 err = m.DeleteSent(r.Context(),id)
 if err != nil {
  switch {
  case errors.Is(err,ErrInvalid), errors.Is(err,ErrRecipient): server.WriteError(w,r,http.StatusBadRequest,err)
  case errors.Is(err,ErrNotFound): server.WriteError(w,r,http.StatusNotFound,err)
  default:
   slog.ErrorContext(r.Context(),r.Method+" "+r.URL.Path+" failed: "+err.Error())
   server.WriteError(w,r,http.StatusInternalServerError,apperror.Internal)
  }
  return
 }
 server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) readAll(c *gin.Context) {
 w,r := c.Writer,c.Request
{{ else -}}
func (m *Module) readAll(w http.ResponseWriter,r *http.Request) {
{{ end }}
 identity, ok := authn.FromContext(r.Context())
 if !ok { server.WriteError(w,r,http.StatusUnauthorized,apperror.Unauthorized); return }
 err := m.ReadAll(r.Context(),identity.UserID)
 if err != nil {
  switch {
  case errors.Is(err,ErrInvalid), errors.Is(err,ErrRecipient): server.WriteError(w,r,http.StatusBadRequest,err)
  case errors.Is(err,ErrNotFound): server.WriteError(w,r,http.StatusNotFound,err)
  default:
   slog.ErrorContext(r.Context(),r.Method+" "+r.URL.Path+" failed: "+err.Error())
   server.WriteError(w,r,http.StatusInternalServerError,apperror.Internal)
  }
  return
 }
 server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) unreadCount(c *gin.Context) {
 w,r := c.Writer,c.Request
{{ else -}}
func (m *Module) unreadCount(w http.ResponseWriter,r *http.Request) {
{{ end }}
 identity, ok := authn.FromContext(r.Context())
 if !ok { server.WriteError(w,r,http.StatusUnauthorized,apperror.Unauthorized); return }
 count, err := m.UnreadCount(r.Context(),identity.UserID)
 if err != nil {
  switch {
  case errors.Is(err,ErrInvalid), errors.Is(err,ErrRecipient): server.WriteError(w,r,http.StatusBadRequest,err)
  case errors.Is(err,ErrNotFound): server.WriteError(w,r,http.StatusNotFound,err)
  default:
   slog.ErrorContext(r.Context(),r.Method+" "+r.URL.Path+" failed: "+err.Error())
   server.WriteError(w,r,http.StatusInternalServerError,apperror.Internal)
  }
  return
 }
 server.WriteOK(w,struct { Count int64 `json:"count"` }{Count:count})
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) recipientOptions(c *gin.Context) {
 w,r := c.Writer,c.Request
{{ else -}}
func (m *Module) recipientOptions(w http.ResponseWriter,r *http.Request) {
{{ end }}
 query, err := url.ParseQuery(r.URL.RawQuery)
 if err != nil || len(query["q"]) > 1 { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
 value, err := m.RecipientOptions(r.Context(),query.Get("q"))
 if err != nil {
  switch {
  case errors.Is(err,ErrInvalid), errors.Is(err,ErrRecipient): server.WriteError(w,r,http.StatusBadRequest,err)
  case errors.Is(err,ErrNotFound): server.WriteError(w,r,http.StatusNotFound,err)
  default:
   slog.ErrorContext(r.Context(),r.Method+" "+r.URL.Path+" failed: "+err.Error())
   server.WriteError(w,r,http.StatusInternalServerError,apperror.Internal)
  }
  return
 }
 server.WriteOK(w,value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) listInbox(c *gin.Context) {
 w,r := c.Writer,c.Request
{{ else -}}
func (m *Module) listInbox(w http.ResponseWriter,r *http.Request) {
{{ end }}
 identity, ok := authn.FromContext(r.Context())
 if !ok { server.WriteError(w,r,http.StatusUnauthorized,apperror.Unauthorized); return }
 query,err := url.ParseQuery(r.URL.RawQuery)
 if err != nil { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
 input := ListInput{Type:query.Get("type"),Scope:query.Get("scope")}
 for _,key := range []string{"type","scope"} { if values,provided := query[key]; provided && (len(values)!=1 || values[0]=="") { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return } }
 if values,provided := query["p"]; provided {
  if len(values)!=1 { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
  parsed,err := strconv.ParseInt(values[0],10,32)
  if err!=nil { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
  value:=int32(parsed);input.Page=&value
 }
 if values,provided := query["s"]; provided {
  if len(values)!=1 { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
  parsed,err := strconv.ParseInt(values[0],10,32)
  if err!=nil { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
  value:=int32(parsed);input.PageSize=&value
 }
 if values,provided := query["read"]; provided {
  if len(values)!=1 || (values[0]!="true" && values[0]!="false") { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
  value,_:=strconv.ParseBool(values[0]);input.Read=&value
 }

 value, err := m.List(r.Context(),identity.UserID,input)
 if err != nil {
  switch {
  case errors.Is(err,ErrInvalid), errors.Is(err,ErrRecipient): server.WriteError(w,r,http.StatusBadRequest,err)
  case errors.Is(err,ErrNotFound): server.WriteError(w,r,http.StatusNotFound,err)
  default:
   slog.ErrorContext(r.Context(),r.Method+" "+r.URL.Path+" failed: "+err.Error())
   server.WriteError(w,r,http.StatusInternalServerError,apperror.Internal)
  }
  return
 }
 server.WriteOK(w,value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) listSent(c *gin.Context) {
 w,r := c.Writer,c.Request
{{ else -}}
func (m *Module) listSent(w http.ResponseWriter,r *http.Request) {
{{ end }}
 query,err := url.ParseQuery(r.URL.RawQuery)
 if err != nil { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
 input := ListInput{Type:query.Get("type"),Scope:query.Get("scope")}
 for _,key := range []string{"type","scope"} { if values,provided := query[key]; provided && (len(values)!=1 || values[0]=="") { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return } }
 if values,provided := query["p"]; provided {
  if len(values)!=1 { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
  parsed,err := strconv.ParseInt(values[0],10,32)
  if err!=nil { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
  value:=int32(parsed);input.Page=&value
 }
 if values,provided := query["s"]; provided {
  if len(values)!=1 { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
  parsed,err := strconv.ParseInt(values[0],10,32)
  if err!=nil { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
  value:=int32(parsed);input.PageSize=&value
 }
 value, err := m.ListSent(r.Context(),input)
 if err != nil {
  switch {
  case errors.Is(err,ErrInvalid), errors.Is(err,ErrRecipient): server.WriteError(w,r,http.StatusBadRequest,err)
  case errors.Is(err,ErrNotFound): server.WriteError(w,r,http.StatusNotFound,err)
  default:
   slog.ErrorContext(r.Context(),r.Method+" "+r.URL.Path+" failed: "+err.Error())
   server.WriteError(w,r,http.StatusInternalServerError,apperror.Internal)
  }
  return
 }
 server.WriteOK(w,value)
}
