// Package contract defines the session→account affinity binding store.
//
// Session affinity ("会话粘度") keeps a multi-turn conversation pinned to the
// same upstream provider account across requests. Re-using the same account
// maximizes provider-side prompt-cache hits (cheaper, lower latency) and keeps
// session-scoped upstream state consistent. The binding is best-effort and
// TTL-bounded: an idle conversation naturally releases its account, and the
// bound account is always re-validated against the live candidate set before it
// is honored, so a drained/disabled account never traps a session.
package contract

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrInvalidInput is returned when a binding operation receives empty required
// arguments.
var ErrInvalidInput = errors.New("invalid session affinity input")

// ChainMarker prefixes a derived conversation digest-chain key. Keys carrying
// this marker support longest-prefix lookup (a later turn's chain is a prefix
// extension of the earlier turn's chain), so a header-less multi-turn
// conversation still resolves to the account that served its earlier turns.
// Keys without this marker (explicit/stable session identifiers) are matched
// exactly.
const ChainMarker = "dc:"

// chainSeparator joins digest-chain segments.
const chainSeparator = "-"

// Binding is a resolved session→account affinity record.
type Binding struct {
	// AccountID is the bound provider account, or 0 when no binding exists.
	AccountID int
	// MatchedKey is the (possibly shorter, longest-prefix) key that resolved the
	// binding. Empty when AccountID is 0.
	MatchedKey string
}

// Found reports whether the lookup resolved a binding.
func (b Binding) Found() bool { return b.AccountID > 0 }

// Store persists session→account affinity bindings with TTL.
//
// Implementations must be safe for concurrent use.
type Store interface {
	// Lookup resolves the account bound to sessionKey within scope. For
	// digest-chain keys it returns the longest-prefix match. On a hit it
	// refreshes the matched binding's TTL to ttl so active conversations stay
	// pinned while idle ones expire. A miss returns a zero Binding and nil error.
	Lookup(ctx context.Context, scope, sessionKey string, ttl time.Duration) (Binding, error)
	// Bind stores sessionKey→accountID within scope with the given TTL,
	// overwriting/refreshing any existing binding for that exact key.
	Bind(ctx context.Context, scope, sessionKey string, accountID int, ttl time.Duration) error
	// Release removes the binding for sessionKey within scope. Best-effort: a
	// missing binding is not an error.
	Release(ctx context.Context, scope, sessionKey string) error
	// AddAccountSession records that a conversation (sessionID) is active on an
	// account, with the given TTL, so the number of distinct conversations an
	// account serves can be capped. Re-recording the same sessionID just refreshes
	// it (one conversation never counts twice).
	AddAccountSession(ctx context.Context, accountID int, sessionID string, ttl time.Duration) error
	// CountAccountSessionsExcluding returns how many distinct active sessions are
	// on accountID other than sessionID (so an existing conversation does not
	// count against its own re-use). Expired sessions are excluded.
	CountAccountSessionsExcluding(ctx context.Context, accountID int, sessionID string) (int, error)
}

// CandidateKeys returns the lookup keys for sessionKey ordered from most to
// least specific. For an exact (non-chain) key it returns the key alone. For a
// digest-chain key it returns the full chain followed by each shorter prefix,
// so the longest still-bound prefix wins.
func CandidateKeys(sessionKey string) []string {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return nil
	}
	if !strings.HasPrefix(sessionKey, ChainMarker) {
		return []string{sessionKey}
	}
	body := strings.TrimPrefix(sessionKey, ChainMarker)
	if body == "" {
		return nil
	}
	segments := strings.Split(body, chainSeparator)
	keys := make([]string, 0, len(segments))
	for i := len(segments); i >= 1; i-- {
		keys = append(keys, ChainMarker+strings.Join(segments[:i], chainSeparator))
	}
	return keys
}
