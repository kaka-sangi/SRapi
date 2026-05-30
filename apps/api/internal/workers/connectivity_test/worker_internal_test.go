package connectivitytest

import (
	"context"
	"net/http"
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

func TestProbeModelOptIn(t *testing.T) {
	// No probe model configured -> not eligible (empty).
	if got := probeModel(accountcontract.ProviderAccount{}, providercontract.Provider{}); got != "" {
		t.Fatalf("probeModel with no config = %q, want empty", got)
	}
	// Account metadata opts in by configuring the probe model.
	acc := accountcontract.ProviderAccount{Metadata: map[string]any{"test_model": "gpt-probe "}}
	if got := probeModel(acc, providercontract.Provider{}); got != "gpt-probe" {
		t.Fatalf("probeModel from account metadata = %q, want gpt-probe", got)
	}
	// Provider config can also supply it.
	prov := providercontract.Provider{ConfigSchema: map[string]any{"compact_probe_model": "prov-model"}}
	if got := probeModel(accountcontract.ProviderAccount{}, prov); got != "prov-model" {
		t.Fatalf("probeModel from provider config = %q, want prov-model", got)
	}
}

type stubAdapter struct {
	resp provideradaptercontract.ConversationResponse
	err  error
}

func (s stubAdapter) InvokeConversation(ctx context.Context, req provideradaptercontract.ConversationRequest) (provideradaptercontract.ConversationResponse, error) {
	return s.resp, s.err
}

func TestConversationProberOutcome(t *testing.T) {
	ctx := context.Background()
	account := accountcontract.ProviderAccount{ID: 1}

	// Success -> OK with the upstream status.
	ok := conversationProber{adapter: stubAdapter{resp: provideradaptercontract.ConversationResponse{StatusCode: 200}}, model: "m"}
	res, err := ok.ProbeAccount(ctx, account, nil)
	if err != nil || !res.OK || res.StatusCode != 200 {
		t.Fatalf("success probe = %+v err=%v, want OK 200", res, err)
	}

	// Upstream ProviderError -> not-OK result (not an error) with class+status,
	// so it is folded into an unhealthy snapshot rather than dropped.
	failAdapter := stubAdapter{err: provideradaptercontract.ProviderError{Class: "rate_limit", StatusCode: http.StatusTooManyRequests, Message: "slow down"}}
	failed := conversationProber{adapter: failAdapter, model: "m"}
	res, err = failed.ProbeAccount(ctx, account, nil)
	if err != nil {
		t.Fatalf("failed probe returned error %v, want nil so the snapshot is recorded", err)
	}
	if res.OK || res.ErrorClass != "rate_limit" || res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("failed probe = %+v, want not-OK rate_limit 429", res)
	}
}
