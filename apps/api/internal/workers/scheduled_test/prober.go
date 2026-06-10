package scheduledtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

const probeInput = "Respond with OK."

// conversationAdapter is the upstream conversation invoker the prober uses.
type conversationAdapter = provideradaptercontract.ConversationAdapter

// probeModelKeys are the account-metadata / provider-config keys that select the
// model used for the connectivity probe; a configured model is the opt-in signal.
var probeModelKeys = []string{"responses_compact_probe_model", "compact_probe_model", "test_model"}

// conversationProber issues a real generative probe and reports the outcome.
// Upstream failures are folded into a not-OK result rather than an error.
type conversationProber struct {
	adapter  provideradaptercontract.ConversationAdapter
	provider providercontract.Provider
	model    string
}

func (p conversationProber) ProbeAccount(ctx context.Context, account accountcontract.ProviderAccount, credential map[string]any) (accountcontract.AccountProbeResult, error) {
	startedAt := time.Now()
	raw, err := json.Marshal(map[string]any{"model": p.model, "input": probeInput})
	if err != nil {
		return accountcontract.AccountProbeResult{OK: false, ErrorClass: "probe_payload_failed", StatusCode: http.StatusInternalServerError, CheckedAt: time.Now().UTC()}, nil
	}
	resp, err := p.adapter.InvokeConversation(ctx, provideradaptercontract.ConversationRequest{
		RequestID:      fmt.Sprintf("scheduled_test_%d", account.ID),
		SourceProtocol: string(gatewaycontract.ProtocolOpenAICompatible),
		SourceEndpoint: string(gatewaycontract.EndpointResponsesCompact),
		TargetProtocol: p.provider.Protocol,
		Model:          p.model,
		InputParts:     []provideradaptercontract.ContentPart{{Kind: provideradaptercontract.ContentPartText, Text: probeInput}},
		RawBody:        raw,
		Provider:       p.provider,
		Account:        account,
		Mapping:        modelcontract.ModelProviderMapping{UpstreamModelName: p.model},
		Credential:     credential,
	})
	latency := int(time.Since(startedAt).Milliseconds())
	if err == nil {
		status := resp.StatusCode
		if status <= 0 {
			status = http.StatusOK
		}
		return accountcontract.AccountProbeResult{OK: true, StatusCode: status, LatencyMS: latency, CheckedAt: time.Now().UTC()}, nil
	}
	var providerErr provideradaptercontract.ProviderError
	status := http.StatusBadGateway
	errorClass := "provider_probe_failed"
	if errors.As(err, &providerErr) {
		if providerErr.StatusCode > 0 {
			status = providerErr.StatusCode
		}
		if strings.TrimSpace(providerErr.Class) != "" {
			errorClass = strings.TrimSpace(providerErr.Class)
		}
	}
	return accountcontract.AccountProbeResult{OK: false, ErrorClass: errorClass, StatusCode: status, LatencyMS: latency, CheckedAt: time.Now().UTC()}, nil
}

func probeModel(planModel string, account accountcontract.ProviderAccount, provider providercontract.Provider) string {
	if model := strings.TrimSpace(planModel); model != "" {
		return model
	}
	for _, values := range []map[string]any{account.Metadata, provider.ConfigSchema, provider.Capabilities} {
		for _, key := range probeModelKeys {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func mapString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	if raw, ok := values[key]; ok {
		if str, ok := raw.(string); ok {
			return strings.TrimSpace(str)
		}
	}
	return ""
}
