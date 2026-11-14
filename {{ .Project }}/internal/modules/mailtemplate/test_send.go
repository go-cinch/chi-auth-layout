package mailtemplate

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

var ErrDeliveryUnavailable = apperror.New("MAIL_TEMPLATE_DELIVERY_UNAVAILABLE", "email delivery is disabled or in debug mode")

var ErrEmailRequired = apperror.New("MAIL_TEMPLATE_EMAIL_REQUIRED", "bind an email address before sending a test")

type TestSender interface {
	TestReady() bool
	EnqueueHTML(context.Context, string, string, string, string) error
}

func (m *Module) SetTestSender(sender TestSender) { m.sender = sender }

type TestSendInput struct {
	Locale  string            `json:"locale"`
	Content Translation       `json:"content"`
	Values  map[string]string `json:"values"`
}

// TestSend queues an editor snapshot without changing draft or publication state.
func (m *Module) TestSend(ctx context.Context, id int64, input TestSendInput) error {
	if input.Locale != "zh-CN" && input.Locale != "en-US" {
		return ErrInvalid
	}
	subject, plain, html, err := renderTest(input)
	if err != nil {
		return err
	}
	if _, err = m.Get(ctx, id); err != nil {
		return err
	}
	// Resolve the recipient from the authenticated account at enqueue time. Neither
	// an editor payload nor a stale frontend account snapshot can choose it.
	identity, ok := authn.FromContext(ctx)
	if !ok || identity.UserID <= 0 || identity.CredentialVersion <= 0 {
		return authn.ErrUnauthorized
	}
	var recipient string
	err = m.store.SQL(ctx).QueryRowContext(ctx, `SELECT COALESCE(CASE WHEN email_verified_at IS NOT NULL THEN email END, '') FROM t_user WHERE id=$1 AND credential_version=$2`, identity.UserID, identity.CredentialVersion).Scan(&recipient)
	if errors.Is(err, sql.ErrNoRows) {
		return authn.ErrUnauthorized
	}
	if err != nil {
		return err
	}
	address, err := mail.ParseAddress(recipient)
	if err != nil || address.Address != recipient || len(recipient) > 254 || strings.ContainsAny(recipient, "\r\n") {
		return ErrEmailRequired
	}
	if m.sender == nil || !m.sender.TestReady() {
		return ErrDeliveryUnavailable
	}
	return m.sender.EnqueueHTML(ctx, recipient, subject, plain, html)
}

func renderTest(input TestSendInput) (string, string, string, error) {
	return renderTranslation(input.Content, input.Values)
}
