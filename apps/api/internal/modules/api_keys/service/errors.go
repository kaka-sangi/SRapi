package service

import "errors"

var (
	ErrInvalidInput      = errors.New("invalid api key input")
	ErrInvalidKey        = errors.New("invalid api key")
	ErrKeyDisabled       = errors.New("api key disabled")
	ErrKeyExpired        = errors.New("api key expired")
	ErrKeyNotFound       = errors.New("api key not found")
	ErrPepperUnavailable = errors.New("api key pepper unavailable")
)
