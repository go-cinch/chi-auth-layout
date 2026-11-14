package mailtemplate

import (
	"context"
	"database/sql"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

type testSender struct {
	ready                           bool
	calls                           int
	recipient, subject, plain, html string
	err                             error
}

func (s *testSender) TestReady() bool { return s.ready }

func (s *testSender) EnqueueHTML(_ context.Context, recipient, subject, plain, html string) error {
	s.calls++
	s.recipient = recipient
	s.subject = subject
	s.plain = plain
	s.html = html
	return s.err
}

func testInput() TestSendInput {
	return TestSendInput{Locale: "zh-CN", Content: sampleContent()["zh-CN"], Values: map[string]string{"code": "edited <value>"}}
}

func TestSendEditorSnapshot(t *testing.T) {
	m, mock := testModule(t)
	sender := &testSender{ready: true}
	m.SetTestSender(sender)
	mock.ExpectQuery("SELECT .* FROM t_mail_template WHERE id").WillReturnRows(recordRow(true, 1, 0, true))
	expectRecipient(mock, "test@example.com")
	input := testInput()
	input.Content.Subject = "Unsaved"
	input.Content.Text = "Code: \x7b\x7bcode\x7d\x7d"
	if err := m.TestSend(testRequest(t, httptest.NewRequest("POST", "/", nil)).Context(), 1, input); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 1 || sender.subject != "Unsaved" || sender.recipient != "test@example.com" || sender.plain != "Code: edited <value>" || !strings.Contains(sender.html, "edited &lt;value&gt;") {
		t.Fatal("snapshot or explicit parameters lost")
	}
	// sqlmock expects only a read: sending must never save or publish the template.
}

func TestSendValidationAndFailures(t *testing.T) {
	for _, change := range []func(*TestSendInput){
		func(i *TestSendInput) { i.Locale = "fr" },
		func(i *TestSendInput) { i.Values = nil }, func(i *TestSendInput) { i.Values["code"] = " " },
		func(i *TestSendInput) { i.Content.Subject = "" }, func(i *TestSendInput) { i.Content.HTML = "" },
		func(i *TestSendInput) {
			i.Content.Subject = "\x7b\x7bcode\x7d\x7d"
			i.Values["code"] = "x\r\nBcc: other@example.com"
		},
		func(i *TestSendInput) { i.Content.HTML = "\x7b\x7bbad()\x7d\x7d" },
	} {
		m, _ := testModule(t)
		sender := &testSender{ready: true}
		m.SetTestSender(sender)
		input := testInput()
		change(&input)
		if err := m.TestSend(testRequest(t, httptest.NewRequest("POST", "/", nil)).Context(), 1, input); !errors.Is(err, ErrInvalid) || sender.calls != 0 {
			t.Fatal("invalid send accepted", err)
		}
	}
	for _, failure := range []error{sql.ErrNoRows, sql.ErrConnDone} {
		m, mock := testModule(t)
		mock.ExpectQuery("SELECT .* FROM t_mail_template WHERE id").WillReturnError(failure)
		if err := m.TestSend(testRequest(t, httptest.NewRequest("POST", "/", nil)).Context(), 1, testInput()); err == nil {
			t.Fatal("database error ignored")
		}
	}
	for _, sender := range []*testSender{nil, {}, {ready: true, err: errors.New("relay rejected")}} {
		m, mock := testModule(t)
		if sender != nil {
			m.SetTestSender(sender)
		}
		mock.ExpectQuery("SELECT .* FROM t_mail_template WHERE id").WillReturnRows(recordRow(true, 1, 0, true))
		expectRecipient(mock, "test@example.com")
		if err := m.TestSend(testRequest(t, httptest.NewRequest("POST", "/", nil)).Context(), 1, testInput()); err == nil {
			t.Fatal("delivery error ignored")
		}
	}
}

func testRequest(t *testing.T, request *http.Request) *http.Request {
	t.Helper()
	manager, err := authn.New("mail-test", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(authn.Identity{Methods: authn.Methods{Primary: authn.Password}, UserID: 7, CredentialVersion: 3, Username: "tester", Code: "TEST0007"})
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request, err = manager.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func expectRecipient(mock sqlmock.Sqlmock, address string) {
	mock.ExpectQuery("SELECT COALESCE\\(CASE WHEN email_verified_at IS NOT NULL THEN email END, ''\\) FROM t_user WHERE id=\\$1 AND credential_version=\\$2").WithArgs(int64(7), int64(3)).WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow(address))
}

func TestSendRequiresCurrentVerifiedAccountEmail(t *testing.T) {
	for _, address := range []string{"", "invalid", "Other <other@example.com>"} {
		m, mock := testModule(t)
		sender := &testSender{ready: true}
		m.SetTestSender(sender)
		mock.ExpectQuery("SELECT .* FROM t_mail_template WHERE id").WillReturnRows(recordRow(true, 1, 0, true))
		expectRecipient(mock, address)
		err := m.TestSend(testRequest(t, httptest.NewRequest("POST", "/", nil)).Context(), 1, testInput())
		if !errors.Is(err, ErrEmailRequired) || sender.calls != 0 {
			t.Fatalf("unbound account sent email: %v", err)
		}
	}
	for _, failure := range []error{sql.ErrNoRows, sql.ErrConnDone} {
		m, mock := testModule(t)
		sender := &testSender{ready: true}
		m.SetTestSender(sender)
		mock.ExpectQuery("SELECT .* FROM t_mail_template WHERE id").WillReturnRows(recordRow(true, 1, 0, true))
		mock.ExpectQuery("SELECT COALESCE").WithArgs(int64(7), int64(3)).WillReturnError(failure)
		err := m.TestSend(testRequest(t, httptest.NewRequest("POST", "/", nil)).Context(), 1, testInput())
		if err == nil || sender.calls != 0 {
			t.Fatal("invalid account or database failure allowed sending")
		}
	}
}
