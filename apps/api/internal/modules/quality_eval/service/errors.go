package service

import "errors"

var (
	ErrInvalidInput = errors.New("invalid quality evaluation input")
	ErrUnavailable  = errors.New("quality evaluation unavailable")
)
