package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/config"
)

func TestLocalizedRouterErrors(t *testing.T) {
	handler, err := NewRouter(&config.Config{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 20 {
		for _, test := range []struct{ language, msg, locale string }{
			{"en-US", "Not Found", "en-US"}, {"zh-CN,zh;q=0.9", "请求的资源不存在", "zh-CN"}, {"fr-FR", "Not Found", "en-US"},
		} {
			group.Go(func() {
				req := httptest.NewRequest("GET", "/missing", nil)
				req.Header.Set("Accept-Language", test.language)
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, req)
				var body ErrorResponse
				if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
					t.Error(err)
					return
				}
				if recorder.Code != 404 || body.ErrorCode != apperror.Code(apperror.NotFound) || body.Msg != test.msg {
					t.Errorf("response: %d %s", recorder.Code, recorder.Body.String())
				}
				if recorder.Header().Get("Content-Language") != test.locale || recorder.Header().Get("Vary") != "Accept-Language" {
					t.Errorf("headers: %v", recorder.Header())
				}
			})
		}
	}
	group.Wait()
}

func TestLocalizedErrorWriter(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "zh-CN")
	recorder := httptest.NewRecorder()
	WriteError(recorder, req, http.StatusBadRequest, apperror.InvalidBody)
	var body ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Msg != "请求内容格式无效" || body.ErrorCode != "HTTP_INVALID_BODY" {
		t.Fatal(recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	recorder.Header().Set("Content-Language", "zh-CN")
	WriteJSON(recorder, 200, func() {})
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 500 || body.Msg != "服务器内部错误，请稍后重试" {
		t.Fatal(recorder.Body.String())
	}
}
