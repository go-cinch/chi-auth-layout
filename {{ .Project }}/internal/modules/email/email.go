package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	mail "github.com/wneessen/go-mail"
)

var ErrUnavailable = errors.New("email delivery is not configured")

type SMTPConfig struct {
	Enabled        bool
	Debug          bool
	Host           string
	Port           int
	Username       string
	Password       string
	From           string
	FromName       string
	HELO           string
	Domain         string
	Security       string
	Authentication string
	Timeout        time.Duration
}

// Module owns SMTP delivery. Authentication owns recipients and verification codes.
// A new client per delivery avoids sharing mutable connection state across requests.
type Module struct{ config SMTPConfig }

func New(cfg SMTPConfig) (*Module, error) {
	m := &Module{config: cfg}
	if !cfg.Enabled || cfg.Debug {
		return m, nil
	}
	if strings.TrimSpace(cfg.Host) == "" || cfg.Port < 1 || cfg.Port > 65535 || cfg.From == "" || cfg.Timeout <= 0 {
		return nil, errors.New("enabled email requires host, port, from and a positive timeout")
	}
	if _, err := m.client(); err != nil {
		return nil, err
	}
	msg := mail.NewMsg()
	if err := msg.FromFormat(cfg.FromName, cfg.From); err != nil {
		return nil, fmt.Errorf("invalid email sender: %w", err)
	}
	return m, nil
}

func (m *Module) Ready() bool { return m != nil && m.config.Enabled }

func (m *Module) client() (*mail.Client, error) {
	cfg := m.config
	options := []mail.Option{mail.WithoutNoop(), mail.WithPort(cfg.Port), mail.WithTimeout(cfg.Timeout), mail.WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.Host})}
	if cfg.HELO != "" {
		options = append(options, mail.WithHELO(cfg.HELO))
	}
	if cfg.Domain != "" {
		options = append(options, mail.WithDomain(cfg.Domain))
	}
	switch cfg.Security {
	case "tls":
		options = append(options, mail.WithSSL())
	case "starttls":
		options = append(options, mail.WithTLSPolicy(mail.TLSMandatory))
	case "plain":
		options = append(options, mail.WithTLSPolicy(mail.NoTLS))
	default:
		return nil, errors.New("email security must be tls, starttls or plain")
	}
	switch strings.ToUpper(cfg.Authentication) {
	case "NONE":
		if cfg.Username != "" || cfg.Password != "" {
			return nil, errors.New("email authentication none cannot contain credentials")
		}
	case "AUTO":
		if cfg.Username == "" || cfg.Password == "" {
			return nil, errors.New("email authentication requires username and password")
		}
		options = append(options, mail.WithUsername(cfg.Username), mail.WithPassword(cfg.Password), mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover))
	case "PLAIN", "LOGIN", "CRAM-MD5", "SCRAM-SHA-1", "SCRAM-SHA-1-PLUS", "SCRAM-SHA-256", "SCRAM-SHA-256-PLUS", "NTLM", "XOAUTH2":
		if cfg.Username == "" || cfg.Password == "" {
			return nil, errors.New("email authentication requires username and password")
		}
		options = append(options, mail.WithUsername(cfg.Username), mail.WithPassword(cfg.Password), mail.WithSMTPAuth(mail.SMTPAuthType(strings.ToUpper(cfg.Authentication))))
	default:
		return nil, errors.New("invalid email authentication mechanism")
	}
	return mail.NewClient(cfg.Host, options...)
}

// TestReady excludes debug mode: test sends must never report simulated delivery.
func (m *Module) TestReady() bool { return m.Ready() && !m.config.Debug }

func (m *Module) SendHTML(ctx context.Context, recipient, subject, plain, html string) error {
    return m.send(ctx, recipient, subject, plain, html)
}

func (m *Module) Send(ctx context.Context, recipient, subject, body string) error {
    return m.send(ctx, recipient, subject, body, "")
}

func (m *Module) send(ctx context.Context, recipient, subject, body, html string) error {
	if !m.Ready() {
		return ErrUnavailable
	}
	if m.config.Debug {
		return ctx.Err()
	}
	message := mail.NewMsg()
	if err := message.FromFormat(m.config.FromName, m.config.From); err != nil {
		return err
	}
	if err := message.To(recipient); err != nil {
		return err
	}
	message.Subject(subject)
	message.SetBodyString(mail.TypeTextPlain, body)
    if html != "" { message.AddAlternativeString(mail.TypeTextHTML, html) }
	message.SetMessageID()
	message.SetDate()
	client, err := m.client()
	if err != nil {
		return err
	}
	started := time.Now()
	err = client.DialAndSendWithContext(ctx, message)
	if err == nil {
		return nil
	}
	// go-mail sets IsDelivered only after the relay acknowledges the end of DATA.
	// A later RSET or QUIT failure cannot undo that acknowledgement.
	accepted := message.IsDelivered()
	stage := "connect"
	if accepted {
		stage = "close"
	}
	var sendErr *mail.SendError
	var smtpCode int
	if errors.As(err, &sendErr) {
		stage = sendErr.Reason.String()
		smtpCode = sendErr.ErrorCode()
	}
	outcome, safeError := "failure", "smtp transaction failed"
	if accepted {
		outcome, safeError = "success", "smtp connection cleanup failed after acceptance"
	}
	durationMS := time.Since(started).Milliseconds()
	// SMTP errors can contain addresses or message content. Log only the library's
	// fixed reason, numeric reply code and our own classification, never err itself.
	logMessage := fmt.Sprintf("email delivery %s: stage %s, smtp_code %d, accepted %t, duration %dms: %s", outcome, stage, smtpCode, accepted, durationMS, safeError)
	slog.WarnContext(ctx, logMessage, "operation", "email_send", "outcome", outcome,
		"error", safeError, "stage", stage, "smtp_code", smtpCode,
		"accepted", accepted, "duration_ms", durationMS)
	if accepted {
		return nil
	}
	return err
}
