package auth

import (
	"context"
	"crypto/hmac"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type queuedTestSender struct{ testEmailSender }

func (s *queuedTestSender) TestReady() bool { return s.Ready() }

type signedTestArgs struct{ e *EmailCodes }

func (a signedTestArgs) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		return false
	}
	var args testEmailDelivery
	return json.Unmarshal([]byte(raw), &args) == nil && hmac.Equal(args.Signature, a.e.testDeliveryMAC(42, args.Payload)) && args.Payload.Recipient == "owner@example.test" && args.Payload.HTML == "<p>actual</p>"
}

func TestTestEmailEnqueuesBeforeSMTP(t *testing.T) {
	e, mock, _ := newEmailCodeTest(t)
	sender := &queuedTestSender{}
	e.sender = sender
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectExec("INSERT INTO river_job").WithArgs(int64(42), signedTestArgs{e}, e.config.SendLimit).WillReturnResult(sqlmock.NewResult(42, 1))
	mock.ExpectCommit()
	if err := e.EnqueueHTML(t.Context(), "owner@example.test", "subject", "actual", "<p>actual</p>"); err != nil {
		t.Fatal(err)
	}
	if sender.body != "" {
		t.Fatal("SMTP ran in request")
	}
}

func TestTestEmailRejectsTampering(t *testing.T) {
	e, _, _ := newEmailCodeTest(t)
	sender := &queuedTestSender{}
	e.sender = sender
	original := testEmailPayload{"owner@example.test", "subject", "plain", "<p>actual</p>", time.Now().Add(time.Minute).UnixMilli()}
	for _, field := range []string{"recipient", "subject", "plain", "html", "expiry", "signature", "job_id", "valid", "smtp_failure"} {
		t.Run(field, func(t *testing.T) {
			sender.body = ""
			sender.err = nil
			job := &river.Job[testEmailDelivery]{JobRow: &rivertype.JobRow{ID: 42}, Args: testEmailDelivery{Payload: original, Signature: e.testDeliveryMAC(42, original)}}
			switch field {
			case "recipient":
				job.Args.Payload.Recipient = "attacker@example.test"
			case "subject":
				job.Args.Payload.Subject = "forged"
			case "plain":
				job.Args.Payload.Plain = "forged"
			case "html":
				job.Args.Payload.HTML = "forged"
			case "expiry":
				job.Args.Payload.Expires++
			case "signature":
				job.Args.Signature = nil
			case "job_id":
				job.ID++
			case "smtp_failure":
				sender.err = errors.New("secret recipient")
			}
			err := e.sendTestEmail(context.Background(), job)
			if field == "valid" {
				if err != nil || sender.body != "plain" {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("failure accepted")
			}
			if field != "smtp_failure" && sender.body != "" {
				t.Fatal("tampered mail sent")
			}
			if strings.Contains(err.Error(), "secret recipient") {
				t.Fatal("transport details leaked")
			}
		})
	}
}

func TestTestEmailMalformedArgumentsAreCancelled(t *testing.T) {
	e, _, _ := newEmailCodeTest(t)
	sender := &queuedTestSender{}
	e.sender = sender
	for _, raw := range []string{`{"signature":"not base64"}`, `{"payload":{"recipient":42}}`, `{}`} {
		var args testEmailDelivery
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			t.Fatal("River must dispatch invalid args to the integrity check")
		}
		if err := e.sendTestEmail(t.Context(), &river.Job[testEmailDelivery]{JobRow: &rivertype.JobRow{ID: 42}, Args: args}); err == nil || sender.body != "" {
			t.Fatal("malformed job accepted", err)
		}
	}
}
