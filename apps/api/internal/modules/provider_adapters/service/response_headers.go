package service

import (
	"net/http"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func withConversationResponseHeaders(resp contract.ConversationResponse, headers http.Header) contract.ConversationResponse {
	if len(headers) == 0 {
		return resp
	}
	resp.Headers = cloneGenericHeaders(headers)
	return resp
}
