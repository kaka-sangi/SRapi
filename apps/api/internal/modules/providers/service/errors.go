package service

import "errors"

var (
	ErrInvalidInput     = errors.New("invalid provider input")
	ErrProviderNotFound = errors.New("provider not found")
	ErrProviderExists   = errors.New("provider already exists")
)
