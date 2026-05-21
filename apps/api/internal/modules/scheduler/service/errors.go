package service

import "errors"

var (
	ErrInvalidInput            = errors.New("invalid scheduler input")
	ErrNoAvailableAccount      = errors.New("no available account")
	ErrUserBalanceInsufficient = errors.New("user balance insufficient")
)
