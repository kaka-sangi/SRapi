package httpserver

import (
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func TestAccountAllowsInboundClient(t *testing.T) {
	codex := gatewayInboundClient{UserAgent: "codex_cli_rs/0.125.0 (x86_64)", Originator: "codex_cli_rs"}
	browser := gatewayInboundClient{UserAgent: "Mozilla/5.0", Originator: ""}
	forgedOriginator := gatewayInboundClient{UserAgent: "curl/8.0", Originator: "codex_cli_rs"}

	// No restriction: everyone allowed.
	if !accountAllowsInboundClient(nil, browser) {
		t.Fatal("unrestricted account must accept any client")
	}
	if !accountAllowsInboundClient(map[string]any{"allowed_clients": []any{}}, browser) {
		t.Fatal("empty allowed_clients means no restriction")
	}

	restricted := map[string]any{"allowed_clients": []any{"codex_cli"}}
	if !accountAllowsInboundClient(restricted, codex) {
		t.Fatal("codex-only account must accept the codex CLI signature")
	}
	if accountAllowsInboundClient(restricted, browser) {
		t.Fatal("codex-only account must reject a generic browser")
	}
	// Two-factor: matching originator but non-matching UA must be rejected.
	if accountAllowsInboundClient(restricted, forgedOriginator) {
		t.Fatal("originator alone (without matching UA) must not pass")
	}

	// Multi-preset list: any listed preset matching is enough.
	multi := map[string]any{"allowed_clients": []any{"claude_code", "codex_cli"}}
	if !accountAllowsInboundClient(multi, codex) {
		t.Fatal("a client matching any listed preset should be accepted")
	}
}

func TestFilterCandidatesByAllowedClients(t *testing.T) {
	acct := func(id int, meta map[string]any) schedulercontract.Candidate {
		return schedulercontract.Candidate{Account: accountcontract.ProviderAccount{ID: id, Metadata: meta}}
	}
	candidates := []schedulercontract.Candidate{
		acct(1, nil), // open
		acct(2, map[string]any{"allowed_clients": []any{"codex_cli"}}), // codex-only
	}
	// A browser keeps only the open account; a codex client keeps both.
	browser := filterCandidatesByAllowedClients(candidates, gatewayInboundClient{UserAgent: "Mozilla/5.0"})
	if len(browser) != 1 || browser[0].Account.ID != 1 {
		t.Fatalf("browser should only reach the open account, got %+v", browser)
	}
	codex := filterCandidatesByAllowedClients(candidates, gatewayInboundClient{UserAgent: "codex_cli_rs/1", Originator: "codex_cli_rs"})
	if len(codex) != 2 {
		t.Fatalf("codex client should reach both accounts, got %d", len(codex))
	}
}
