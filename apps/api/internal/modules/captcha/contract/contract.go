package contract

import (
	"context"
	"errors"
)

// ErrCaptchaRequired is returned when verification is enabled but no token was
// supplied by the client.
var ErrCaptchaRequired = errors.New("captcha token required")

// ErrCaptchaFailed is returned when the provider rejects the supplied token.
var ErrCaptchaFailed = errors.New("captcha verification failed")

// Verifier checks a client-supplied captcha token against a provider.
type Verifier interface {
	// Verify reports whether the token is valid. remoteIP may be empty.
	Verify(ctx context.Context, secret, token, remoteIP string) (bool, error)
}
