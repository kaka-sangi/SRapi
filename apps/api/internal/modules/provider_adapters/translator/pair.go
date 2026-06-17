package translator

import "fmt"

// Pair is the registry key for a Translator. Direction matters — a
// translator that converts Codex → OpenAI responses is distinct from the
// reverse direction, and registering both is the registry's job, not a
// pseudo-bidirectional helper here. Mirrors CLIProxyAPI's pair-keyed lookup
// at sdk/translator/.
type Pair struct {
	From Format
	To   Format
}

// String returns a stable identifier for the pair, suitable for use in
// metrics labels and log fields ("codex->openai_responses").
func (p Pair) String() string {
	return fmt.Sprintf("%s->%s", p.From.String(), p.To.String())
}

// Valid reports whether both ends of the pair are non-empty. The registry
// uses this to reject Register calls with blank formats — otherwise an
// empty Format would silently match every input.
func (p Pair) Valid() bool {
	return !p.From.Empty() && !p.To.Empty()
}

// Identity reports whether the From and To formats are the same. Identity
// pairs are no-ops (the registry returns input unchanged) — registering
// one is allowed but pointless, and asking the registry to translate
// identity pairs is cheap.
func (p Pair) Identity() bool {
	return p.From == p.To
}
