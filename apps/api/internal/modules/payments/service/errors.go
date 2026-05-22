package service

import "errors"

var (
	ErrInvalidInput        = errors.New("invalid payment input")
	ErrInvalidTransition   = errors.New("invalid payment order transition")
	ErrProviderUnavailable = errors.New("payment provider unavailable")
	ErrSignatureInvalid    = errors.New("payment webhook signature invalid")
	ErrOrderMismatch       = errors.New("payment order mismatch")
)
