package service

import "errors"

var (
	ErrInvalidInput            = errors.New("invalid subscription input")
	ErrDuplicateSubscription   = errors.New("active subscription for this plan already exists")
)
