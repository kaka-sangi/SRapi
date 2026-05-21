package service

import "errors"

var (
	ErrInvalidInput           = errors.New("invalid auth input")
	ErrSessionNotFound        = errors.New("session not found")
	ErrSessionExpired         = errors.New("session expired")
	ErrSessionUserUnavailable = errors.New("session user unavailable")
	ErrCSRFTokenInvalid       = errors.New("csrf token invalid")
)
