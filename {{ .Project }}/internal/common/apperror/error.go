// Package apperror defines language-independent errors safe to expose to clients.
package apperror

import "errors"

type Error struct {
	code    string
	message string
}

func New(code, message string) *Error { return &Error{code: code, message: message} }

func (e *Error) Error() string { return e.message }

// Public unwraps expected errors; unexpected failures never expose internal details.
func Public(err error) *Error {
	var public *Error
	if errors.As(err, &public) && public != nil {
		return public
	}
	return Internal
}

func Code(err error) string { return Public(err).code }

var (
	KeywordQuery       = New("HTTP_INVALID_KEYWORD_QUERY", "keyword must be one value")
	InvalidBody        = New("HTTP_INVALID_BODY", "invalid request body")
	InvalidQuery       = New("HTTP_INVALID_QUERY", "invalid query parameters")
	UsernameQuery      = New("HTTP_INVALID_USERNAME_QUERY", "username must be one value")
	PublicPurpose      = New("HTTP_INVALID_PUBLIC_PURPOSE", "purpose must be login or register")
	PasswordPurpose    = New("HTTP_INVALID_PASSWORD_PURPOSE", "purpose must be password_change or password_reset")
	CaptchaExpired     = New("HTTP_CAPTCHA_EXPIRED", "point selection challenge expired")
	Page               = New("HTTP_INVALID_PAGE", "p must be one int32 value")
	PageSize           = New("HTTP_INVALID_PAGE_SIZE", "s must be one int32 value")
	UserStatus         = New("HTTP_INVALID_USER_STATUS", "status must be comma-separated values from 0 to 2")
	CategoryType       = New("HTTP_INVALID_CATEGORY_FORMAT", "category must be one int16 value")
	Category           = New("HTTP_INVALID_CATEGORY_VALUE", "category must be 0 or 1")
	IdempotencyKey     = New("HTTP_INVALID_IDEMPOTENCY_KEY", "invalid idempotency key")
	DuplicateRequest   = New("HTTP_DUPLICATE_REQUEST", "idempotency key has already been used")
	Internal           = New("HTTP_INTERNAL", "internal server error")
	BadRequest         = New("HTTP_BAD_REQUEST", "Bad Request")
	Unauthorized       = New("HTTP_UNAUTHORIZED", "Unauthorized")
	Forbidden          = New("HTTP_FORBIDDEN", "Forbidden")
	NotFound           = New("HTTP_NOT_FOUND", "Not Found")
	MethodNotAllowed   = New("HTTP_METHOD_NOT_ALLOWED", "Method Not Allowed")
	GatewayTimeout     = New("HTTP_GATEWAY_TIMEOUT", "Gateway Timeout")
	ServiceUnavailable = New("HTTP_SERVICE_UNAVAILABLE", "Service Unavailable")
	FeatureDisabled     = New("SYSTEM_FEATURE_DISABLED", "this feature is disabled by the system")
	ActionID           = New("HTTP_INVALID_ACTION_ID", "invalid action id")
	RoleID             = New("HTTP_INVALID_ROLE_ID", "invalid role id")
	UserID             = New("HTTP_INVALID_USER_ID", "invalid user id")
	UserGroupID        = New("HTTP_INVALID_USER_GROUP_ID", "invalid user group id")
	WhitelistID        = New("HTTP_INVALID_WHITELIST_ID", "invalid whitelist id")
	DictionaryID       = New("HTTP_INVALID_DICTIONARY_ID", "invalid dictionary id")
	DictionaryEnabled  = New("HTTP_INVALID_DICTIONARY_ENABLED", "enabled must be one boolean value")
)
