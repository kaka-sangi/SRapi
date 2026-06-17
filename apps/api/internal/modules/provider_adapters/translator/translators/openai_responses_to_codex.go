// Package translators wires per-pair translation logic into the parent
// translator.Registry at package init. Each file in this directory owns
// exactly one (from, to) pair so adding a new format is a one-file change
// (mirrors CLIProxyAPI's internal/translator/<source>/<target>/ layout).
//
// Migration policy: a translator may delegate to the existing inline
// transform in service/ until the inline body is fully extracted into the
// translator. The first generation of translators is deliberately thin
// (one rewrite responsibility per translator); the registry's
// fallthrough-on-miss contract means the inline path keeps working until
// the translator is feature-complete and the inline call site has been
// migrated to consult the registry.
package translators

import (
	"encoding/json"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/translator"
)

// modelAliasRewriter rewrites the JSON body's "model" field via a caller-
// supplied alias map. Kept as a private helper so the translator can stay
// pure-functional (no config dependency in this layer); the registry call
// site supplies the alias map computed from the runtime config + the
// account's OAuth channel.
//
// nil-safe across the board: nil rawJSON, missing "model" field, nil alias
// map all fall through to the input unchanged.
func modelAliasRewriter(modelName string, rawJSON []byte, aliases map[string]string) []byte {
	if len(rawJSON) == 0 || len(aliases) == 0 {
		return rawJSON
	}
	var payload map[string]any
	if err := json.Unmarshal(rawJSON, &payload); err != nil {
		// Body isn't JSON the rewriter understands — return unchanged so
		// the upstream sees the caller's original bytes. The inline
		// transforms exhibited the same defensive behaviour.
		return rawJSON
	}
	current, _ := payload["model"].(string)
	if current == "" {
		current = modelName
	}
	alias, ok := aliases[current]
	if !ok || alias == "" || alias == current {
		return rawJSON
	}
	payload["model"] = alias
	out, err := json.Marshal(payload)
	if err != nil {
		return rawJSON
	}
	return out
}

// OpenAIResponsesToCodexAliasContext is the per-call alias context the
// translator needs. Plumbed via a context-bound value or a closure capture
// at the call site — the translator itself stays pure for testability.
type OpenAIResponsesToCodexAliasContext struct {
	// Aliases maps canonical model name → upstream alias (per the
	// runtime config's Codex.ModelAlias[<channel>] map).
	Aliases map[string]string
}

// RegisterOpenAIResponsesToCodexWithAliases registers the translator for
// the (openai_responses → codex) pair using the supplied alias context.
// Call sites (codex.go) build the alias map per-request from the active
// runtime config + the account's OAuth channel and pass it through this
// constructor instead of registering at package init() time, because the
// alias map is dynamic. The legacy package init() registration registers
// an identity translator that falls through to the inline path.
func RegisterOpenAIResponsesToCodexWithAliases(reg *translator.Registry, aliases map[string]string) {
	reg.Register(
		translator.FormatOpenAIResponses,
		translator.FormatCodex,
		func(modelName string, rawJSON []byte, _ bool) []byte {
			return modelAliasRewriter(modelName, rawJSON, aliases)
		},
		nil,
	)
}

// init registers an IDENTITY translator on the Default() registry so call
// sites that consult registry.TranslateRequest before the per-request
// override is installed get a defined behaviour: the input bytes back
// unchanged. This is the same fallthrough the registry itself emits on a
// missing pair — the registration here just makes the path observable in
// metrics ("translator was consulted, transform was no-op") rather than
// invisible. The dynamic per-request alias override
// (RegisterOpenAIResponsesToCodexWithAliases) shadows this at call time.
func init() {
	translator.Default().Register(
		translator.FormatOpenAIResponses,
		translator.FormatCodex,
		func(_ string, rawJSON []byte, _ bool) []byte { return rawJSON },
		nil,
	)
}
