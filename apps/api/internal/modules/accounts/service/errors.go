package service

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidInput      = errors.New("invalid account input")
	ErrAccountNotFound   = errors.New("account not found")
	ErrAccountExists     = errors.New("account already exists")
	ErrCredentialMissing = errors.New("account credential missing")
	ErrEncryptionFailed  = errors.New("account credential encryption failed")
	ErrProxyUnavailable  = errors.New("account proxy unavailable")
)

type MixedChannelError struct {
	GroupID          int
	AccountPlatform  string
	ExistingPlatform string
}

func (e *MixedChannelError) Error() string {
	return fmt.Sprintf("group %d already contains %s accounts; adding a %s account would create a mixed-platform channel", e.GroupID, e.ExistingPlatform, e.AccountPlatform)
}
