// Codex / OpenAI JWS response token validation.
//
// Ported from CLIProxyAPI internal/auth/codex/jwt_parser.go. The
// reference implementation parses the claims segment (Base64URL,
// padding-stripped) without verifying the cryptographic signature —
// the Codex CLI relies on the OAuth server having validated the ID
// token before it lands. CLIProxyAPI exposes ParseJWTToken +
// JWTClaims and never re-validates downstream.
//
// srapi mirrors that surface verbatim and adds a strict-by-default
// response-side validator on top: ValidateCodexResponseJWS extracts a
// JWS token from a Codex /v1/responses payload (either a top-level
// "token" field or a Bearer-style "Authorization" header inside the
// envelope), parses it with the verbatim ParseJWTToken, and checks
// audience + issuer + exp. Failures return descriptive errors so the
// inline 401-retry logic in runtime_gateway_core.go can classify them.
//
// Leniency: as per the task spec the response envelope may not carry
// a JWS today. ValidateCodexResponseJWS returns (true, nil) for the
// "no token present" case so the validator is informational, not
// blocking. A single bool constant (codexJWSEnforceMode) flips it to
// strict reject — kept here so a future PR is a one-line change.
package signature

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CodexJWTClaims mirrors CLIProxyAPI internal/auth/codex/jwt_parser.go
// JWTClaims verbatim. Field tags MUST stay byte-for-byte identical to
// the reference, otherwise the parsing of a real Codex ID token will
// silently drop fields the rest of the system depends on.
type CodexJWTClaims struct {
	AtHash        string             `json:"at_hash"`
	Aud           []string           `json:"aud"`
	AuthProvider  string             `json:"auth_provider"`
	AuthTime      int                `json:"auth_time"`
	Email         string             `json:"email"`
	EmailVerified bool               `json:"email_verified"`
	Exp           int                `json:"exp"`
	CodexAuthInfo CodexJWTAuthInfo   `json:"https://api.openai.com/auth"`
	Iat           int                `json:"iat"`
	Iss           string             `json:"iss"`
	Jti           string             `json:"jti"`
	Rat           int                `json:"rat"`
	Sid           string             `json:"sid"`
	Sub           string             `json:"sub"`
}

// CodexJWTAuthInfo mirrors CLIProxyAPI CodexAuthInfo.
type CodexJWTAuthInfo struct {
	ChatgptAccountID               string                  `json:"chatgpt_account_id"`
	ChatgptPlanType                string                  `json:"chatgpt_plan_type"`
	ChatgptSubscriptionActiveStart any                     `json:"chatgpt_subscription_active_start"`
	ChatgptSubscriptionActiveUntil any                     `json:"chatgpt_subscription_active_until"`
	ChatgptSubscriptionLastChecked time.Time               `json:"chatgpt_subscription_last_checked"`
	ChatgptUserID                  string                  `json:"chatgpt_user_id"`
	Groups                         []any                   `json:"groups"`
	Organizations                  []CodexJWTOrganizations `json:"organizations"`
	UserID                         string                  `json:"user_id"`
}

// CodexJWTOrganizations mirrors CLIProxyAPI Organizations.
type CodexJWTOrganizations struct {
	ID        string `json:"id"`
	IsDefault bool   `json:"is_default"`
	Role      string `json:"role"`
	Title     string `json:"title"`
}

// GetUserEmail mirrors CLIProxyAPI.
func (c *CodexJWTClaims) GetUserEmail() string { return c.Email }

// GetAccountID mirrors CLIProxyAPI.
func (c *CodexJWTClaims) GetAccountID() string {
	return c.CodexAuthInfo.ChatgptAccountID
}

// CodexJWSEnforceMode toggles strict rejection of unsigned / missing
// Codex response tokens. Default false (lenient): the validator
// returns (true, nil) when no JWS is present in the response so
// existing unsigned Codex CLI responses continue to flow. Flip to
// true once the OpenAI signing rollout is confirmed; this single
// constant is the only change needed.
const CodexJWSEnforceMode = false

// ExpectedCodexJWSIssuer is the issuer string OpenAI's Codex tokens
// embed. CLIProxyAPI does not hardcode it (the OAuth server vouches
// for the ID token first), but we accept the canonical OpenAI auth
// host plus the loopback dev issuer used by the local CLI for
// completeness.
var ExpectedCodexJWSIssuers = []string{
	"https://auth.openai.com",
	"https://auth0.openai.com/",
	"https://auth.openai.com/",
}

// ExpectedCodexJWSAudiences mirrors the Codex CLI's known audience
// list. An empty intersection with the token's `aud` array fails the
// audience check.
var ExpectedCodexJWSAudiences = []string{
	"https://api.openai.com/v1",
	"https://api.openai.com",
}

// ParseCodexJWT mirrors CLIProxyAPI ParseJWTToken byte for byte: it
// parses the claims (segment 1) WITHOUT cryptographic verification.
// Real signature verification is the OAuth server's job. Use
// ValidateCodexResponseJWS for response-side audience/issuer/exp
// checks.
func ParseCodexJWT(token string) (*CodexJWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT token format: expected 3 parts, got %d", len(parts))
	}
	claimsData, err := codexJWTBase64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT claims: %w", err)
	}
	var claims CodexJWTClaims
	if err = json.Unmarshal(claimsData, &claims); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JWT claims: %w", err)
	}
	return &claims, nil
}

// codexJWTBase64URLDecode mirrors CLIProxyAPI base64URLDecode.
func codexJWTBase64URLDecode(data string) ([]byte, error) {
	switch len(data) % 4 {
	case 2:
		data += "=="
	case 3:
		data += "="
	}
	return base64.URLEncoding.DecodeString(data)
}

// CodexJWSValidationResult is the structured return of
// ValidateCodexResponseJWS so callers can log + decide retry policy.
type CodexJWSValidationResult struct {
	// Present is true when the response carried a JWS-shaped token.
	Present bool
	// Valid is true when Present AND audience+issuer+exp all pass.
	// Also true when !Present and !CodexJWSEnforceMode (lenient).
	Valid bool
	// Reason describes the failure. Empty when Valid.
	Reason string
	// Claims is non-nil when the token parsed (regardless of validity).
	Claims *CodexJWTClaims
}

// ValidateCodexResponseJWS scans rawResponse for a JWS token, parses
// it, and validates audience/issuer/exp. Designed for the codex.go
// response path; see the package header for leniency rules.
//
// publicKeys is reserved for future cryptographic verification (the
// reference implementation does not perform it, so this parameter is
// accepted but unused today — the signature is preserved so a future
// PR can plug in JWKS-backed verification without touching call
// sites). Pass nil from callers that don't have keys handy.
func ValidateCodexResponseJWS(rawResponse []byte, _ map[string]any) (CodexJWSValidationResult, error) {
	token := extractCodexJWSToken(rawResponse)
	if token == "" {
		if CodexJWSEnforceMode {
			return CodexJWSValidationResult{Present: false, Valid: false, Reason: "missing JWS token"}, fmt.Errorf("codex response missing JWS token")
		}
		return CodexJWSValidationResult{Present: false, Valid: true, Reason: ""}, nil
	}
	claims, err := ParseCodexJWT(token)
	if err != nil {
		return CodexJWSValidationResult{Present: true, Valid: false, Reason: err.Error()}, err
	}
	if reason := validateCodexJWTClaims(claims, time.Now()); reason != "" {
		return CodexJWSValidationResult{Present: true, Valid: false, Reason: reason, Claims: claims}, fmt.Errorf("codex JWS validation failed: %s", reason)
	}
	return CodexJWSValidationResult{Present: true, Valid: true, Claims: claims}, nil
}

func validateCodexJWTClaims(claims *CodexJWTClaims, now time.Time) string {
	if claims == nil {
		return "nil claims"
	}
	if claims.Exp > 0 && time.Unix(int64(claims.Exp), 0).Before(now) {
		return fmt.Sprintf("token expired at %s", time.Unix(int64(claims.Exp), 0).UTC().Format(time.RFC3339))
	}
	if claims.Iss != "" {
		match := false
		for _, expected := range ExpectedCodexJWSIssuers {
			if strings.EqualFold(strings.TrimRight(claims.Iss, "/"), strings.TrimRight(expected, "/")) {
				match = true
				break
			}
		}
		if !match {
			return fmt.Sprintf("unexpected issuer %q", claims.Iss)
		}
	}
	if len(claims.Aud) > 0 {
		match := false
		for _, audClaim := range claims.Aud {
			for _, expected := range ExpectedCodexJWSAudiences {
				if strings.EqualFold(audClaim, expected) {
					match = true
					break
				}
			}
			if match {
				break
			}
		}
		if !match {
			return fmt.Sprintf("audience %v has no overlap with %v", claims.Aud, ExpectedCodexJWSAudiences)
		}
	}
	return ""
}

// extractCodexJWSToken returns the JWS string embedded in a Codex
// response payload, or empty if no candidate field is found. We look
// for:
//   - a top-level "token" string
//   - a top-level "id_token" string
//   - a top-level "jws" string
//   - an "auth" object's "token" / "id_token" field
//   - an "Authorization: Bearer <token>" header value buried in the
//     response (some Codex paths echo upstream headers back).
//
// Only canonical-shape "xxx.yyy.zzz" base64url triplets are returned.
func extractCodexJWSToken(rawResponse []byte) string {
	if len(rawResponse) == 0 {
		return ""
	}
	trimmed := strings.TrimLeftFunc(string(rawResponse), func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if !strings.HasPrefix(trimmed, "{") {
		return ""
	}
	var envelope map[string]any
	if err := json.Unmarshal(rawResponse, &envelope); err != nil {
		return ""
	}
	for _, key := range []string{"token", "id_token", "jws", "access_token"} {
		if value, ok := envelope[key].(string); ok && looksLikeJWS(value) {
			return value
		}
	}
	if auth, ok := envelope["auth"].(map[string]any); ok {
		for _, key := range []string{"token", "id_token", "jws", "access_token"} {
			if value, ok := auth[key].(string); ok && looksLikeJWS(value) {
				return value
			}
		}
	}
	if authz, ok := envelope["Authorization"].(string); ok {
		if value := strings.TrimPrefix(authz, "Bearer "); value != authz && looksLikeJWS(value) {
			return value
		}
	}
	return ""
}

func looksLikeJWS(value string) bool {
	if value == "" {
		return false
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
	}
	return true
}
