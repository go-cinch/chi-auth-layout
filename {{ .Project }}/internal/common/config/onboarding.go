package config

import (
	"fmt"
	"github.com/knadh/koanf/v2"
	"strconv"
)

// Older deployment YAML omits these switches. Only an explicit false may
// relax its existing onboarding requirements.
func loadOnboardingPolicy(c *Config, values *koanf.Koanf) error {
	for _, policy := range []struct {
		key    string
		target *bool
	}{
		{"auth.password.reset.required", &c.Auth.Password.Reset.Required},
		{"auth.password.reset.requiredAfterAdminReset", &c.Auth.Password.Reset.RequiredAfterAdminReset},
		{"auth.totp.requireBinding", &c.Auth.TOTP.RequireBinding},
	} {
		if !values.Exists(policy.key) {
			*policy.target = true
			continue
		}
		switch value := values.Get(policy.key).(type) {
		case bool:
			*policy.target = value
		case string:
			// Environment overrides are represented as strings before decoding.
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("%s must be a boolean", policy.key)
			}
			*policy.target = parsed
		default:
			return fmt.Errorf("%s must be a boolean", policy.key)
		}
	}
	// Before the policies were split, both gates used the first-login switch.
	if !values.Exists("auth.password.reset.requiredAfterAdminReset") {
		c.Auth.Password.Reset.RequiredAfterAdminReset = c.Auth.Password.Reset.Required
	}
	return nil
}
