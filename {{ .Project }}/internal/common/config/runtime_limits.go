package config

import (
	"fmt"
	"time"

	"github.com/knadh/koanf/v2"
)

// Defaults preserve older configuration directories and programmatic routers.
// The generated conf files declare the same defaults for new services.
func DefaultAudit() AuditConfig {
	return AuditConfig{DefaultWindow: 168 * time.Hour, MaxWindow: 2160 * time.Hour, ExportMaxRows: 10000, RecordTimeout: time.Second, PayloadLimit: 64 << 10}
}

func DefaultHTTPBodyLimits() HTTPBodyLimitsConfig {
	return HTTPBodyLimitsConfig{AuthChallenge: 1 << 10, SliderChallenge: 2 << 10, AuthVerification: 4 << 10, Auth: 16 << 10, Business: 1 << 20, Dictionary: 2 << 20}
}

func ResolveAudit(c AuditConfig) (AuditConfig, error) {
	d := DefaultAudit()
	if c.DefaultWindow == 0 {
		c.DefaultWindow = d.DefaultWindow
	}
	if c.MaxWindow == 0 {
		c.MaxWindow = d.MaxWindow
	}
	if c.ExportMaxRows == 0 {
		c.ExportMaxRows = d.ExportMaxRows
	}
	if c.RecordTimeout == 0 {
		c.RecordTimeout = d.RecordTimeout
	}
	if c.PayloadLimit == 0 {
		c.PayloadLimit = d.PayloadLimit
	}
	if c.DefaultWindow < time.Millisecond || c.DefaultWindow%time.Millisecond != 0 || c.MaxWindow < c.DefaultWindow || c.MaxWindow%time.Millisecond != 0 {
		return c, fmt.Errorf("audit.defaultWindow and audit.maxWindow must be positive whole milliseconds with defaultWindow <= maxWindow")
	}
	if c.ExportMaxRows <= 0 || c.ExportMaxRows == int(^uint(0)>>1) {
		return c, fmt.Errorf("audit.exportMaxRows must be positive with room for one overflow row")
	}
	if c.RecordTimeout <= 0 {
		return c, fmt.Errorf("audit.recordTimeout must be positive")
	}
	if c.PayloadLimit <= 0 {
		return c, fmt.Errorf("audit.payloadLimit must be positive")
	}
	return c, nil
}

func ResolveHTTPBodyLimits(c HTTPBodyLimitsConfig) (HTTPBodyLimitsConfig, error) {
	d := DefaultHTTPBodyLimits()
	for _, item := range []struct {
		name     string
		value    *int
		fallback int
	}{
		{"authChallenge", &c.AuthChallenge, d.AuthChallenge}, {"sliderChallenge", &c.SliderChallenge, d.SliderChallenge},
		{"authVerification", &c.AuthVerification, d.AuthVerification}, {"auth", &c.Auth, d.Auth},
		{"business", &c.Business, d.Business}, {"dictionary", &c.Dictionary, d.Dictionary},
	} {
		if *item.value == 0 {
			*item.value = item.fallback
		}
		if *item.value < 0 {
			return c, fmt.Errorf("http.bodyLimits.%s must be positive", item.name)
		}
	}
	return c, nil
}

func loadRuntimeLimits(c *Config, values *koanf.Koanf) error {
	// Omitted leaves retain compatibility defaults; explicit zero never disables
	// limits or turns a ticket/cache entry into a non-expiring Redis value.
	for _, item := range []struct {
		key  string
		zero bool
	}{
		{"dictionary.cacheTTL", c.Dictionary.CacheTTL == 0}, {"auth.mfa.loginTicketTTL", c.Auth.MFA.LoginTicketTTL == 0},
		{"audit.defaultWindow", c.Audit.DefaultWindow == 0}, {"audit.maxWindow", c.Audit.MaxWindow == 0},
		{"audit.exportMaxRows", c.Audit.ExportMaxRows == 0}, {"audit.recordTimeout", c.Audit.RecordTimeout == 0}, {"audit.payloadLimit", c.Audit.PayloadLimit == 0},
		{"http.bodyLimits.authChallenge", c.HTTP.BodyLimits.AuthChallenge == 0}, {"http.bodyLimits.sliderChallenge", c.HTTP.BodyLimits.SliderChallenge == 0},
		{"http.bodyLimits.authVerification", c.HTTP.BodyLimits.AuthVerification == 0}, {"http.bodyLimits.auth", c.HTTP.BodyLimits.Auth == 0},
		{"http.bodyLimits.business", c.HTTP.BodyLimits.Business == 0}, {"http.bodyLimits.dictionary", c.HTTP.BodyLimits.Dictionary == 0},
	} {
		if values.Exists(item.key) && item.zero {
			return fmt.Errorf("%s must be positive", item.key)
		}
	}
	if !values.Exists("dictionary.cacheTTL") {
		c.Dictionary.CacheTTL = 10 * time.Minute
	}
	if !values.Exists("auth.mfa.loginTicketTTL") {
		c.Auth.MFA.LoginTicketTTL = 5 * time.Minute
	}
	if c.Dictionary.CacheTTL <= 0 {
		return fmt.Errorf("dictionary.cacheTTL must be positive")
	}
	if c.Auth.MFA.LoginTicketTTL < time.Millisecond || c.Auth.MFA.LoginTicketTTL%time.Millisecond != 0 {
		return fmt.Errorf("auth.mfa.loginTicketTTL must be a positive whole number of milliseconds")
	}
	var err error
	c.Audit, err = ResolveAudit(c.Audit)
	if err != nil {
		return err
	}
	c.HTTP.BodyLimits, err = ResolveHTTPBodyLimits(c.HTTP.BodyLimits)
	return err
}
