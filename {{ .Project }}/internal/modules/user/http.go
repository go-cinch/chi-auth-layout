package user

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"

{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- else }}
	"github.com/go-chi/chi/v5"
{{- end }}
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules"
	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
)

var _ modules.HTTPModule = (*Module)(nil)
var _ modules.IdempotentHTTPModule = (*Module)(nil)

type CreateUserRequest struct {
	ActionBindings *[]actionmodule.ResourceBinding `json:"action_bindings,omitempty"`
	ChallengeID    string                          `json:"challenge_id" example:"u7rUnVYJdWzvX41WeyuR1J6mM8Xr0jYjNQQHf9U5gEo"`
	Credential     string                          `json:"credential" example:"eyJhbGciOiJSU0EtT0FFUC0yNTYiLCJlbmMiOiJBMjU2R0NNIn0..."`
	Username       string                          `json:"username" example:"operator"`
	RoleID         *int64                          `json:"role_id,omitempty"`
	ActionCodes    []string                        `json:"action_codes,omitempty"`
	Metadata       map[string]any                  `json:"metadata,omitempty"`
}

type UpdateUserRequest struct {
	ActionBindings *[]actionmodule.ResourceBinding `json:"action_bindings,omitempty"`
	ChallengeID    *string                         `json:"challenge_id,omitempty" example:"u7rUnVYJdWzvX41WeyuR1J6mM8Xr0jYjNQQHf9U5gEo"`
	Credential     *string                         `json:"credential,omitempty" example:"eyJhbGciOiJSU0EtT0FFUC0yNTYiLCJlbmMiOiJBMjU2R0NNIn0..."`
	Username       *string                         `json:"username,omitempty"`
	RoleID         *int64                          `json:"role_id,omitempty"`
	ActionCodes    *[]string                       `json:"action_codes,omitempty"`
	Status         *int16                          `json:"status,omitempty"`
	Metadata       *map[string]any                 `json:"metadata,omitempty"`
}

func (*Module) Name() string { return "/user" }

func (*Module) Idempotent() bool { return true }

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) HTTP(r *gin.RouterGroup) {
	r.GET("", m.list)
	r.GET("/summary", m.summary)
	r.GET("/:id", m.get)
	r.POST("", m.create)
	r.PATCH("/:id", m.update)
	r.DELETE("/:id", m.delete)
}
{{ else -}}
func (m *Module) HTTP() http.Handler {
	r := chi.NewRouter()
	r.Get("/", m.list)
	r.Get("/summary", m.summary)
	r.Get("/{id}", m.get)
	r.Post("/", m.create)
	r.Patch("/{id}", m.update)
	r.Delete("/{id}", m.delete)
	return r
}
{{ end }}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) get(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) get(w http.ResponseWriter, r *http.Request) {
{{ end }}
{{ if eq .Computed.http_router_final "gin" -}}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end }}
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.UserID)
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	fields, err := report.ParseFields(query, listFields...)
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	value, err := m.GetSelected(r.Context(), id, fields)
	if errors.Is(err, ErrInvalid) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	if errors.Is(err, ErrNotFound) {
		server.WriteError(w, r, http.StatusNotFound, ErrNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get user "+strconv.FormatInt(id, 10)+" failed: "+err.Error(), "operation", "get_user", "outcome", "failure", "error", err, "user_id", id)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) create(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) create(w http.ResponseWriter, r *http.Request) {
{{ end }}
	var request CreateUserRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	password, err := m.registrationCredentials.OpenRegistrationPassword(r.Context(), authmodule.EncryptedRegisterInput{
		ChallengeID: request.ChallengeID,
		Credential:  request.Credential,
	})
	if errors.Is(err, authmodule.ErrInvalidRegistrationCredential) {
		server.WriteError(w, r, http.StatusBadRequest, authmodule.ErrInvalidRegistrationCredential)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "decrypt user creation credential failed: "+err.Error(), "operation", "decrypt_user_creation_credential", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	input := CreateUserInput{
		Username:       request.Username,
		Password:       password,
		RoleID:         request.RoleID,
		ActionCodes:    request.ActionCodes,
		ActionBindings: request.ActionBindings,
		Metadata:       request.Metadata,
	}
	value, err := m.Create(r.Context(), input)
	if errors.Is(err, authn.ErrPermissionAdministrationRequired) || errors.Is(err, authn.ErrPermissionGrantRequired) {
		server.WriteError(w, r, http.StatusForbidden, err)
		return
	}
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrConflict) {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	if errors.Is(err, ErrRoleNotFound) {
		server.WriteError(w, r, http.StatusNotFound, ErrRoleNotFound)
		return
	}
	if errors.Is(err, ErrActionNotFound) {
		server.WriteError(w, r, http.StatusNotFound, ErrActionNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "create user failed: "+err.Error(), "operation", "create_user", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) update(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) update(w http.ResponseWriter, r *http.Request) {
{{ end }}
{{ if eq .Computed.http_router_final "gin" -}}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end }}
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.UserID)
		return
	}
	var request UpdateUserRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	input := UpdateUserInput{
		Username:       request.Username,
		RoleID:         request.RoleID,
		ActionCodes:    request.ActionCodes,
		ActionBindings: request.ActionBindings,
		Status:         request.Status,
		Metadata:       request.Metadata,
	}
	if request.ChallengeID != nil || request.Credential != nil {
		challengeID, credential := "", ""
		if request.ChallengeID != nil {
			challengeID = *request.ChallengeID
		}
		if request.Credential != nil {
			credential = *request.Credential
		}
		password, err := m.registrationCredentials.OpenRegistrationPassword(r.Context(), authmodule.EncryptedRegisterInput{
			ChallengeID: challengeID,
			Credential:  credential,
		})
		if errors.Is(err, authmodule.ErrInvalidRegistrationCredential) {
			server.WriteError(w, r, http.StatusBadRequest, authmodule.ErrInvalidRegistrationCredential)
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "decrypt user "+strconv.FormatInt(id, 10)+" update credential failed: "+err.Error(), "operation", "decrypt_user_update_credential", "outcome", "failure", "error", err, "user_id", id)
			server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
			return
		}
		input.Password = &password
	}
	value, err := m.Update(r.Context(), id, input)
	if errors.Is(err, authn.ErrPermissionAdministrationRequired) || errors.Is(err, authn.ErrPermissionGrantRequired) {
		server.WriteError(w, r, http.StatusForbidden, err)
		return
	}
	if errors.Is(err, apperror.FeatureDisabled) {
		server.WriteError(w, r, http.StatusForbidden, err)
		return
	}
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrConflict) {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	if errors.Is(err, ErrNotFound) {
		server.WriteError(w, r, http.StatusNotFound, ErrNotFound)
		return
	}
	if errors.Is(err, ErrRoleNotFound) {
		server.WriteError(w, r, http.StatusNotFound, ErrRoleNotFound)
		return
	}
	if errors.Is(err, ErrActionNotFound) {
		server.WriteError(w, r, http.StatusNotFound, ErrActionNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "update user "+strconv.FormatInt(id, 10)+" failed: "+err.Error(), "operation", "update_user", "outcome", "failure", "error", err, "user_id", id)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) delete(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) delete(w http.ResponseWriter, r *http.Request) {
{{ end }}
{{ if eq .Computed.http_router_final "gin" -}}
	parts := strings.Split(c.Param("id"), ",")
{{ else -}}
	parts := strings.Split(chi.URLParam(r, "id"), ",")
{{ end }}
	if len(parts) > 100 {
		server.WriteError(w, r, http.StatusBadRequest, ErrIDs)
		return
	}
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			server.WriteError(w, r, http.StatusBadRequest, ErrIDs)
			return
		}
		ids = append(ids, id)
	}
	err := m.Delete(r.Context(), ids...)
	if errors.Is(err, authn.ErrPermissionAdministrationRequired) || errors.Is(err, authn.ErrPermissionGrantRequired) {
		server.WriteError(w, r, http.StatusForbidden, err)
		return
	}
	if errors.Is(err, apperror.FeatureDisabled) {
		server.WriteError(w, r, http.StatusForbidden, err)
		return
	}
	if errors.Is(err, ErrIDs) {
		server.WriteError(w, r, http.StatusBadRequest, ErrIDs)
		return
	}
	if errors.Is(err, ErrNotFound) {
		server.WriteError(w, r, http.StatusNotFound, ErrNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "delete "+strconv.Itoa(len(ids))+" users failed: "+err.Error(), "operation", "delete_users", "outcome", "failure", "error", err, "count", len(ids))
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) list(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) list(w http.ResponseWriter, r *http.Request) {
{{ end }}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	if values, provided := query["q"]; provided && len(values) != 1 {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	fields, fieldErr := report.ParseFields(query, listFields...)
	if fieldErr != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	input := ListUsersInput{Fields: fields, Q: query.Get("q"), Username: query.Get("username"), Code: query.Get("code")}
	if values, provided := query["exclude_user_group_id"]; provided {
		if len(values) != 1 {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
		id, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || id <= 0 {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
		input.ExcludeUserGroupID = &id
	}

	if values, provided := query["created_from"]; provided {
		if len(values) != 1 {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
		parsed, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || !validTimeRange(&parsed, nil) {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
		input.CreatedFrom = &parsed
	}
	if values, provided := query["created_to"]; provided {
		if len(values) != 1 {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
		parsed, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || !validTimeRange(&parsed, nil) {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
		input.CreatedTo = &parsed
	}
	if values, provided := query["last_logged_in_from"]; provided {
		if len(values) != 1 {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
		parsed, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || !validTimeRange(&parsed, nil) {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
		input.LastLoggedInFrom = &parsed
	}
	if values, provided := query["last_logged_in_to"]; provided {
		if len(values) != 1 {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
		parsed, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || !validTimeRange(&parsed, nil) {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
		input.LastLoggedInTo = &parsed
	}
	if values, provided := query["status"]; provided {
		if len(values) != 1 {
			server.WriteError(w, r, http.StatusBadRequest, apperror.UserStatus)
			return
		}
		statuses, ok := parseStatuses(values[0])
		if !ok {
			server.WriteError(w, r, http.StatusBadRequest, apperror.UserStatus)
			return
		}
		input.Statuses = statuses
	}
	if values, provided := query["p"]; provided {
		if len(values) != 1 {
			server.WriteError(w, r, http.StatusBadRequest, apperror.Page)
			return
		}
		parsed, err := strconv.ParseInt(values[0], 10, 32)
		if err != nil {
			server.WriteError(w, r, http.StatusBadRequest, apperror.Page)
			return
		}
		value := int32(parsed)
		input.Page = &value
	}
	if values, provided := query["s"]; provided {
		if len(values) != 1 {
			server.WriteError(w, r, http.StatusBadRequest, apperror.PageSize)
			return
		}
		parsed, err := strconv.ParseInt(values[0], 10, 32)
		if err != nil {
			server.WriteError(w, r, http.StatusBadRequest, apperror.PageSize)
			return
		}
		value := int32(parsed)
		input.PageSize = &value
	}
	value, err := m.List(r.Context(), input)
	if errors.Is(err, ErrInvalid) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "list users failed: "+err.Error(), "operation", "list_users", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, value)
}

func parseStatuses(value string) ([]int16, bool) {
	result := make([]int16, 0)
	seen := make(map[int16]struct{})
	for _, item := range strings.Split(value, ",") {
		parsed, err := strconv.ParseInt(strings.TrimSpace(item), 10, 16)
		status := int16(parsed)
		if err != nil || !validStatus(status) {
			return nil, false
		}
		if _, ok := seen[status]; ok {
			continue
		}
		seen[status] = struct{}{}
		result = append(result, status)
	}
	return result, len(result) > 0
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) summary(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) summary(w http.ResponseWriter, r *http.Request) {
{{ end }}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	fields, err := report.ParseFields(query, "total", "counts", "trend")
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	input := SummaryInput{Fields: fields}
	for key := range query {
		if key != "days" && key != "time_zone" && key != "f" {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
	}
	if values, provided := query["days"]; provided {
		if len(values) != 1 {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
		input.Days, err = strconv.Atoi(values[0])
		if err != nil || (input.Days != 14 && input.Days != 30) {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
	}
	if values, provided := query["time_zone"]; provided {
		if len(values) != 1 || values[0] == "" {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
		input.TimeZone = values[0]
	}
	value, err := m.Summary(r.Context(), input)
	if errors.Is(err, ErrInvalid) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "summarize users failed: "+err.Error(), "operation", "summarize_users", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, value)
}
