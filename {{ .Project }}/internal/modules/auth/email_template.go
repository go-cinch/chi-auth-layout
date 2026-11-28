package auth

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

type EmailTemplateRenderer interface {
	RenderPublished(context.Context, string, string, map[string]string) (string, string, string, error)
}

type EmailTemplateIDs struct {
	EmailLinked string
	SignInCode, EmailBindingCode, SensitiveOperationCode, AuthenticatorResetCode, ConfirmEmailUnlinking, EmailUnlinked, AuthenticatorLinked, AuthenticatorRelinked, SensitiveOperationNotification string
}

func (e *EmailCodes) SetEmailTemplates(renderer EmailTemplateRenderer) { e.templates = renderer }

// The purpose-to-parameter contract is owned by Auth, never by editor samples or
// client-supplied maps. Only the template identifiers are deployment configuration.
func (e *EmailCodes) deliveryMessage(ctx context.Context, id string, job emailAttempt) (string, string, string, error) {
	var templateID string
	values := map[string]string{"account": job.account}
	ids := e.config.Templates
	switch job.purpose {
	case "login":
		templateID = ids.SignInCode
	case "bind":
		templateID = ids.EmailBindingCode
	case "sensitive":
		templateID = ids.SensitiveOperationCode
		values["authorization_duration"] = emailAuthorizationDuration(e.config.SensitiveLifetime, job.language)
	case "totp_reset":
		templateID = ids.AuthenticatorResetCode
	case "unlink":
		templateID = ids.ConfirmEmailUnlinking
		if !validUnlinkURL(e.config.UnlinkURL) {
			return "", "", "", ErrEmailUnavailable
		}
		values["action_url"] = e.config.UnlinkURL + "#token=" + id + "." + e.digest("unlink-link", id)
		values["expires_at"] = time.UnixMilli(job.expires).In(emailTimeZone(job.timezone)).Format(time.RFC3339)
	case "unlink_notice":
		templateID = ids.EmailUnlinked
	case "email_linked":
		templateID = ids.EmailLinked
	case "totp_bound":
		templateID = ids.AuthenticatorLinked
	case "totp_rebound":
		templateID = ids.AuthenticatorRelinked
	case "sensitive_notice":
		templateID = ids.SensitiveOperationNotification
		values["activity_summary"] = sensitiveActivitySummary(job.detail, job.language)
		values["activity_started_at"], values["activity_ended_at"] = sensitiveActivityPeriod(job.detail, job.language, job.timezone)
	default:
		return "", "", "", errors.New("unsupported email delivery purpose")
	}
	if code := e.deliveryCode(id, job.purpose); code != "" {
		values["code"] = code
		values["expires_at"] = time.UnixMilli(job.expires).In(emailTimeZone(job.timezone)).Format(time.RFC3339)
	}
	switch job.purpose {
	case "unlink_notice", "email_linked", "totp_bound", "totp_rebound", "sensitive_notice":
		link, err := emailAuditURL(e.config.AuditURL)
		if err != nil {
			return "", "", "", err
		}
		values["audit_url"] = link
	}
	if e.templates == nil || strings.TrimSpace(templateID) == "" {
		return "", "", "", ErrEmailUnavailable
	}
	return e.templates.RenderPublished(ctx, templateID, job.language, values)
}

// Use recipient-facing units rather than Go's duration notation in email copy.
func emailAuthorizationDuration(duration time.Duration, language string) string {
	if duration < time.Second { return duration.String() }
	seconds := int64((duration + time.Second - 1) / time.Second)
	units := []struct { seconds int64; zh, en string }{
		{3600, "小时", "hour"}, {60, "分钟", "minute"}, {1, "秒", "second"},
	}
	parts := make([]string, 0, 3)
	for _, unit := range units {
		count := seconds / unit.seconds
		seconds %= unit.seconds
		if count == 0 { continue }
		label := unit.zh
		if language == "en-US" {
			label = unit.en
			if count != 1 { label += "s" }
		}
		parts = append(parts, strconv.FormatInt(count, 10)+" "+label)
	}
	return strings.Join(parts, " ")
}

func emailAuditURL(base string) (string, error) {
	if !validUnlinkURL(base) {
		return "", ErrEmailUnavailable
	}
	link, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	target := link
	if link.Fragment != "" {
		target, err = url.Parse(link.Fragment)
		if err != nil {
			return "", err
		}
	}
	query := target.Query()
	query.Del("username")
	query.Set("days", "30")
	target.RawQuery = query.Encode()
	if target != link {
		link.Fragment = target.String()
	}
	return link.String(), nil
}

// Resolve the current recipient preference; empty, invalid and host-local zones
// consistently fall back to Shanghai. Embedded tzdata also covers minimal images.
func emailTimeZone(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name != "" && name != "Local" {
		if location, err := time.LoadLocation(name); err == nil {
			return location
		}
	}
	location, _ := time.LoadLocation("Asia/Shanghai")
	return location
}
