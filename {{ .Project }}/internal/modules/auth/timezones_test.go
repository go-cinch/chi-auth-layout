package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
)

type timezoneStub struct {
	err   error
	empty bool
}

func (s timezoneStub) TimezoneOptions(context.Context) (dictionary.TimezoneCatalog, error) {
	if s.empty {
		return dictionary.TimezoneCatalog{Items: []dictionary.TimezoneOption{}}, s.err
	}
	return dictionary.TimezoneCatalog{Items: []dictionary.TimezoneOption{
		{Value: "Asia/Shanghai", Enabled: true},
	}}, s.err
}

func TestPublicTimezoneOptions(t *testing.T) {
	m, _, _ := newAuthTestModule(t)
	if _, err := m.TimezoneOptions(t.Context()); err == nil {
		t.Fatal("missing source")
	}
	m.SetTimezoneSource(timezoneStub{})
	response := emailHTTP(t, m, t.Context(), "GET", "/pub/timezones", nil)
	var value dictionary.TimezoneCatalog
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &value) != nil || len(value.Items) != 1 || value.Items[0].Value != "Asia/Shanghai" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(response.Code, response.Body.String())
	}
	m.SetTimezoneSource(timezoneStub{empty: true})
	response = emailHTTP(t, m, t.Context(), "GET", "/pub/timezones", nil)
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &value) != nil || value.Items == nil || len(value.Items) != 0 {
		t.Fatal(response.Body.String())
	}
	m.SetTimezoneSource(timezoneStub{err: errors.New("offline")})
	if response = emailHTTP(t, m, t.Context(), "GET", "/pub/timezones", nil); response.Code != 500 {
		t.Fatal(response.Code)
	}
	if !ownsAuthHTTPMethod("/pub/timezones", "GET") || ownsAuthHTTPMethod("/pub/timezones", "POST") {
		t.Fatal("method classification")
	}
}
