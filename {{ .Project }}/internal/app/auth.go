package app

import (
	"context"
	"fmt"
{{- if not .Computed.enable_redis_final }}
	"log/slog"
{{- end }}
	"strings"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
	dictionarymodule "{{ .Computed.module_name_final }}/internal/modules/dictionary"
	emailmodule "{{ .Computed.module_name_final }}/internal/modules/email"
{{- if .Computed.enable_redis_final }}
	"{{ .Computed.module_name_final }}/internal/infra/rds"
{{- end }}
)

func (a *Application) initializeAuth(ctx context.Context, cfg *config.Config) error {
	var idempotencyKeys idempotency.Store = idempotency.NewMemoryStore()

{{- if .Computed.enable_redis_final }}
	idempotencyKeys = rds.NewIdempotencyStore(a.rds)

{{- end }}
	var challengeStore authmodule.ChallengeStore = authmodule.NewMemoryChallengeStore()

{{- if .Computed.enable_redis_final }}
	var sessionStore authmodule.SessionStore = authmodule.NewRedisSessionStore(a.rds)

{{- else }}
	memorySessions := authmodule.NewMemorySessionStore()
	a.cleanups = append(a.cleanups, func() { _ = memorySessions.Close() })
	var sessionStore authmodule.SessionStore = memorySessions
	slog.WarnContext(ctx, "using process-local sessions: single instance only, restart loses sessions, capacity 10000", "operation", "initialize_session_store", "outcome", "success", "store", "memory", "capacity", 10000)

{{- end }}
	var pointCaptchaStore authmodule.PointCaptchaStore = authmodule.NewMemoryPointCaptchaStore()
	var passwordFailureStore authmodule.PasswordFailureStore = authmodule.NewMemoryPasswordFailureStore()
	var dictionaryCache dictionarymodule.ValueCache = dictionarymodule.NewMemoryValueCache()

{{- if .Computed.enable_redis_final }}
	challengeStore = authmodule.NewRedisChallengeStore(a.rds)
	pointCaptchaStore = authmodule.NewRedisPointCaptchaStore(a.rds)
	passwordFailureStore = authmodule.NewRedisPasswordFailureStore(a.rds)
	redisDictionaryCache, cleanupDictionaryCache, err := rds.NewRedisNamespaceCache(cfg.Redis.DSN, cfg.RedisPrefix(), "dictionary:cache")
	if err != nil {
		return fmt.Errorf("initialize dictionary cache: %w", err)
	}
	dictionaryCache = redisDictionaryCache
	a.cleanups = append(a.cleanups, cleanupDictionaryCache)

{{- end }}
	if err := a.initializeDictionary(ctx, cfg, dictionaryCache); err != nil {
		return err
	}
	credentials, err := authmodule.NewCredentials(
		cfg.Auth.ChallengeEncryption.KeyID,
		cfg.Auth.ChallengeEncryption.PrivateKeyFile,
		cfg.Auth.ChallengeEncryption.TTL,
		challengeStore,
	)
	if err != nil {
		return fmt.Errorf("initialize challenge encryption: %w", err)
	}
	loginCaptcha, err := authmodule.NewPointCaptcha(
		pointCaptchaStore,
		a.dictionaryModule,
		int64(cfg.Auth.Captcha.Point.Threshold),
		cfg.Auth.Captcha.Point.TTL,
		cfg.Auth.Captcha.Point.Width,
		cfg.Auth.Captcha.Point.Height,
		cfg.Auth.Captcha.Point.Tolerance,
		cfg.Auth.Testing.SkipPointCaptchaAnswer,
	)
	if err != nil {
		return fmt.Errorf("initialize point captcha: %w", err)
	}
	sliderCaptcha, err := sliderCaptchaFromConfig(cfg, pointCaptchaStore)
	if err != nil {
		return fmt.Errorf("initialize slider captcha: %w", err)
	}
	passwordGuard, err := authmodule.NewPasswordChangeGuard(
		passwordFailureStore,
		loginCaptcha,
		int64(cfg.Auth.Password.Change.Threshold),
		int64(cfg.Auth.Password.Change.LockThreshold),
		cfg.Auth.Password.Change.FailureTTL,
	)
	if err != nil {
		return fmt.Errorf("initialize password change protection: %w", err)
	}
	loginSessions, err := authmodule.NewSessions(
		sessionStore,
		authmodule.SessionConfig{
			Issuer:           cfg.Auth.Session.Jwt.Issuer,
			Key:              cfg.Auth.Session.Jwt.Key,
			SessionLifetime:  cfg.Auth.Session.Refresh.SessionLifetime,
			RememberLifetime: cfg.Auth.Session.Refresh.RememberLifetime,
			RetryWindow:      cfg.Auth.Session.Refresh.RetryWindow,
		},
	)
	if err != nil {
		return fmt.Errorf("initialize login sessions: %w", err)
	}
	authenticator, err := authn.New(cfg.Auth.Session.Jwt.Issuer, cfg.Auth.Session.Jwt.Key, cfg.Auth.Session.Jwt.Lifetime, authmodule.NewIdentityValidator(a.db, loginSessions, a.authPath))
	if err != nil {
		return fmt.Errorf("initialize authentication: %w", err)
	}

	var verificationState authmodule.VerificationStateStore

{{- if .Computed.enable_redis_final }}
	verificationState = authmodule.NewRedisVerificationStore(a.rds)

{{- end }}
	verification, err := authmodule.NewVerification(a.db, verificationState, authmodule.VerificationConfig{
		TOTPEnabled: cfg.Auth.TOTP.Enabled, TOTPDebug: cfg.Auth.Testing.TOTPCode, EmailEnabled: cfg.Email.Enabled,
		TOTPEnrollmentOptional: !cfg.Auth.TOTP.RequireBinding,
		LoginTicketTTL:     cfg.Auth.MFA.LoginTicketTTL,
		TOTPPeriod:         cfg.Auth.TOTP.Period, TOTPEnrollmentTTL: cfg.Auth.TOTP.EnrollmentTTL,
		SensitiveLifetime:    cfg.Auth.Verification.SensitiveLifetime,
		RateLimitWindow:      cfg.Auth.Verification.RateLimit.Window,
		RateLimitMaxAttempts: int64(cfg.Auth.Verification.RateLimit.MaxAttempts),
		TOTPAccountNamePrefix:    cfg.Auth.TOTP.AccountNamePrefix,
		Issuer:               cfg.Auth.Session.Jwt.Issuer, SigningKey: cfg.Auth.Session.Jwt.Key,
		EncryptionKeyFile: cfg.Auth.Verification.EncryptionKeyFile,
	})
	if err != nil {
		return fmt.Errorf("initialize verification: %w", err)
	}
	a.idempotencyKeys = idempotencyKeys
	a.credentials = credentials
	a.loginCaptcha = loginCaptcha
	a.sliderCaptcha = sliderCaptcha
	a.passwordGuard = passwordGuard
	a.loginSessions = loginSessions
	a.authenticator = authenticator
	a.verification = verification
	return a.initializeEmail(cfg)
}

func (a *Application) initializeEmail(cfg *config.Config) error {
	emailModule, err := emailmodule.New(emailmodule.SMTPConfig{
		Enabled: cfg.Email.Enabled, Debug: cfg.Email.SkipDelivery, Host: cfg.Email.Host, Port: cfg.Email.Port,
		Username: cfg.Email.Username, Password: cfg.Email.Password, From: cfg.Email.From,
		FromName: cfg.Email.FromName, HELO: cfg.Email.Helo, Domain: cfg.Email.Domain, Security: cfg.Email.Security,
		Authentication: cfg.Email.Authentication, Timeout: cfg.Email.Timeout,
	})
	if err != nil {
		return fmt.Errorf("initialize email delivery: %w", err)
	}
	queueEnabled := cfg.Server.Env == "dev" && cfg.Email.Enabled && cfg.Auth.Testing.EmailQueueEndpoint
	var queueToken string
	if queueEnabled {
		queueToken = cfg.Auth.Testing.EmailQueueToken
		if queueToken != "" && (len(queueToken) < 32 || strings.ContainsAny(queueToken, " \t\r\n")) {
			return fmt.Errorf("auth.testing.emailQueueToken must be empty or at least 32 bytes with no whitespace")
		}
	}
	var tiers []authmodule.AlertTier
	for _, tier := range cfg.Auth.Email.AlertTiers {
		tiers = append(tiers, authmodule.AlertTier{Window: tier.Window, Count: tier.Count})
	}
	emailCodes, err := authmodule.NewEmailCodes(a.db, emailModule, authmodule.EmailConfig{
		HistoryRetentionDays: cfg.Auth.History.RetentionDays,
		Templates: authmodule.EmailTemplateIDs{
			SignInCode: cfg.Auth.Email.Templates.SignInCode, EmailBindingCode: cfg.Auth.Email.Templates.EmailBindingCode,
			SensitiveOperationCode: cfg.Auth.Email.Templates.SensitiveOperationCode, AuthenticatorResetCode: cfg.Auth.Email.Templates.AuthenticatorResetCode,
			ConfirmEmailUnlinking: cfg.Auth.Email.Templates.ConfirmEmailUnlinking, EmailUnlinked: cfg.Auth.Email.Templates.EmailUnlinked,
			EmailLinked:         cfg.Auth.Email.Templates.EmailLinked,
			AuthenticatorLinked: cfg.Auth.Email.Templates.AuthenticatorLinked, AuthenticatorRelinked: cfg.Auth.Email.Templates.AuthenticatorRelinked,
			SensitiveOperationNotification: cfg.Auth.Email.Templates.SensitiveOperationNotification,
		}, AuditURL: cfg.Auth.Email.AuditURL,
		TestingQueueEndpoint: queueEnabled,
		TestingQueueToken:    queueToken,
		Debug: cfg.Auth.Testing.EmailCode, AlertTiers: tiers, SensitiveLifetime: cfg.Auth.Verification.SensitiveLifetime,
		RequireBinding: cfg.Auth.Email.RequireBinding, CodeTTL: cfg.Auth.Email.CodeTTL,
		ResendInterval: cfg.Auth.Email.ResendInterval, MaxAttempts: cfg.Auth.Email.MaxAttempts,
		SendLimit:         cfg.Auth.Email.SendLimit,
		UnlinkURL: cfg.Auth.Email.UnlinkURL,
	}, cfg.Auth.Session.Jwt.Key)
	if err != nil {
		return fmt.Errorf("initialize email verification: %w", err)
	}
	a.emailModule = emailModule
	a.emailCodes = emailCodes
	return nil
}

func sliderCaptchaFromConfig(cfg *config.Config, store authmodule.PointCaptchaStore) (*authmodule.SliderCaptcha, error) {
	return authmodule.NewSliderCaptcha(store, authmodule.SliderCaptchaConfig{
		TTL:             cfg.Auth.Captcha.Slider.TTL,
		MinimumDuration: cfg.Auth.Captcha.Slider.MinimumDuration,
		SkipDuration:    cfg.Auth.Testing.SkipSliderCaptchaDuration,
	})
}
