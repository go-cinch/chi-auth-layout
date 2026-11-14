package app

import (
	"context"
	"fmt"
{{- if .Computed.enable_trace_final }}
	"log/slog"
{{- end }}
	"net/http"
	"os"
	"time"

{{- if .Computed.enable_grpc_final }}
	"google.golang.org/grpc"
{{- end }}
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"{{ .Computed.module_name_final }}/internal/common/logging"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
{{- if .Computed.enable_trace_final }}
	"{{ .Computed.module_name_final }}/internal/common/tracing"
{{- end }}
	"{{ .Computed.module_name_final }}/internal/infra/db"
{{- if .Computed.enable_redis_final }}
	"{{ .Computed.module_name_final }}/internal/infra/rds"
{{- end }}
	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
	auditmodule "{{ .Computed.module_name_final }}/internal/modules/audit"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
	dictionarymodule "{{ .Computed.module_name_final }}/internal/modules/dictionary"
	emailmodule "{{ .Computed.module_name_final }}/internal/modules/email"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
	mailtemplatemodule "{{ .Computed.module_name_final }}/internal/modules/mailtemplate"
	msgmodule "{{ .Computed.module_name_final }}/internal/modules/msg"
	rolemodule "{{ .Computed.module_name_final }}/internal/modules/role"
	usermodule "{{ .Computed.module_name_final }}/internal/modules/user"
	groupmodule "{{ .Computed.module_name_final }}/internal/modules/usergroup"
)

// Application keeps direct dependency fields for modulegen's type-based wiring.
type Application struct {
	authPath                    string
	passwordResetOptional       bool
	onboardingVisibility        authmodule.OnboardingVisibility
	auditSettings               config.AuditConfig
	pagination                  pagination.Limits
	localizationRefreshInterval time.Duration

	authenticator   *authn.Manager
	idempotencyKeys idempotency.Store
	credentials     *authmodule.Credentials
	loginSessions   *authmodule.Sessions
	verification             *authmodule.Verification
	loginCaptcha    *authmodule.PointCaptcha
	sliderCaptcha   *authmodule.SliderCaptcha
	passwordGuard   *authmodule.PasswordChangeGuard
	emailCodes      *authmodule.EmailCodes

	localization       *localization.Module
	actionModule       *actionmodule.Module
	auditModule        *auditmodule.Module
	authModule         *authmodule.Module
	dictionaryModule   *dictionarymodule.Module
	emailModule        *emailmodule.Module
	mailTemplateModule *mailtemplatemodule.Module
	msgModule          *msgmodule.Module
	roleModule         *rolemodule.Module
	userModule         *usermodule.Module
	groupModule        *groupmodule.Module

	server         *http.Server
	profilerServer *http.Server

{{- if .Computed.enable_grpc_final }}
	grpcServer *grpc.Server
	grpcAddr   string

{{- end }}
	shutdownTimeout time.Duration

	db *db.Store

{{- if .Computed.enable_redis_final }}
	rds *rds.Client

{{- end }}
	cleanups []func()
}

func New(ctx context.Context, confPath string) (*Application, error) {
	cfg, overrides, err := config.LoadDir(confPath)
	if err != nil {
		return nil, err
	}

	if err := cfg.ValidateServer(); err != nil { return nil, err }

	authPath, err := config.NormalizeHTTPPath(cfg.Auth.Path)
	if err != nil {
		return nil, err
	}

	if err := logging.Init(os.Stdout, cfg.Log.Level); err != nil {
		return nil, err
	}
	config.LogOverrides(overrides)
	limits, err := pagination.New(cfg.Pagination.MaxP, cfg.Pagination.MaxS)
	if err != nil {
		return nil, err
	}
	application := &Application{
		passwordResetOptional:       !cfg.Auth.Password.Reset.Required,
		onboardingVisibility:        authmodule.OnboardingVisibility{TOTP: cfg.Auth.TOTP.HideOnboarding, PasswordReset: cfg.Auth.Password.Reset.HideOnboarding, Email: cfg.Auth.Email.HideOnboarding},
		auditSettings:               cfg.Audit,
		authPath:                    authPath,
		pagination:                  limits,
		localizationRefreshInterval: cfg.Dictionary.LocalizationRefreshInterval,
	}
	initialized := false
	defer func() {
		if !initialized {
			application.Close()
		}
	}()
	if err := application.initializeInfrastructure(ctx, cfg); err != nil {
		return nil, err
	}
	if err := application.initializeAuth(ctx, cfg); err != nil {
		return nil, err
	}
	application.wireBusinessModules()

	httpModules {{ if .Computed.enable_grpc_final }}, grpcModules {{ end }} := application.generatedModules()
	if err := application.configureTransports(cfg, httpModules {{ if .Computed.enable_grpc_final }}, grpcModules {{ end }}); err != nil {
		return nil, err
	}
	initialized = true
	return application, nil
}

func (a *Application) initializeInfrastructure(ctx context.Context, cfg *config.Config) error {

{{- if .Computed.enable_trace_final }}
	if cfg.Tracer.Enabled {
		provider, err := tracing.NewProvider(ctx, cfg)
		if err != nil {
			return err
		}
		a.cleanups = append(a.cleanups, func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := provider.Shutdown(shutdownCtx); err != nil {
				slog.ErrorContext(shutdownCtx, "shutdown tracer failed: "+err.Error(), slog.String("operation", "tracer_shutdown"), slog.String("outcome", "failure"), slog.Any("error", err))
			}
		})
	}

{{- end }}
	dbStore, cleanupDB, err := db.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	a.db = dbStore
	a.cleanups = append(a.cleanups, cleanupDB)

{{- if .Computed.enable_redis_final }}
	rdsClient, cleanupRDS, err := rds.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize redis: %w", err)
	}
	a.rds = rdsClient
	a.cleanups = append(a.cleanups, cleanupRDS)

{{- end }}
	return nil
}
