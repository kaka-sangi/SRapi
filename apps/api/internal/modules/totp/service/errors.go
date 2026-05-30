package service

import totpcontract "github.com/srapi/srapi/apps/api/internal/modules/totp/contract"

var (
	ErrInvalidInput         = totpcontract.ErrInvalidInput
	ErrSecretNotFound       = totpcontract.ErrSecretNotFound
	ErrSecretDisabled       = totpcontract.ErrSecretDisabled
	ErrSecretAlreadyEnabled = totpcontract.ErrSecretAlreadyEnabled
	ErrInvalidCode          = totpcontract.ErrInvalidCode
	ErrSecretDecrypt        = totpcontract.ErrSecretDecrypt
	ErrRecoveryCodeRandom   = totpcontract.ErrRecoveryCodeRandom
)
