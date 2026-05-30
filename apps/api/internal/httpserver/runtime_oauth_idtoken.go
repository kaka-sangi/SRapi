package httpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// verifyOIDCIDToken validates a provider id_token per OIDC: OIDC discovery from
// the issuer yields the JWKS, go-oidc verifies the signature and the iss / aud
// (== clientID) / exp claims, and we additionally check the nonce matches the
// one bound to this authorization flow (replay/CSRF defense). Called only when an
// issuer is configured for the provider; an empty/invalid token is rejected.
func verifyOIDCIDToken(ctx context.Context, issuer, clientID, rawIDToken, expectedNonce string) error {
	if strings.TrimSpace(rawIDToken) == "" {
		return errors.New("oidc id_token missing")
	}
	provider, err := oidc.NewProvider(ctx, strings.TrimSpace(issuer))
	if err != nil {
		return fmt.Errorf("oidc discovery: %w", err)
	}
	token, err := provider.Verifier(&oidc.Config{ClientID: strings.TrimSpace(clientID)}).Verify(ctx, rawIDToken)
	if err != nil {
		return fmt.Errorf("oidc verify: %w", err)
	}
	var claims struct {
		Nonce string `json:"nonce"`
	}
	if err := token.Claims(&claims); err != nil {
		return fmt.Errorf("oidc claims: %w", err)
	}
	if strings.TrimSpace(expectedNonce) != "" && claims.Nonce != expectedNonce {
		return errors.New("oidc id_token nonce mismatch")
	}
	return nil
}
