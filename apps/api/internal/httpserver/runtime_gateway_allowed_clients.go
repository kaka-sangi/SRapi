package httpserver

import (
	"context"
	"net/http"
	"strings"

	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

// Account-level inbound client gating ("只允许指定官方客户端请求该上游账号").
//
// An OAuth/upstream account can be restricted to specific official client
// signatures so a generic caller cannot drive it: hitting a provider's OAuth
// backend (ChatGPT/Claude) with a non-official client signature risks the
// account being flagged or banned for ToS violations. Mirrors sub2api's
// codex_cli_only_allowed_clients.
//
// The restriction lives in account metadata key "allowed_clients" as a list of
// preset keys (e.g. ["codex_cli","claude_code"]). Empty/absent = no restriction
// (every client accepted), preserving existing behavior.

const accountAllowedClientsMetadataKey = "allowed_clients"

// gatewayAllowedClientPreset is an official client signature. Matching is
// two-factor: the inbound Originator must equal Originator (case-insensitive)
// AND every UAContains marker must appear in the User-Agent — so a forgeable
// originator alone cannot pass.
type gatewayAllowedClientPreset struct {
	Originator string
	UAContains []string
}

// gatewayAllowedClientPresets are the recognized official-client signatures an
// account's allowed_clients list may reference.
var gatewayAllowedClientPresets = map[string]gatewayAllowedClientPreset{
	"codex_cli":   {Originator: "codex_cli_rs", UAContains: []string{"codex_cli_rs/"}},
	"claude_code": {Originator: "claude code", UAContains: []string{"claude code/"}},
	"gemini_cli":  {Originator: "gemini_cli", UAContains: []string{"geminicli/"}},
}

type gatewayInboundClientKey struct{}

// gatewayInboundClient is the request's client signature used for account
// allowed-clients gating.
type gatewayInboundClient struct {
	UserAgent  string
	Originator string
}

func withGatewayInboundClient(ctx context.Context, r *http.Request) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, gatewayInboundClientKey{}, gatewayInboundClient{
		UserAgent:  strings.TrimSpace(r.Header.Get("User-Agent")),
		Originator: strings.TrimSpace(firstNonEmpty(r.Header.Get("Originator"), r.Header.Get("X-Originator"))),
	})
}

func gatewayInboundClientFromContext(ctx context.Context) gatewayInboundClient {
	if client, ok := ctx.Value(gatewayInboundClientKey{}).(gatewayInboundClient); ok {
		return client
	}
	return gatewayInboundClient{}
}

// accountAllowsInboundClient reports whether an account accepts a request from
// the given client signature. No allowed_clients restriction accepts everyone.
func accountAllowsInboundClient(metadata map[string]any, client gatewayInboundClient) bool {
	presets, ok := metadataStringList(metadata, accountAllowedClientsMetadataKey)
	if !ok || len(presets) == 0 {
		return true
	}
	for _, name := range presets {
		preset, known := gatewayAllowedClientPresets[strings.ToLower(strings.TrimSpace(name))]
		if known && gatewayClientMatchesPreset(client, preset) {
			return true
		}
	}
	return false
}

func gatewayClientMatchesPreset(client gatewayInboundClient, preset gatewayAllowedClientPreset) bool {
	if strings.TrimSpace(preset.Originator) == "" {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(client.Originator), strings.TrimSpace(preset.Originator)) {
		return false
	}
	ua := strings.ToLower(client.UserAgent)
	for _, marker := range preset.UAContains {
		marker = strings.ToLower(strings.TrimSpace(marker))
		if marker == "" || !strings.Contains(ua, marker) {
			return false
		}
	}
	return true
}

// filterCandidatesByAllowedClients drops candidates whose account restricts
// inbound clients to signatures the current request does not match. A nil/empty
// client signature only passes accounts with no restriction.
func filterCandidatesByAllowedClients(candidates []schedulercontract.Candidate, client gatewayInboundClient) []schedulercontract.Candidate {
	filtered := make([]schedulercontract.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if accountAllowsInboundClient(candidate.Account.Metadata, client) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}
