package service

import (
	"errors"

	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
)

var (
	ErrInvalidInput        = contract.ErrInvalidInput
	ErrInvalidTransition   = errors.New("invalid payment order transition")
	ErrProviderUnavailable = errors.New("payment provider unavailable")
	ErrSignatureInvalid    = errors.New("payment webhook signature invalid")
	ErrOrderMismatch       = errors.New("payment order mismatch")
)
