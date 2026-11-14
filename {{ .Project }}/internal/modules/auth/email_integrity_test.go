package auth

import (
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func expectEmailSeal(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT verification_id, COALESCE").WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "recipient", "purpose", "created_at", "expired_at", "notice_detail", "account", "language"}))
}

func TestEmailIntegrityCoversDeliveryInputs(t *testing.T) {
	e, _, _ := newEmailCodeTest(t)
	original := emailAttempt{userID: 1, recipient: "user@example.test", purpose: "bind", created: 1, expires: 2, detail: "detail", account: "owner", language: "zh-CN"}
	original.signature = e.deliveryMAC("ticket", original)
	if !e.validDelivery("ticket", original) {
		t.Fatal("valid signature rejected")
	}
	for _, change := range []func(*emailAttempt){
		func(j *emailAttempt) { j.recipient = "attacker@example.test" }, func(j *emailAttempt) { j.purpose = "login" },
		func(j *emailAttempt) { j.userID++ }, func(j *emailAttempt) { j.created++ }, func(j *emailAttempt) { j.expires++ },
		func(j *emailAttempt) { j.detail = "forged" }, func(j *emailAttempt) { j.account = "other" },
		func(j *emailAttempt) { j.language = "en-US" }, func(j *emailAttempt) { j.signature = "" },
	} {
		job := original
		change(&job)
		if e.validDelivery("ticket", job) {
			t.Fatal("tampering accepted")
		}
	}
	if e.validDelivery("other-ticket", original) {
		t.Fatal("signature transplant accepted")
	}
}
