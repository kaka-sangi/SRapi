package service

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func TestCodexResponsesHeadersForwardsAcceptLanguage(t *testing.T) {
	req := contract.ConversationRequest{
		RequestSettings: map[string]any{"accept-language": "fr-FR,fr;q=0.9"},
	}
	headers := codexResponsesHeaders(req, false, map[string]any{})
	if got := headers.Get("Accept-Language"); got != "fr-FR,fr;q=0.9" {
		t.Fatalf("expected Accept-Language to be forwarded, got %q", got)
	}
}

func TestCodexResponsesHeadersOmitsAcceptLanguageWhenAbsent(t *testing.T) {
	headers := codexResponsesHeaders(contract.ConversationRequest{}, false, map[string]any{})
	if _, ok := headers["Accept-Language"]; ok {
		t.Fatalf("expected Accept-Language header to be absent, got %q", headers.Get("Accept-Language"))
	}
}

func TestCodexResponseInputItemsHeadersForwardsAcceptLanguage(t *testing.T) {
	req := contract.ResponseInputItemsRequest{
		RequestSettings: map[string]any{"accept-language": "de-DE"},
	}
	headers := codexResponseInputItemsHeaders(req)
	if got := headers.Get("Accept-Language"); got != "de-DE" {
		t.Fatalf("expected Accept-Language to be forwarded, got %q", got)
	}
}

func TestCodexRealtimeHeadersForwardsAcceptLanguage(t *testing.T) {
	req := contract.RealtimeRequest{
		RequestSettings: map[string]any{"accept-language": "ja-JP,ja;q=0.8"},
	}
	headers := codexRealtimeHeaders(req, []byte(`{}`))
	if got := headers.Get("Accept-Language"); got != "ja-JP,ja;q=0.8" {
		t.Fatalf("expected Accept-Language to be forwarded, got %q", got)
	}
}
