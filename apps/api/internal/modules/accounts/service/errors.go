package service

import "errors"

var (
	ErrInvalidInput      = errors.New("invalid account input")
	ErrAccountNotFound   = errors.New("account not found")
	ErrAccountExists     = errors.New("account already exists")
	ErrCredentialMissing = errors.New("account credential missing")
	ErrEncryptionFailed  = errors.New("account credential encryption failed")
	ErrProxyUnavailable  = errors.New("account proxy unavailable")
)
