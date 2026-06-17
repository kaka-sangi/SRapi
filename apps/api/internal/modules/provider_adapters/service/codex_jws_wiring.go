// Wiring glue for the Codex JWS response validator
// (apps/api/internal/pkg/signature/codex_jws.go).
//
// Why the validator runs as a WARN (not BLOCK) by default:
//
//	Today many Codex CLI responses are unsigned (the OAuth server
//	signs the *upstream* ID token, but the Codex /v1/responses body
//	itself is sometimes returned raw — the OpenAI signing rollout is
//	in progress at time of writing). Hard-rejecting an unsigned body
//	would break every existing account's traffic. Per the task spec
//	we therefore land the validator in inform-only mode: validate +
//	log on failure, but pass the body through to the parser.
//
//	When OpenAI confirms the rollout, flipping signature.CodexJWSEnforceMode
//	to true is the single line that converts the validator from
//	informational to blocking; the call site here doesn't need to
//	change.
//
// Best-effort: a panic in the validator must not reach the response
// path, so codexValidateUpstreamResponseJWS swallows panics via a
// deferred recover and treats them as "no validation performed".
package service

import (
	"fmt"

	"github.com/srapi/srapi/apps/api/internal/pkg/signature"
)

// codexValidateUpstreamResponseJWS runs the JWS validator over a
// successful upstream response body. The return value reports whether
// the body was rejected; in lenient mode (the default) the function
// ALWAYS returns nil, even when the validator found a forged token,
// so that the existing response delivery is not perturbed. The
// caller is expected to log the returned diagnostic when non-empty.
func codexValidateUpstreamResponseJWS(body []byte) (rejection error, diagnostic string) {
	defer func() {
		if r := recover(); r != nil {
			diagnostic = fmt.Sprintf("codex jws validator panic: %v", r)
			rejection = nil
		}
	}()
	res, err := signature.ValidateCodexResponseJWS(body, nil)
	if !res.Present {
		// No JWS in the body; lenient mode treats this as a pass.
		return nil, ""
	}
	if res.Valid {
		return nil, ""
	}
	// Found a token but it failed validation. In strict mode the
	// validator already returned an error and we propagate; in
	// lenient mode we surface the diagnostic so the caller can
	// metric/log it without blocking the response.
	diag := fmt.Sprintf("codex jws response invalid: %s", res.Reason)
	if signature.CodexJWSEnforceMode {
		if err != nil {
			return err, diag
		}
		return fmt.Errorf("%s", diag), diag
	}
	return nil, diag
}
