package service

import "errors"

var (
	ErrInvalidInput                 = errors.New("invalid auth input")
	ErrSessionNotFound              = errors.New("session not found")
	ErrSessionExpired               = errors.New("session expired")
	ErrSessionUserUnavailable       = errors.New("session user unavailable")
	ErrCSRFTokenInvalid             = errors.New("csrf token invalid")
	ErrSecondFactorRequired         = errors.New("second factor required")
	ErrSecondFactorInvalid          = errors.New("second factor invalid")
	ErrPasswordResetInvalid         = errors.New("password reset token invalid")
	ErrPasswordResetUnavailable     = errors.New("password reset unavailable")
	ErrEmailVerificationInvalid     = errors.New("email verification token invalid")
	ErrEmailVerificationUnavailable = errors.New("email verification unavailable")
	ErrPendingOAuthInvalid          = errors.New("pending oauth session invalid")
	ErrPendingOAuthEmailInvalid     = errors.New("pending oauth email verification invalid")
	ErrPendingOAuthTargetMismatch   = errors.New("pending oauth target mismatch")
	ErrPendingOAuthUnavailable      = errors.New("pending oauth unavailable")
	ErrOAuthUnavailable             = errors.New("oauth unavailable")
)
