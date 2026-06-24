package service

import "errors"

var (
	ErrInvalidInput          = errors.New("invalid user input")
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrUserDisabled          = errors.New("user disabled")
	ErrUserNotFound          = errors.New("user not found")
	ErrUserAlreadyExists     = errors.New("user already exists")
	ErrRoleNotFound          = errors.New("role not found")
	ErrRoleImmutable         = errors.New("built-in roles cannot be modified")
	ErrIdentityAlreadyBound  = errors.New("user auth identity already bound")
	ErrIdentityNotFound      = errors.New("user auth identity not found")
	ErrIdentityUnbindBlocked = errors.New("user auth identity cannot be unbound")
	ErrCurrencyMismatch      = errors.New("cannot mix currencies: change currency only when balance is zero")
)
