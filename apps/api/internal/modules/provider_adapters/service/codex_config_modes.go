// Package service — Codex global-config modes ported verbatim from
// CLIProxyAPI's internal/config and sdk/config packages.
//
// This file is intentionally small and pure. It exposes:
//
//   - ResolveCodexModelAlias(cfg, channel, canonical): nested-map exact-match
//     lookup of the per-channel OAuth model alias, e.g.
//     {"openai":{"gpt-5-codex":"gpt-5.1-codex-internal"}}. Returns the alias
//     when one is configured, or the canonical name unchanged otherwise.
//     Exact match only — no glob/wildcard — mirroring CLIProxyAPI.
//
//   - ShouldDisableCodexImageGeneration(cfg, userAgent): three-state enum
//     ("never" | "always" | "auto") that the gateway consults before letting
//     a Codex request ship a hosted `image_generation` tool. The "auto" arm
//     matches the same User-Agent family CLIProxyAPI flags as broken
//     (specific Codex CLI builds that mis-route the tool).
//
// Both helpers are nil-safe and fall back to the safe default (allow / no
// alias) when the config is unset — see the per-function godoc for the
// exact semantics. The wiring layer is responsible for calling these at
// the codex.go request-build path; this file MUST NOT take any side
// effects on its own.
package service

import (
	"regexp"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/config"
)

// codexDisableImageGenAutoUARegex matches the User-Agent families CLIProxyAPI
// flags as known to mis-route the hosted image_generation tool. The pattern
// is case-insensitive and anchored on the originator + version triplet that
// the official Codex CLI ships in its UA header — see the constants in
// codex.go (codexDefaultUserAgent). Bare "codex_cli_rs/0.x.y" without the
// "(OS arch) terminal" suffix is the smoking-gun signature.
//
// We deliberately match only the family, never specific versions — the
// upstream rollout cadence is fast enough that a hard-coded version pin
// would silently stop matching after a release. The trade-off is the same
// one CLIProxyAPI accepted: the "auto" mode is conservative-by-family,
// not per-version. Operators who need finer control set "always" instead.
var codexDisableImageGenAutoUARegex = regexp.MustCompile(`(?i)codex_cli_rs/[\d.]+`)

// ResolveCodexModelAlias returns the upstream model name the gateway should
// rewrite the request to, given the per-channel OAuth model alias map.
//
// The lookup is exact-match only (no glob/wildcard, no regex), matching
// CLIProxyAPI's behaviour. Channel keys are normalized to lower-case before
// the lookup; canonical model names are matched verbatim (the upstream
// registry is case-sensitive).
//
// When either argument is empty, the cfg pointer is nil, the map is unset,
// or there is no entry for the (channel, canonical) pair, the function
// returns the canonical name unchanged so the call site can use it as a
// drop-in for the original model field.
func ResolveCodexModelAlias(cfg *config.Config, channel, canonical string) string {
	canonical = strings.TrimSpace(canonical)
	if canonical == "" {
		return canonical
	}
	if cfg == nil {
		return canonical
	}
	if len(cfg.Codex.ModelAlias) == 0 {
		return canonical
	}
	normalizedChannel := strings.ToLower(strings.TrimSpace(channel))
	if normalizedChannel == "" {
		return canonical
	}
	aliases, ok := cfg.Codex.ModelAlias[normalizedChannel]
	if !ok || len(aliases) == 0 {
		return canonical
	}
	alias, ok := aliases[canonical]
	if !ok {
		return canonical
	}
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return canonical
	}
	return alias
}

// ShouldDisableCodexImageGeneration reports whether the gateway must reject
// any Codex request that ships a hosted `image_generation` tool, based on
// the global DisableImageGeneration enum and the inbound User-Agent.
//
// Semantics — verbatim port of CLIProxyAPI's three-state enum:
//   - "never"  (default, also any unknown / unset value): allow the tool,
//     return false.
//   - "always": block unconditionally, return true.
//   - "auto":   block only when the User-Agent matches the known-broken
//     Codex CLI family (see codexDisableImageGenAutoUARegex). An empty
//     User-Agent does NOT match, so "auto" never blocks an unidentified
//     client — that's the safe default for forward-compat with new CLIs.
//
// nil-cfg is treated as "never".
func ShouldDisableCodexImageGeneration(cfg *config.Config, userAgent string) bool {
	if cfg == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Codex.DisableImageGeneration)) {
	case "always":
		return true
	case "auto":
		return codexDisableImageGenAutoUARegex.MatchString(userAgent)
	default:
		return false
	}
}
