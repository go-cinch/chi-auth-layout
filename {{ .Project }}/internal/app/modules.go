package app

import (
	"context"
	"fmt"

	"{{ .Computed.module_name_final }}/internal/common/config"
	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
	auditmodule "{{ .Computed.module_name_final }}/internal/modules/audit"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
	dictionarymodule "{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
	mailtemplatemodule "{{ .Computed.module_name_final }}/internal/modules/mailtemplate"
	msgmodule "{{ .Computed.module_name_final }}/internal/modules/msg"
	rolemodule "{{ .Computed.module_name_final }}/internal/modules/role"
	usermodule "{{ .Computed.module_name_final }}/internal/modules/user"
	groupmodule "{{ .Computed.module_name_final }}/internal/modules/usergroup"
)

func (a *Application) initializeDictionary(ctx context.Context, cfg *config.Config, dictionaryCache dictionarymodule.ValueCache) error {
	localizer := localization.New(authmodule.AuthHTTPPaths(a.authPath))
	dictionaryModule, err := dictionarymodule.New(a.db, dictionaryCache, cfg.Dictionary.CacheTTL, a.pagination, localizer)
	if err != nil {
		return fmt.Errorf("initialize dictionary module: %w", err)
	}
	if err := dictionaryModule.LoadLocalization(ctx); err != nil {
		return fmt.Errorf("initialize localization: %w", err)
	}
	if _, err := dictionaryModule.TimezoneOptions(ctx); err != nil {
		return fmt.Errorf("initialize timezone dictionary: %w", err)
	}
	if _, err := dictionaryModule.UIPreferenceDefaults(ctx, dictionarymodule.UIPreferenceClient); err != nil {
		return fmt.Errorf("initialize UI preference dictionary: %w", err)
	}
	a.dictionaryModule = dictionaryModule
	a.localization = localizer
	return nil
}

func (a *Application) wireBusinessModules() {
	// Build in dependency order; generatedModules registers these same instances.
	// The dictionary is initialized earlier because captcha construction needs it.
	a.actionModule = actionmodule.New(a.db, a.pagination, a.localization)
	a.authModule = authmodule.New(a.db, a.authenticator, a.credentials, a.loginSessions, a.loginCaptcha, a.sliderCaptcha, a.passwordGuard, a.localization, a.actionModule, a.verification, a.authPath)
	a.authModule.SetEmailCodes(a.emailCodes)
	a.authModule.SetUIPreferenceSource(a.dictionaryModule)
	a.authModule.SetTimezoneSource(a.dictionaryModule)
	a.authModule.SetPasswordResetRequired(!a.passwordResetOptional)
	a.authModule.SetOnboardingVisibility(a.onboardingVisibility)
	if a.authenticator != nil {
		a.authenticator.SetHTTPSecurityPolicy(a.authModule.CheckHTTPSecurity)
	}
	a.auditModule = auditmodule.New(a.db, a.pagination, a.auditSettings)
	a.auditModule.SetAuthHTTPPath(a.authPath)
	if a.dictionaryModule != nil {
		a.actionModule.SetNameLocalization(a.dictionaryModule)
		a.actionModule.SetDefinitionCatalog(a.dictionaryModule)
		a.dictionaryModule.SetPermissionGroups(a.actionModule)
		a.dictionaryModule.SetResourcePermissions(a.actionModule)
	}
	a.roleModule = rolemodule.New(a.db, a.pagination, a.actionModule)
	a.userModule = usermodule.New(a.db, a.pagination, a.credentials, a.passwordGuard, a.actionModule, a.roleModule)
	a.groupModule = groupmodule.New(a.db, a.pagination, a.actionModule, a.userModule)
	a.actionModule.SetPermissionAdministration(a.authModule)
	if a.dictionaryModule != nil {
		a.dictionaryModule.SetPermissionAdministration(a.authModule)
	}
	a.userModule.SetPermissionAdministration(a.authModule)
	a.roleModule.SetPermissionAdministration(a.authModule)
	a.groupModule.SetPermissionAdministration(a.authModule)
	a.msgModule = msgmodule.New(a.db, a.pagination)
	a.mailTemplateModule = mailtemplatemodule.New(a.db, a.pagination)
	if a.emailCodes != nil {
		a.mailTemplateModule.SetTestSender(a.emailCodes)
		a.emailCodes.SetEmailTemplates(a.mailTemplateModule)
	}
}
