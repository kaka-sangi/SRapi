package service

import "errors"

var (
	ErrInvalidInput  = errors.New("invalid model input")
	ErrModelNotFound = errors.New("model not found")
	ErrModelExists   = errors.New("model already exists")
	ErrAliasExists   = errors.New("model alias already exists")
	ErrMappingExists = errors.New("model provider mapping already exists")
)
