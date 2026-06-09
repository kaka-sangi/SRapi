package httpserver

import (
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func gatewayTokenCountFromProvider(resp provideradaptercontract.TokenCountResponse) gatewaycontract.TokenCountResponse {
	return gatewaycontract.TokenCountResponse{
		TotalTokens:             resp.TotalTokens,
		CachedContentTokenCount: cloneIntPtr(resp.CachedContentTokenCount),
		PromptTokensDetails:     gatewayModalityTokenCountsFromProvider(resp.PromptTokensDetails),
		CacheTokensDetails:      gatewayModalityTokenCountsFromProvider(resp.CacheTokensDetails),
		Metadata:                cloneAnyMap(resp.Metadata),
	}
}

func gatewayModalityTokenCountsFromProvider(values []provideradaptercontract.ModalityTokenCount) []gatewaycontract.ModalityTokenCount {
	if len(values) == 0 {
		return nil
	}
	out := make([]gatewaycontract.ModalityTokenCount, 0, len(values))
	for _, value := range values {
		out = append(out, gatewaycontract.ModalityTokenCount{
			Modality:   value.Modality,
			TokenCount: value.TokenCount,
			Metadata:   cloneAnyMap(value.Metadata),
		})
	}
	return out
}
