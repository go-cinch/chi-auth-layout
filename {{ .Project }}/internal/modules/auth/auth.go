package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
	"{{ .Computed.module_name_final }}/internal/common/apperror"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/code"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var (
	ErrInvalid               = apperror.New("AUTH_INVALID", "invalid login")
	ErrLoginFailed           = apperror.New("AUTH_LOGIN_FAILED", "invalid username or password")
	ErrPendingApproval       = apperror.New("AUTH_PENDING_APPROVAL", "user registration is pending approval")
	ErrLocked                = apperror.New("AUTH_LOCKED", "user is locked")
	ErrPointCaptchaRequired  = apperror.New("AUTH_POINT_CAPTCHA_REQUIRED", "invalid captcha")
	ErrUnauthorized          = apperror.New("AUTH_UNAUTHORIZED", "unauthorized")
	ErrInvalidRegistration   = apperror.New("AUTH_INVALID_REGISTRATION", "invalid registration")
	ErrUsernameExists        = apperror.New("AUTH_USERNAME_EXISTS", "username already exists")
	ErrInvalidPasswordChange = apperror.New("AUTH_INVALID_PASSWORD_CHANGE", "invalid password change")
	ErrIncorrectPassword     = apperror.New("AUTH_INCORRECT_PASSWORD", "current password is incorrect")
	ErrSamePassword          = apperror.New("AUTH_SAME_PASSWORD", "new password must differ from current password")
	ErrPasswordChangeLocked  = apperror.New("AUTH_PASSWORD_CHANGE_LOCKED", "self-service password change is locked; please contact an administrator")
)

const PasswordChangeFailuresMetadataKey = "password_change_failures"

const (
	SuperUserID   int64 = 1
	SuperUsername       = "super"
)

type Switches struct {
	PasswordResetRequired      bool
	ProtectSuper               bool
	ProtectCaptchaDictionaries bool
	EnableE2ETest              bool
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{4,49}$`)

type LoginInput struct {
	Username      string         `json:"username" example:"super"`
	Password      string         `json:"password" example:"super"`
	RememberMe    bool           `json:"remember_me"`
	CaptchaID     string         `json:"captcha_id,omitempty"`
	CaptchaPoints []CaptchaPoint `json:"captcha_points,omitempty"`
	SliderProof   string         `json:"slider_proof,omitempty"`
}

type LoginResult struct {
	PasswordResetRequired bool   `json:"password_reset_required"`
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

type RegisterInput struct {
	Username    string `json:"username" example:"operator"`
	Password    string `json:"password" example:"change-me"`
	SliderProof string `json:"slider_proof,omitempty"`
}

type PasswordChangeInput struct {
	OldPassword   string         `json:"old_password" example:"current-password"`
	NewPassword   string         `json:"new_password" example:"new-password"`
	CaptchaID     string         `json:"captcha_id,omitempty"`
	CaptchaPoints []CaptchaPoint `json:"captcha_points,omitempty"`
}

type UsernameAvailability struct {
	Available bool `json:"available"`
}

type UserRole struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Word string `json:"word"`
}

type UserInfo struct {
	ID         int64      `json:"id"`
	Username   string     `json:"username"`
	Code       string     `json:"code"`
	Role       *UserRole  `json:"role,omitempty"`
	Permission Permission `json:"permission"`
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

type passwordChangeFailure struct {
	cause           error
	captchaRequired bool
}

func (e *passwordChangeFailure) Error() string { return e.cause.Error() }

func (e *passwordChangeFailure) Unwrap() error { return e.cause }

func requiresPasswordChangeCaptcha(err error) bool {
	var failure *passwordChangeFailure
	return errors.As(err, &failure) && failure.captchaRequired
}

type Module struct {
	store         *db.Store
	authenticator *authn.Manager
	credentials   *Credentials
	sessions      *Sessions
	captcha       *PointCaptcha
	sliderCaptcha *SliderCaptcha
	passwordGuard *PasswordChangeGuard
	switches      Switches
}

func New(store *db.Store, authenticator *authn.Manager, credentials *Credentials, sessions *Sessions, captcha *PointCaptcha, sliderCaptcha *SliderCaptcha, passwordGuard *PasswordChangeGuard, switches Switches) *Module {
	return &Module{store: store, authenticator: authenticator, credentials: credentials, sessions: sessions, captcha: captcha, sliderCaptcha: sliderCaptcha, passwordGuard: passwordGuard, switches: switches}
}

func (m *Module) Register(ctx context.Context, input RegisterInput) error {
	username := strings.TrimSpace(input.Username)
	passwordLength := len([]byte(input.Password))
	if username != input.Username || !usernamePattern.MatchString(username) || passwordLength < 6 || passwordLength > 72 {
		return ErrInvalidRegistration
	}
	input.Username = username
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash registration password: %w", err)
	}
	var id int64
	if err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT nextval('t_user_id_seq')`).Scan(&id); err != nil {
		return fmt.Errorf("allocate registered user id: %w", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	for range 5 {
		userCode, err := code.Generate()
		if err != nil {
			return fmt.Errorf("create registered user code: %w", err)
		}
		result, err := m.store.SQL(ctx).ExecContext(ctx, `INSERT INTO t_user (id, created_at, updated_at, role_id, action, username, code, password, status, wrong) VALUES ($1, $2, $3, NULL, '', $4, $5, $6, 0, 0) ON CONFLICT ON CONSTRAINT uk_user_code DO NOTHING`,
			id, now, now, input.Username, userCode, string(passwordHash))
		if err != nil {
			var postgresError *pq.Error
			if errors.As(err, &postgresError) && postgresError.Code == "23505" && postgresError.Constraint == "uk_user_username" {
				return ErrUsernameExists
			}
			return fmt.Errorf("create registered user: %w", err)
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("create registered user result: %w", err)
		}
		if inserted == 1 {
			return nil
		}
	}
	return errors.New("create registered user: generate unique code")
}

func (m *Module) UsernameAvailability(ctx context.Context, username string) (*UsernameAvailability, error) {
	normalized := strings.TrimSpace(username)
	if normalized != username || !usernamePattern.MatchString(normalized) {
		return nil, ErrInvalidRegistration
	}
	username = normalized
	var exists bool
	if err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM t_user WHERE username = $1)`, username).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check username availability: %w", err)
	}
	return &UsernameAvailability{Available: !exists}, nil
}

func (m *Module) PasswordChangeChallenge(ctx context.Context) (*CredentialChallenge, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	if m.switches.ProtectSuper && identity.UserID == SuperUserID {
		return nil, apperror.FeatureDisabled
	}
	return m.credentials.PasswordChangeChallenge(ctx, identity.UserID)
}

func (m *Module) OpenPasswordChange(ctx context.Context, encrypted EncryptedPasswordChangeInput) (PasswordChangeInput, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return PasswordChangeInput{}, ErrUnauthorized
	}
	if m.switches.ProtectSuper && identity.UserID == SuperUserID {
		return PasswordChangeInput{}, apperror.FeatureDisabled
	}
	return m.credentials.OpenPasswordChange(ctx, identity.UserID, encrypted)
}

func (m *Module) NewPasswordChangeCaptcha(ctx context.Context) (*PointCaptchaChallenge, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	return m.passwordGuard.NewChallenge(ctx, identity.UserID)
}

func (m *Module) RefreshPasswordChangeCaptcha(ctx context.Context, captchaID string) (*PointCaptchaChallenge, error) {
	if _, ok := authn.FromContext(ctx); !ok {
		return nil, ErrUnauthorized
	}
	return m.passwordGuard.RefreshChallenge(ctx, captchaID)
}

func (m *Module) VerifyPasswordChangeCaptcha(ctx context.Context, captchaID string, points []CaptchaPoint) (*PointCaptchaVerificationResult, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	verified, err := m.passwordGuard.Check(ctx, identity.UserID, captchaID, points)
	if err != nil {
		return nil, err
	}
	if verified {
		return &PointCaptchaVerificationResult{Verified: true}, nil
	}
	captcha, err := m.passwordGuard.NewChallenge(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}
	return &PointCaptchaVerificationResult{Captcha: captcha}, nil
}

func (m *Module) ChangePassword(ctx context.Context, input PasswordChangeInput) error {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if m.switches.ProtectSuper && identity.UserID == SuperUserID {
		return apperror.FeatureDisabled
	}
	if identity.PasswordResetRequired {
		return authn.ErrPasswordResetRequired
	}
	oldPasswordLength := len([]byte(input.OldPassword))
	newPasswordLength := len([]byte(input.NewPassword))
	if oldPasswordLength == 0 || oldPasswordLength > 72 || newPasswordLength < 6 || newPasswordLength > 72 {
		return ErrInvalidPasswordChange
	}
	locked, err := m.passwordChangeLocked(ctx, identity)
	if err != nil {
		return err
	}
	if locked {
		return ErrPasswordChangeLocked
	}
	failureCount, err := m.passwordGuard.FailureCount(ctx, identity.UserID)
	if err != nil {
		return err
	}
	if m.passwordGuard.LockRequired(failureCount) {
		if err := m.persistPasswordChangeLock(ctx, identity); err != nil {
			return err
		}
		return ErrPasswordChangeLocked
	}
	captchaRequired := m.passwordGuard.CaptchaRequired(failureCount)
	if captchaRequired {
		verified, err := m.passwordGuard.Verify(ctx, identity.UserID, input.CaptchaID, input.CaptchaPoints)
		if err != nil {
			return err
		}
		if !verified {
			return &passwordChangeFailure{cause: ErrPointCaptchaRequired, captchaRequired: true}
		}
	}
	err = m.store.Tx(ctx, func(ctx context.Context) error {
		var currentPasswordHash string
		var credentialVersion int64
		err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT password, credential_version FROM t_user WHERE id = $1 AND code = $2 FOR UPDATE`, identity.UserID, identity.Code).Scan(&currentPasswordHash, &credentialVersion)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnauthorized
		}
		if err != nil {
			return fmt.Errorf("query current password: %w", err)
		}
		if credentialVersion != identity.CredentialVersion || credentialVersion <= 0 {
			return ErrUnauthorized
		}
		if bcrypt.CompareHashAndPassword([]byte(currentPasswordHash), []byte(input.OldPassword)) != nil {
			return ErrIncorrectPassword
		}
		if bcrypt.CompareHashAndPassword([]byte(currentPasswordHash), []byte(input.NewPassword)) == nil {
			return ErrSamePassword
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash changed password: %w", err)
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		result, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user SET password = $1, credential_version = credential_version + 1, wrong = 0, updated_at = $2 WHERE id = $3 AND code = $4`, string(passwordHash), now, identity.UserID, identity.Code)
		if err != nil {
			return fmt.Errorf("update current password: %w", err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("update current password result: %w", err)
		}
		if updated != 1 {
			return ErrUnauthorized
		}
		return nil
	})
	if errors.Is(err, ErrIncorrectPassword) {
		failureCount, guardErr := m.passwordGuard.RecordFailure(ctx, identity.UserID)
		if guardErr != nil {
			return guardErr
		}
		if m.passwordGuard.LockRequired(failureCount) {
			if err := m.persistPasswordChangeLock(ctx, identity); err != nil {
				return err
			}
			return ErrPasswordChangeLocked
		}
		return &passwordChangeFailure{cause: ErrIncorrectPassword, captchaRequired: m.passwordGuard.CaptchaRequired(failureCount)}
	}
	if errors.Is(err, ErrSamePassword) && captchaRequired {
		return &passwordChangeFailure{cause: ErrSamePassword, captchaRequired: true}
	}
	if err != nil {
		return err
	}
	if err := m.passwordGuard.Clear(ctx, identity.UserID); err != nil {
		slog.WarnContext(ctx, "clear password change failures failed: "+err.Error())
	}
	return nil
}

func (m *Module) passwordChangeLocked(ctx context.Context, identity authn.Identity) (bool, error) {
	const query = `SELECT CASE
		WHEN metadata ? 'password_change_failures' AND jsonb_typeof(metadata -> 'password_change_failures') = 'number'
		THEN (metadata ->> 'password_change_failures')::numeric >= $3
		ELSE false
	END FROM t_user WHERE id = $1 AND code = $2`
	var locked bool
	err := m.store.SQL(ctx).QueryRowContext(ctx, query, identity.UserID, identity.Code, m.passwordGuard.LockThreshold()).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrUnauthorized
	}
	if err != nil {
		return false, fmt.Errorf("query password change lock: %w", err)
	}
	return locked, nil
}

func (m *Module) persistPasswordChangeLock(ctx context.Context, identity authn.Identity) error {
	now := time.Now().UTC().Truncate(time.Microsecond)
	result, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user SET metadata = jsonb_set(metadata, '{password_change_failures}', to_jsonb($1::bigint), true), updated_at = $2 WHERE id = $3 AND code = $4`,
		m.passwordGuard.LockThreshold(), now, identity.UserID, identity.Code)
	if err != nil {
		return fmt.Errorf("persist password change lock: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("persist password change lock result: %w", err)
	}
	if updated != 1 {
		return ErrUnauthorized
	}
	if err := m.passwordGuard.Clear(ctx, identity.UserID); err != nil {
		slog.WarnContext(ctx, "clear locked password change failures failed: "+err.Error())
	}
	return nil
}

func (m *Module) LoginVerification(ctx context.Context, username string) (*LoginVerificationResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || utf8.RuneCountInString(username) > 50 {
		return nil, ErrInvalid
	}
	var wrong int64
	err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT wrong FROM t_user WHERE username = $1`, username).Scan(&wrong)
	if errors.Is(err, sql.ErrNoRows) {
		return &LoginVerificationResult{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query login verification: %w", err)
	}
	if !m.captcha.Required(wrong) {
		return &LoginVerificationResult{}, nil
	}
	captcha, err := m.captcha.NewChallenge(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("create login verification captcha: %w", err)
	}
	return &LoginVerificationResult{CaptchaRequired: true, Captcha: captcha}, nil
}

func (m *Module) VerifyLoginCaptcha(ctx context.Context, username, captchaID string, points []CaptchaPoint) (*PointCaptchaVerificationResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || utf8.RuneCountInString(username) > 50 {
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
	if input.Username == "" || input.Password == "" {
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
	if bcrypt.CompareHashAndPassword([]byte(user.passwordHash), []byte(input.Password)) != nil {
		wrong, err := m.recordWrongPassword(ctx, user.identity.UserID)
		if err != nil {
			return nil, err
		}
		return nil, &loginFailure{cause: ErrLoginFailed, captchaRequired: m.captcha.Required(wrong)}
	}
	result, sessionID, err := m.issueLoginResult(ctx, user.identity, input.RememberMe)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	var loginCount int64
	query := `UPDATE t_user SET last_logged_in_at = CASE WHEN login_count > 0 THEN $1 ELSE last_logged_in_at END, login_count = CASE WHEN login_count > 0 THEN login_count + 1 ELSE 0 END, wrong = 0, updated_at = $1 WHERE id = $2 AND credential_version = $3 AND status = $4 RETURNING login_count`
	if !m.switches.PasswordResetRequired {
		query = `UPDATE t_user SET last_logged_in_at = $1, login_count = login_count + 1, wrong = 0, updated_at = $1 WHERE id = $2 AND credential_version = $3 AND status = $4 RETURNING login_count`
	}
	err = m.store.SQL(ctx).QueryRowContext(ctx, query, now, user.identity.UserID, user.identity.CredentialVersion, userStatusActive).Scan(&loginCount)
	if err != nil {
		_ = m.sessions.Revoke(context.WithoutCancel(ctx), sessionID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("update user login: %w", err)
	}
	result.PasswordResetRequired = m.switches.PasswordResetRequired && loginCount == 0
	return result, nil
}

func (m *Module) Refresh(ctx context.Context, refreshToken string) (*LoginResult, error) {
	session, err := m.sessions.ConsumeRefresh(ctx, refreshToken)
	if errors.Is(err, ErrSessionNotFound) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	identity, err := m.refreshIdentity(ctx, session.identity())
	if err != nil {
		_ = m.sessions.Revoke(ctx, session.SessionID)
		return nil, err
	}
	refresh, err := m.sessions.Renew(ctx, session, identity)
	if err != nil {
		_ = m.sessions.Revoke(ctx, session.SessionID)
		return nil, fmt.Errorf("renew login session: %w", err)
	}
	identity.SessionID = refresh.SessionID
	token, expiredAt, err := m.authenticator.Issue(identity)
	if err != nil {
		_ = m.sessions.Revoke(ctx, refresh.SessionID)
		return nil, fmt.Errorf("issue refreshed login token: %w", err)
	}
	return &LoginResult{AccessToken: token, RefreshToken: refresh.RefreshToken, ExpiredAt: expiredAt.UnixMilli(), PasswordResetRequired: identity.PasswordResetRequired}, nil
}

func (m *Module) refreshIdentity(ctx context.Context, identity authn.Identity) (authn.Identity, error) {
	current, status, err := currentCredentialIdentity(ctx, m.store, identity, m.switches.PasswordResetRequired)
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
	now := time.Now().UTC().Truncate(time.Microsecond)
	result, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user SET status = $1, metadata = metadata - 'lock_expired_at', updated_at = $2 WHERE id = $3 AND status = $4 AND metadata ? 'lock_expired_at' AND metadata ->> 'lock_expired_at' ~ '^[0-9]+$' AND (metadata ->> 'lock_expired_at')::numeric > 0 AND (metadata ->> 'lock_expired_at')::numeric <= $5`,
		userStatusActive, now, userID, userStatusLocked, now.UnixMilli())
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
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query, time.Now().UTC().Truncate(time.Microsecond), userID).Scan(&wrong); err != nil {
		return 0, fmt.Errorf("record wrong password: %w", err)
	}
	return wrong, nil
}

func (m *Module) Info(ctx context.Context) (*UserInfo, error) {
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
	permission, err := m.permissions(ctx, value.ID)
	if err != nil {
		return nil, err
	}
	value.Permission = permission
	return &value, nil
}

func (m *Module) Logout(ctx context.Context, refreshToken string) error {
	return m.sessions.RevokeRefresh(ctx, refreshToken)
}
