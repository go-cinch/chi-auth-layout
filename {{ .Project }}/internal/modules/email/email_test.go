package email

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func smtpConfig() SMTPConfig {
	return SMTPConfig{Enabled: true, Host: "127.0.0.1", Port: 2525, From: "sender@example.com", FromName: "{{ (fromJSON .Computed.brand).title }}", HELO: "localhost", Domain: "TEST", Security: "plain", Authentication: "none", Timeout: time.Second}
}

func TestSMTPConfiguration(t *testing.T) {
	disabled, err := New(SMTPConfig{})
	if err != nil || disabled.Ready() || !errors.Is(disabled.Send(context.Background(), "", "", ""), ErrUnavailable) {
		t.Fatal("unconfigured SMTP must stay disabled", err)
	}
	var absent *Module
	if absent.Ready() || !errors.Is(absent.Send(context.Background(), "", "", ""), ErrUnavailable) {
		t.Fatal("nil sender must fail closed")
	}
	for _, security := range []string{"tls", "starttls", "plain"} {
		cfg := smtpConfig()
		cfg.Security = security
		if m, err := New(cfg); err != nil || !m.Ready() {
			t.Fatal(security, err)
		}
	}
	for _, method := range []string{"auto", "PLAIN", "LOGIN", "CRAM-MD5", "SCRAM-SHA-1", "SCRAM-SHA-256", "XOAUTH2"} {
		cfg := smtpConfig()
		cfg.Security = "starttls"
		cfg.Authentication = method
		cfg.Username = "name"
		cfg.Password = "credential"
		if _, err := New(cfg); err != nil {
			t.Fatal(method, err)
		}
	}
	invalid := []func(*SMTPConfig){
		func(c *SMTPConfig) { c.Host = "" }, func(c *SMTPConfig) { c.Port = 0 }, func(c *SMTPConfig) { c.Port = 65536 },
		func(c *SMTPConfig) { c.From = "" }, func(c *SMTPConfig) { c.Timeout = 0 },
		func(c *SMTPConfig) { c.Security = "opportunistic" }, func(c *SMTPConfig) { c.Authentication = "invalid" },
		func(c *SMTPConfig) { c.Username = "unexpected" }, func(c *SMTPConfig) { c.Authentication = "auto" },
		func(c *SMTPConfig) { c.Authentication = "PLAIN" }, func(c *SMTPConfig) { c.From = "not an email" },
	}
	for i, change := range invalid {
		cfg := smtpConfig()
		change(&cfg)
		if _, err := New(cfg); err == nil {
			t.Fatalf("invalid configuration %d accepted", i)
		}
	}
}

func TestSMTPDelivery(t *testing.T) {
	for _, scenario := range []struct {
		name         string
		dataResponse string
		quitResponse string
		wantSuccess  bool
	}{
		{"accepted", "250 accepted", "221 bye", true},
		{"accepted then connection close failed", "250 accepted", "421 closing recipient@example.com Verification 123456", true},
		{"relay rejected message", "550 rejected recipient@example.com Verification 123456", "221 bye", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			checkSMTPDelivery(t, scenario.dataResponse, scenario.quitResponse, scenario.wantSuccess, false)
            checkSMTPDelivery(t, scenario.dataResponse, scenario.quitResponse, scenario.wantSuccess, true)
		})
	}
}

func checkSMTPDelivery(t *testing.T, dataResponse, quitResponse string, wantSuccess, html bool) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	messages := make(chan string, 1)
	failures := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			failures <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(conn)
		write := func(value string) { _, _ = fmt.Fprint(conn, value+"\r\n") }
		write("220 localhost SMTP test")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				failures <- err
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				write("250-localhost")
				write("250 8BITMIME")
			case strings.HasPrefix(line, "DATA"):
				write("354 send message")
				var body strings.Builder
				for {
					part, err := reader.ReadString('\n')
					if err != nil {
						failures <- err
						return
					}
					if part == ".\r\n" {
						break
					}
					body.WriteString(part)
				}
				messages <- body.String()
				write(dataResponse)
			case strings.HasPrefix(line, "QUIT"):
				write(quitResponse)
				return
			default:
				write("250 OK")
			}
		}
	}()
	cfg := smtpConfig()
	cfg.Port = listener.Addr().(*net.TCPAddr).Port
	module, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	send := func() error {
        if html { return module.SendHTML(context.Background(), "recipient@example.com", "Verification", "Your code: 123456", "<b>123456</b>") }
        return module.Send(context.Background(), "recipient@example.com", "Verification", "Your code: 123456")
    }
    var logs bytes.Buffer
    previousLogger := slog.Default()
    slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
    defer slog.SetDefault(previousLogger)
    if err := send(); (err == nil) != wantSuccess {
		t.Fatalf("send success = %t, want %t", err == nil, wantSuccess)
	}
    if dataResponse == "250 accepted" && quitResponse == "221 bye" {
        if logs.Len() != 0 { t.Fatalf("successful delivery logged an error: %s", logs.String()) }
    } else {
        var entry map[string]any
        if err := json.Unmarshal(logs.Bytes(), &entry); err != nil { t.Fatal(err, logs.String()) }
        outcome, safeError := "failure", "smtp transaction failed"
        if wantSuccess { outcome, safeError = "success", "smtp connection cleanup failed after acceptance" }
        if entry["operation"] != "email_send" || entry["outcome"] != outcome || entry["error"] != safeError || entry["accepted"] != wantSuccess {
            t.Fatalf("incorrect delivery log: %v", entry)
        }
        stage, stageOK := entry["stage"].(string)
        code, codeOK := entry["smtp_code"].(float64)
        duration, durationOK := entry["duration_ms"].(float64)
        if !stageOK || stage == "" || !codeOK || !durationOK || code != float64(int(code)) || duration < 0 || duration != float64(int64(duration)) {
            t.Fatalf("wrong log field types or values: %v", entry)
        }
        wantMessage := fmt.Sprintf("email delivery %s: stage %s, smtp_code %d, accepted %t, duration %dms: %s", outcome, stage, int(code), wantSuccess, int64(duration), safeError)
        if entry["msg"] != wantMessage { t.Fatalf("message and structured fields disagree: %v", entry) }
        for _, secret := range []string{"recipient@example.com", "sender@example.com", "Verification", "123456", "<b>"} {
            if strings.Contains(logs.String(), secret) { t.Fatalf("SMTP error leaked %q", secret) }
        }
    }
	select {
	case body := <-messages:
        if html && (!strings.Contains(body,"multipart/alternative") || !strings.Contains(body,"text/html") || !strings.Contains(body,"text/plain")) { t.Fatal("missing multipart bodies") }
		if !strings.Contains(body, "recipient@example.com") || !strings.Contains(body, "123456") || !strings.Contains(body, "Verification") {
			t.Fatal("SMTP message lost its recipient or payload")
		}
	case err := <-failures:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("SMTP message missing")
	}
	if err := module.Send(context.Background(), "invalid recipient", "", ""); err == nil {
		t.Fatal("invalid recipient accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := module.Send(ctx, "recipient@example.com", "", ""); err == nil {
		t.Fatal("canceled send must fail")
	}
}

func TestDebugDeliveryNeverConnectsToSMTP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := smtpConfig()
	cfg.Debug = true
	cfg.Port = listener.Addr().(*net.TCPAddr).Port
	sender, err := New(cfg)
	if err != nil || !sender.Ready() {
		t.Fatal("debug sender unavailable", err)
	}
	if err := sender.Send(t.Context(), "recipient@example.com", "Verification", "000000"); err != nil {
		t.Fatal(err)
	}
	_ = listener.(*net.TCPListener).SetDeadline(time.Now().Add(50 * time.Millisecond))
	if conn, err := listener.Accept(); err == nil {
		conn.Close()
		t.Fatal("debug delivery connected to SMTP")
	} else if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatal(err)
	}
	sender, err = New(SMTPConfig{Enabled: true, Debug: true})
	if err != nil || !sender.Ready() {
		t.Fatal("debug mode requires SMTP settings", err)
	}
	if err := sender.Send(t.Context(), "recipient@example.com", "", ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := sender.Send(ctx, "recipient@example.com", "", ""); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
	sender, err = New(SMTPConfig{Debug: true})
	if err != nil || sender.Ready() || !errors.Is(sender.Send(t.Context(), "", "", ""), ErrUnavailable) {
		t.Fatal("debug enabled a disabled transport", err)
	}
}

func TestHTMLDeliveryDebugAndDisabled(t *testing.T) {
    m, _ := New(SMTPConfig{})
    if !errors.Is(m.SendHTML(t.Context(),"test@example.com","Subject","Text","<p>Text</p>"),ErrUnavailable) { t.Fatal("disabled sender delivered") }
    m, _ = New(SMTPConfig{Enabled:true,Debug:true})
    if m.TestReady() { t.Fatal("debug test sender ready") }
    if err := m.SendHTML(t.Context(),"test@example.com","Subject","Text","<p>Text</p>"); err != nil { t.Fatal(err) }
}
