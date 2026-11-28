package auth

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"time"

	"github.com/riverqueue/river"
)

type testEmailPayload struct {
	Recipient string `json:"recipient"`
	Subject   string `json:"subject"`
	Plain     string `json:"plain"`
	HTML      string `json:"html"`
	Expires   int64  `json:"expired_at"`
}
type testEmailDelivery struct {
	Payload   testEmailPayload `json:"payload"`
	Signature []byte           `json:"signature"`
	invalid   bool
}

// Decode corrupted queue arguments inside our worker so invalid types/signatures
// are warned and cancelled immediately instead of retried by River's decoder.
func (args *testEmailDelivery) UnmarshalJSON(raw []byte) error {
	type wire testEmailDelivery
	var decoded wire
	if err := json.Unmarshal(raw, &decoded); err != nil {
		*args = testEmailDelivery{invalid: true}
		return nil
	}
	*args = testEmailDelivery(decoded)
	return nil
}

func (testEmailDelivery) Kind() string { return "mail_template_test_deliver" }

func (e *EmailCodes) TestReady() bool {
	if e == nil || e.config.Debug {
		return false
	}
	sender, ok := e.sender.(interface{ TestReady() bool })
	return ok && sender.TestReady()
}

func (e *EmailCodes) testDeliveryMAC(id int64, payload testEmailPayload) []byte {
	raw, _ := json.Marshal(struct {
		ID      int64
		Payload testEmailPayload
	}{id, payload})
	return e.mac("test-delivery-v1", string(raw))
}

// Persist the rendered editor snapshot before acknowledging the request. Binding
// the MAC to River's numeric job ID also prevents copying args to another job.
func (e *EmailCodes) EnqueueHTML(ctx context.Context, recipient, subject, plain, html string) error {
	if !e.TestReady() {
		return ErrEmailUnavailable
	}
	return e.store.Tx(ctx, func(tx context.Context) error {
		var id int64
		if err := e.store.SQL(tx).QueryRowContext(tx, `SELECT nextval(pg_get_serial_sequence('river_job','id'))`).Scan(&id); err != nil {
			return err
		}
		payload := testEmailPayload{recipient, subject, plain, html, time.Now().Add(24 * time.Hour).UnixMilli()}
		raw, err := json.Marshal(testEmailDelivery{Payload: payload, Signature: e.testDeliveryMAC(id, payload)})
		if err != nil {
			return err
		}
		_, err = e.store.SQL(tx).ExecContext(tx, `INSERT INTO river_job(id,kind,queue,args,max_attempts) VALUES($1,'mail_template_test_deliver','mail_template_test',$2::jsonb,$3)`, id, string(raw), e.config.SendLimit)
		return err
	})
}

func (e *EmailCodes) sendTestEmail(ctx context.Context, job *river.Job[testEmailDelivery]) error {
	if job.Args.invalid || !hmac.Equal(job.Args.Signature, e.testDeliveryMAC(job.ID, job.Args.Payload)) {
		warnEmailIntegrity(ctx, job.ID)
		return river.JobCancel(errors.New("email integrity check failed"))
	}
	payload := job.Args.Payload
	if payload.Expires <= time.Now().UnixMilli() {
		return river.JobCancel(errors.New("test email expired"))
	}
	if !e.TestReady() {
		return ErrEmailUnavailable
	}
	ctx, cancel := context.WithDeadline(ctx, time.UnixMilli(payload.Expires))
	defer cancel()
	if err := e.sender.SendHTML(ctx, payload.Recipient, payload.Subject, payload.Plain, payload.HTML); err != nil {
		// Do not persist SMTP errors that may contain recipient or message content.
		return errors.New("test email transport failed")
	}
	if e.sentQueue != nil {
		e.sentQueue.record(sentEmail{
			Queue:     job.Queue,
			Recipient: payload.Recipient,
		})
	}
	return nil
}
