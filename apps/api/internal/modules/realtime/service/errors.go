package service

import "errors"

var (
	ErrInvalidInput  = errors.New("invalid realtime slot input")
	ErrLimitExceeded = errors.New("realtime slot limit exceeded")
	ErrSlotNotFound  = errors.New("realtime slot not found")
)
