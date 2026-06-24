package service

import (
	"errors"

	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
)

var (
	ErrInvalidInput          = contract.ErrInvalidInput
	ErrInvalidTransition     = errors.New("invalid payment order transition")
	ErrProviderUnavailable   = errors.New("payment provider unavailable")
	ErrProviderConfigInvalid = errors.New("payment provider config invalid")
	ErrSignatureInvalid      = errors.New("payment webhook signature invalid")
	ErrOrderMismatch         = errors.New("payment order mismatch")
	ErrTooManyPendingOrders  = errors.New("too many pending payment orders")
)
