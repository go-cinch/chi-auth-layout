package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
)

func TestSliderCaptchaFromConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Captcha.Slider.TTL = time.Minute
	cfg.Auth.Captcha.Slider.MinimumDuration = time.Second
	if _, err := sliderCaptchaFromConfig(cfg, authmodule.NewMemoryPointCaptchaStore()); err != nil {
		t.Fatal(err)
	}
	cfg.Auth.Captcha.Slider.TTL = 0
	if _, err := sliderCaptchaFromConfig(cfg, authmodule.NewMemoryPointCaptchaStore()); err == nil {
		t.Fatal("invalid slider captcha configuration was accepted")
	}
}

func TestEmailQueueEndpointConfiguration(t *testing.T) {
	const token = "test-token-1234567890-1234567890-1234567890"
	for _, tc := range []struct {
		name, env      string
		email, enabled bool
		token          string
		authorization  string
		status         int
	}{
		{"protected", "dev", true, true, token, "Bearer " + token, http.StatusOK},
		{"protected_without_authorization", "dev", true, true, token, "", http.StatusUnauthorized},
		{"anonymous", "dev", true, true, "", "", http.StatusOK},
		{"production", "prod", true, true, "", "", http.StatusNotFound},
		{"email_disabled", "dev", false, true, "", "", http.StatusNotFound},
		{"endpoint_disabled", "dev", true, false, "", "", http.StatusNotFound},
		{"invalid_token", "dev", true, true, "too-short", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Server.Env = tc.env
			cfg.Email.Enabled = tc.email
			cfg.Email.SkipDelivery = true
			cfg.Auth.Testing.EmailQueueEndpoint = tc.enabled
			cfg.Auth.Testing.EmailQueueToken = tc.token
			cfg.Auth.Session.Jwt.Key = strings.Repeat("s", 32)
			cfg.Auth.Email.CodeTTL = time.Minute
			cfg.Auth.Email.ResendInterval = time.Second
			cfg.Auth.Email.MaxAttempts = 3
			cfg.Auth.Email.SendLimit = 3
			app := &Application{db: &db.Store{}}
			err := app.initializeEmail(cfg)
			if tc.status == 0 {
				if err == nil || !strings.Contains(err.Error(), "auth.testing.emailQueueToken") {
					t.Fatalf("expected a token configuration error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			manager := testAuthenticator(t)
			module := authmodule.New(app.db, manager, nil, nil, nil, nil, nil, nil, nil, &authmodule.Verification{}, "/auth")
			module.SetEmailCodes(app.emailCodes)
			router, err := server.NewRouter(cfg, manager, nil, module)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/auth/pub/testing/email-queue", nil)
			if tc.authorization != "" {
				request.Header.Set("Authorization", tc.authorization)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", response.Code, tc.status, response.Body)
			}
		})
	}
}
