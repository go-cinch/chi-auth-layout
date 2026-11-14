package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

var (
	ErrInvalid              = apperror.New("AUTH_INVALID", "invalid login")
	ErrLoginFailed          = apperror.New("AUTH_LOGIN_FAILED", "invalid username or password")
	ErrPendingApproval      = apperror.New("AUTH_PENDING_APPROVAL", "user registration is pending approval")
	ErrLocked               = apperror.New("AUTH_LOCKED", "user is locked")
	ErrPointCaptchaRequired = apperror.New("AUTH_POINT_CAPTCHA_REQUIRED", "invalid captcha")
	ErrInfoScopeRequired    = apperror.New("AUTH_INFO_SCOPE_REQUIRED", "please specify a permission scope")
	ErrInfoScopeInvalid     = apperror.New("AUTH_INFO_SCOPE_INVALID", "invalid permission scope")
	ErrUnauthorized         = apperror.New("AUTH_UNAUTHORIZED", "unauthorized")
)

type LoginInput struct {
	Method           string         `json:"method,omitempty"`
	BrowserUUID      string         `json:"browser_uuid,omitempty"`
	VerificationID   string         `json:"verification_id,omitempty"`
	VerificationCode string         `json:"verification_code,omitempty"`
	Username         string         `json:"username" example:"super"`
	Password         string         `json:"password" example:"super"`
	RememberMe       bool           `json:"remember_me"`
	CaptchaID        string         `json:"captcha_id,omitempty"`
	CaptchaPoints    []CaptchaPoint `json:"captcha_points,omitempty"`
	SliderProof      string         `json:"slider_proof,omitempty"`
}

type LoginResult struct {
	MFARequired           bool   `json:"-"`
	MFATicket             string `json:"-"`
	TOTPEnrollmentRequired bool   `json:"-"`
	PasswordResetRequired bool   `json:"-"`
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	ExpiredAt             int64  `json:"expired_at"`
}

type LoginVerificationResult struct {
	CaptchaRequired bool                   `json:"captcha_required"`
	Captcha         *PointCaptchaChallenge `json:"captcha,omitempty"`
}

type PointCaptchaVerificationResult struct {
	Verified bool                   `json:"verified"`
	Captcha  *PointCaptchaChallenge `json:"captcha,omitempty"`
}

type UserRole struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Word string `json:"word"`
}

type SelfMetadata struct {
	Language             string `json:"language"`
	Timezone             string `json:"timezone"`
	EmailBindingSkipped  bool   `json:"email_binding_skipped"`
	TOTPEnrollmentSkipped bool   `json:"totp_enrollment_skipped"`
	PasswordResetSkipped bool   `json:"password_reset_skipped"`
}

type TOTPInfo struct {
	Bound bool `json:"bound"`
}

type UserInfo struct {
	UIPreferences        map[string]dictionary.UIPreferences `json:"ui_preferences,omitempty"`
	TOTP                  *TOTPInfo                            `json:"totp,omitempty"`
	Metadata             *SelfMetadata                       `json:"metadata,omitempty"`
	Email                string                              `json:"email"`
	EmailVerifiedAt      *int64                              `json:"email_verified_at"`
	EmailBindingRequired bool                                `json:"email_binding_required"`
	selectedFields       []string
	Scope                string     `json:"scope"`
	ID                   int64      `json:"id"`
	Username             string     `json:"username"`
	Code                 string     `json:"code"`
	Role                 *UserRole  `json:"role,omitempty"`
	Permission           Permission `json:"permission"`
}

type loginUser struct {
	identity     authn.Identity
	passwordHash string
	status       int16
	wrong        int64
}

const (
	userStatusPending int16 = iota
	userStatusActive
	userStatusLocked
)

type loginFailure struct {
	cause           error
	captchaRequired bool
}

func (e *loginFailure) Error() string { return e.cause.Error() }

func (e *loginFailure) Unwrap() error { return e.cause }

func requiresPointCaptcha(err error) bool {
	var failure *loginFailure
	return errors.As(err, &failure) && failure.captchaRequired
}

type CatalogSource interface {
	PublicCatalog(...string) localization.PublicCatalog
	Current() localization.Catalog
	Select([]string) localization.Catalog
}

type Module struct {
	uiPreferenceSource     UIPreferenceSource
	timezoneSource         TimezoneSource
	passwordResetOptional  bool
	onboardingVisibility   OnboardingVisibility
	emailCodes             *EmailCodes
	verification                    *Verification
	httpPath               string
	defaultUserPermissions DefaultUserPermissions
	catalogSource          CatalogSource
	permissionRoutes       []modules.HTTPPermissionRoute
	store                  *db.Store
	authenticator          *authn.Manager
	credentials            *Credentials
	sessions               *Sessions
	captcha                *PointCaptcha
	sliderCaptcha          *SliderCaptcha
	passwordGuard          *PasswordChangeGuard
}

func New(store *db.Store, authenticator *authn.Manager, credentials *Credentials, sessions *Sessions, captcha *PointCaptcha, sliderCaptcha *SliderCaptcha, passwordGuard *PasswordChangeGuard, catalog CatalogSource, defaults DefaultUserPermissions, verification *Verification, paths ...string) *Module {
	if verification == nil {
		panic("auth: verification dependency is required; provide an explicitly disabled Verification when not enforcing it")
	}
	prefix := ""
	if len(paths) > 0 {
		prefix = paths[0]
	}
	if prefix == "/" {
		prefix = ""
	}
	return &Module{httpPath: prefix, defaultUserPermissions: defaults, catalogSource: catalog, store: store, authenticator: authenticator, credentials: credentials, sessions: sessions, captcha: captcha, sliderCaptcha: sliderCaptcha, passwordGuard: passwordGuard, verification: verification}
}

func (m *Module) loginCaptchaRequired(ctx context.Context, username string) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, ErrInvalid
	}
	resolved, err := m.resolveLoginUsername(ctx, username)
	if errors.Is(err, ErrLoginFailed) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var wrong int64
	err = m.store.SQL(ctx).QueryRowContext(ctx, `SELECT wrong FROM t_user WHERE username = $1`, resolved).Scan(&wrong)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query login verification: %w", err)
	}
	return m.captcha.Required(wrong), nil
}

// LoginVerification is the legacy combined requirement/challenge operation.
// New clients read /pub/status, then explicitly request POST /pub/captcha.
func (m *Module) LoginVerification(ctx context.Context, username string) (*LoginVerificationResult, error) {
	required, err := m.loginCaptchaRequired(ctx, username)
	if err != nil {
		return nil, err
	}
	result := &LoginVerificationResult{CaptchaRequired: required}
	if required {
		result.Captcha, err = m.captcha.NewChallenge(ctx, strings.TrimSpace(username))
		if err != nil {
			return nil, fmt.Errorf("create login verification captcha: %w", err)
		}
	}
	return result, nil
}

func (m *Module) VerifyLoginCaptcha(ctx context.Context, username, captchaID string, points []CaptchaPoint) (*PointCaptchaVerificationResult, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, ErrInvalid
	}
	verified, err := m.captcha.Check(ctx, username, captchaID, points)
	if err != nil {
		return nil, err
	}
	if verified {
		return &PointCaptchaVerificationResult{Verified: true}, nil
	}
	captcha, err := m.captcha.NewChallenge(ctx, username)
	if err != nil {
		return nil, err
	}
	return &PointCaptchaVerificationResult{Captcha: captcha}, nil
}

func (m *Module) Login(ctx context.Context, input LoginInput) (*LoginResult, error) {
	input.Username = strings.TrimSpace(input.Username)
	if input.Method == "email_code" {
		return m.loginEmail(ctx, input)
	}
	if input.Method != "" && input.Method != "password" {
		return nil, ErrInvalid
	}
	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" || strings.TrimSpace(input.Password) == "" {
		return nil, ErrInvalid
	}
	user, err := m.loginUser(ctx, input.Username)
	if err != nil {
		return nil, err
	}
	if user.status == userStatusPending {
		return nil, ErrPendingApproval
	}
	if user.status == userStatusLocked {
		unlocked, err := m.unlockExpiredUser(ctx, user.identity.UserID)
		if err != nil {
			return nil, err
		}
		if !unlocked {
			return nil, ErrLocked
		}
		user.status = userStatusActive
	}
	if user.status != userStatusActive {
		return nil, ErrLoginFailed
	}
	if m.captcha.Required(user.wrong) {
		verified, err := m.captcha.Verify(ctx, input.Username, input.CaptchaID, input.CaptchaPoints)
		if err != nil {
			return nil, err
		}
		if !verified {
			return nil, &loginFailure{cause: ErrPointCaptchaRequired, captchaRequired: true}
		}
	} else {
		if m.sliderCaptcha == nil {
			return nil, errors.New("slider captcha is unavailable")
		}
		if err := m.sliderCaptcha.ConsumeProof(ctx, sliderCaptchaPurposeLogin, input.Username, input.SliderProof); err != nil {
			return nil, err
		}
	}
	if !VerifyPassword(user.passwordHash, input.Password) {
		wrong, err := m.recordWrongPassword(ctx, user.identity.UserID)
		if err != nil {
			return nil, err
		}
		return nil, &loginFailure{cause: ErrLoginFailed, captchaRequired: m.captcha.Required(wrong)}
	}
	return m.completeFirstFactorLogin(ctx, user.identity, input.RememberMe, authn.Password)
}

func (m *Module) completeFirstFactorLogin(ctx context.Context, identity authn.Identity, rememberMe bool, primary authn.Method) (*LoginResult, error) {
	identity.Methods = authn.Methods{Primary: primary}
	if !m.totpEnabled() {
		return m.finishLogin(ctx, identity, rememberMe, false)
	}
	bound, err := m.verification.hasTOTP(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}
	if bound {
		return m.beginMFALogin(ctx, identity, rememberMe)
	}
	return m.finishLogin(ctx, identity, rememberMe, true)
}

// The primary-factor or MFA continuation flow has already established enrollment state.
// Session issuance does not query or overwrite that business decision.
func (m *Module) finishLogin(ctx context.Context, identity authn.Identity, rememberMe, enrollmentRequired bool) (*LoginResult, error) {
	result, sessionID, err := m.issueLoginResult(ctx, identity, rememberMe)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	var loginCount int64
	query := `UPDATE t_user SET last_logged_in_at = CASE WHEN login_count > 0 THEN $1 ELSE last_logged_in_at END, login_count = CASE WHEN login_count > 0 THEN login_count + 1 ELSE 0 END, wrong = 0, updated_at = $1 WHERE id = $2 AND credential_version = $3 AND status = $4`
	// An email login may be waiting for MFA while the owner unlinks the address.
	if identity.Methods.Primary == authn.EmailCode {
		query += " AND email IS NOT NULL AND email_verified_at IS NOT NULL"
	}
	query += " RETURNING login_count"
	err = m.store.SQL(ctx).QueryRowContext(ctx, query, now, identity.UserID, identity.CredentialVersion, userStatusActive).Scan(&loginCount)
	if err != nil {
		_ = m.sessions.Revoke(context.WithoutCancel(ctx), sessionID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("update user login: %w", err)
	}
	auditctx.Identity(ctx, identity.UserID, identity.Username)
	result.PasswordResetRequired = loginCount == 0
	result.TOTPEnrollmentRequired = enrollmentRequired
	if enrollmentRequired && identity.Methods.Primary == authn.Password {
		if err := m.grantTOTPEnrollment(ctx, identity, sessionID); err != nil {
			_ = m.sessions.Revoke(context.WithoutCancel(ctx), sessionID)
			return nil, err
		}
	}
	return result, nil
}

func (m *Module) Refresh(ctx context.Context, refreshToken string) (*LoginResult, error) {
	session, err := m.sessions.ReadRefresh(ctx, refreshToken)
	if errors.Is(err, ErrSessionNotFound) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	identity, err := m.refreshIdentity(ctx, session.identity())
	if err != nil {
		// Infrastructure failures must not destroy a valid, retryable session.
		if errors.Is(err, ErrUnauthorized) {
			if revokeErr := m.sessions.Revoke(ctx, session.SessionID); revokeErr != nil {
				return nil, revokeErr
			}
		}
		return nil, err
	}
	required, err := m.totpEnrollmentRequired(ctx, identity)
	if err != nil {
		return nil, err
	}
	// Prepare the access token before committing the rotation. If signing fails,
	// the caller can still use its existing access and refresh credentials.
	token, expiredAt, err := m.authenticator.Issue(identity)
	if err != nil {
		return nil, fmt.Errorf("issue refreshed login token: %w", err)
	}
	refresh, err := m.sessions.Renew(ctx, session, identity)
	if errors.Is(err, ErrSessionNotFound) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, fmt.Errorf("renew login session: %w", err)
	}
	return &LoginResult{AccessToken: token, RefreshToken: refresh.RefreshToken, ExpiredAt: expiredAt.UnixMilli(), PasswordResetRequired: identity.PasswordResetRequired, TOTPEnrollmentRequired: required}, nil
}

func (m *Module) refreshIdentity(ctx context.Context, identity authn.Identity) (authn.Identity, error) {
	current, status, err := currentCredentialIdentity(ctx, m.store, identity)
	if err != nil {
		return authn.Identity{}, err
	}
	if status == userStatusLocked {
		unlocked, unlockErr := m.unlockExpiredUser(ctx, identity.UserID)
		if unlockErr != nil {
			return authn.Identity{}, unlockErr
		}
		if unlocked {
			return current, nil
		}
	}
	if status != userStatusActive {
		return authn.Identity{}, ErrUnauthorized
	}
	return current, nil
}

func (m *Module) loginUser(ctx context.Context, username string) (*loginUser, error) {
	username, resolveErr := m.resolveLoginUsername(ctx, username)
	if resolveErr != nil {
		return nil, resolveErr
	}
	const query = `SELECT id, username, code, password, status, wrong, credential_version FROM t_user WHERE username = $1`
	var user loginUser
	err := m.store.SQL(ctx).QueryRowContext(ctx, query, username).Scan(
		&user.identity.UserID,
		&user.identity.Username,
		&user.identity.Code,
		&user.passwordHash,
		&user.status,
		&user.wrong,
		&user.identity.CredentialVersion,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrLoginFailed
	}
	if err != nil {
		return nil, fmt.Errorf("query login user: %w", err)
	}
	return &user, nil
}

func (m *Module) unlockExpiredUser(ctx context.Context, userID int64) (bool, error) {
	now := time.Now().UnixMilli()
	result, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user SET status = $1, metadata = metadata - 'lock_expired_at', updated_at = $2 WHERE id = $3 AND status = $4 AND metadata ? 'lock_expired_at' AND metadata ->> 'lock_expired_at' ~ '^[0-9]+$' AND (metadata ->> 'lock_expired_at')::numeric > 0 AND (metadata ->> 'lock_expired_at')::numeric <= $5`,
		userStatusActive, now, userID, userStatusLocked, now)
	if err != nil {
		return false, fmt.Errorf("unlock expired user: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("unlock expired user result: %w", err)
	}
	return count == 1, nil
}

func (m *Module) recordWrongPassword(ctx context.Context, userID int64) (int64, error) {
	const query = `UPDATE t_user SET wrong = wrong + 1, updated_at = $1 WHERE id = $2 RETURNING wrong`
	var wrong int64
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query, time.Now().UnixMilli(), userID).Scan(&wrong); err != nil {
		return 0, fmt.Errorf("record wrong password: %w", err)
	}
	return wrong, nil
}

func (m *Module) Info(ctx context.Context, scopes ...string) (*UserInfo, error) {
	scope := ""
	if len(scopes) > 0 {
		scope = strings.TrimSpace(scopes[0])
	}
	if scope == "" {
		return nil, ErrInfoScopeRequired
	}
	if !action.ValidScope(scope) {
		return nil, ErrInfoScopeInvalid
	}
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	const query = `SELECT u.id, u.username, u.code, r.id, r.name, r.word
		FROM t_user AS u
		LEFT JOIN t_role AS r ON r.id = u.role_id
		WHERE u.id = $1 AND u.code = $2`
	var value UserInfo
	var roleID sql.NullInt64
	var roleName, roleWord sql.NullString
	err := m.store.SQL(ctx).QueryRowContext(ctx, query, identity.UserID, identity.Code).Scan(
		&value.ID, &value.Username, &value.Code, &roleID, &roleName, &roleWord,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, fmt.Errorf("query current user: %w", err)
	}
	if roleID.Valid {
		value.Role = &UserRole{ID: roleID.Int64, Name: roleName.String, Word: roleWord.String}
	}
	catalog := m.catalogSource.Select([]string{"resource_catalog"})
	permission, err := m.permissionsFromCatalog(ctx, value.ID, scope, nil, catalog)
	if err != nil {
		return nil, err
	}
	value.Scope = scope
	permission.Navigation = permissionNavigation(catalog.ResourceCatalog, scope, permission.Menus)
	value.Permission = permission
	if err := m.populateEmailInfo(ctx, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (m *Module) Logout(ctx context.Context, refreshToken string) error {
	// Authenticate the token itself, not the stored record: older generations and
	// repeated logout must still be able to finish a previously failed cleanup.
	sessionID, _, err := m.sessions.parseRefresh(refreshToken)
	if errors.Is(err, ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// Revoke first so no new request can use this grant. Attempt both operations
	// even when one fails; a retry with the same signed token can finish either.
	revokeErr := m.sessions.Revoke(ctx, sessionID)
	return errors.Join(revokeErr, m.clearSensitiveProof(ctx, sessionID))
}
