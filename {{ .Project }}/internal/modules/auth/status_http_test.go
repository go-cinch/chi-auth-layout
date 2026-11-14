package auth

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStatusQueryContract(t *testing.T) {
	for _, test := range []struct {
		public bool
		query  string
	}{
		{true, "purpose=login&f=requirements&username=owner"},
		{true, "purpose=login_totp_recovery&f=methods"},
		{true, "purpose=login&f=email_code&browser_uuid=" + emailTestUUID},
		{false, "purpose=email_bind&f=email_code&browser_uuid=" + emailTestUUID},
		{true, "purpose=login_totp_recovery&f=email_code&browser_uuid=" + emailTestUUID},
		{false, "purpose=totp_recovery&f=email_code&browser_uuid=" + emailTestUUID},
		{false, "purpose=sensitive&f=email_code&browser_uuid=" + emailTestUUID},
		{true, "purpose=login&f=email_code&username=owner&browser_uuid=" + emailTestUUID},
		{false, "purpose=email_unbind&f=methods"},
		{false, "purpose=totp_enrollment&f=requirements"},
		{false, "purpose=sensitive&f=grant,methods"},
	} {
		query, _ := url.ParseQuery(test.query)
		if _, err := parseStatusQuery(query, test.public); err != nil {
			t.Fatal(test, err)
		}
	}
	for _, raw := range []string{
		"", "purpose=login", "purpose=login&f=", "purpose=login&f=requirements&f=email_code",
		"purpose=login&purpose=login&f=requirements", "purpose=login&f=requirements,requirements",
		"purpose=login&f=requirements,", "purpose=login&f=options", "purpose=login&f=methods",
		"purpose=login&f=requirements", "purpose=login&f=email_code&username=owner",
		"purpose=login&f=requirements&username=owner&username=other", "purpose=login&f=requirements&username=owner&fields=all",
		"purpose=sensitive&f=methods", "purpose=totp_recovery&f=methods",
		"purpose=login_totp_recovery&f=methods&username=other", "purpose=login_totp_recovery&f=email_code",
	} {
		query, _ := url.ParseQuery(raw)
		if _, err := parseStatusQuery(query, true); err == nil {
			t.Fatal(raw)
		}
	}
	query, _ := url.ParseQuery("purpose=login&f=requirements&username=owner")
	if _, err := parseStatusQuery(query, false); err == nil {
		t.Fatal("authenticated endpoint accepted public purpose")
	}
}

func TestStatusHTTPBoundaries(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	for _, path := range []string{"/pub/status?broken=%zz", "/status?broken=%zz", "/status?purpose=login&f=requirements&username=owner", "/pub/status?purpose=sensitive&f=methods"} {
		if response := emailHTTP(t, m, t.Context(), "GET", path, nil); response.Code != 400 {
			t.Fatal(path, response.Code)
		}
	}
	if response := emailHTTP(t, m, t.Context(), "GET", "/status?purpose=sensitive&f=methods", nil); response.Code != 401 {
		t.Fatal(response.Code)
	}
	mock.ExpectQuery("SELECT wrong FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"wrong"}).AddRow(0))
	response := emailHTTP(t, m, t.Context(), "GET", "/pub/status?purpose=login&f=requirements&username=owner", nil)
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(response.Code, response.Body.String())
	}
	var value StatusResult
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil || *value.Requirements.CaptchaRequired {
		t.Fatal(value, err)
	}
	ctx := authenticatedContext(t, manager, m.sessions)
	response = emailHTTP(t, m, ctx, "GET", "/status?purpose=totp_enrollment&f=requirements", nil)
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(response.Code, response.Body.String())
	}
	response = emailHTTP(t, m, ctx, "GET", "/pub/opt", nil)
	var options map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &options)
	if response.Code != 200 || len(options) != 4 || options["totp_enabled"] != false || options["email_enabled"] != false {
		t.Fatal(options)
	}
}

func TestLoginRecoveryStatusRequiresTicketHeader(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	const path = "/pub/status?purpose=login_totp_recovery&f=methods"
	for _, ticket := range []string{"", "invalid"} {
		response := emailHTTPHeader(t, m, ctx, "GET", path, nil, "X-MFA-Ticket", ticket)
		if response.Code != 400 || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(response.Code)
		}
	}
	pending, err := m.beginMFALogin(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow(""))
	expectTOTPBound(mock, true)
	response := emailHTTPHeader(t, m, ctx, "GET", path, nil, "X-MFA-Ticket", pending.MFATicket)
	var value StatusResult
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil || response.Code != 200 || value.Methods == nil || value.Methods.Email || !value.Methods.RecoveryCode {
		t.Fatal(response.Code, response.Body.String(), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
