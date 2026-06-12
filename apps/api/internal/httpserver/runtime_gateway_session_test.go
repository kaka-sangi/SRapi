package httpserver

import (
	"net/http/httptest"
	"strings"
	"testing"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	sessionaffinitycontract "github.com/srapi/srapi/apps/api/internal/modules/sessionaffinity/contract"
)

func TestAnthropicSessionSeed(t *testing.T) {
	cases := []struct {
		name   string
		userID string
		want   string
	}{
		{"json form", `{"device_id":"dev","account_uuid":"acct","session_id":"sess"}`, "dev|acct|sess"},
		{"json without session", `{"device_id":"dev"}`, ""},
		{"legacy form", "user_" + strings.Repeat("a", 64) + "_account_b1b2_session_" + strings.Repeat("c", 36), strings.Repeat("a", 64) + "|b1b2|" + strings.Repeat("c", 36)},
		{"unknown per-user id", "user-12345", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := anthropicSessionSeed(tc.userID); got != tc.want {
				t.Fatalf("anthropicSessionSeed(%q) = %q, want %q", tc.userID, got, tc.want)
			}
		})
	}
}

func TestBuildGatewayDigestChainPrefixProperty(t *testing.T) {
	turn1 := gatewaycontract.CanonicalRequest{
		Instructions: "system prompt",
		Messages: []gatewaycontract.Message{
			{Role: "user", Content: []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Text: "hello"}}},
		},
	}
	turn2 := gatewaycontract.CanonicalRequest{
		Instructions: "system prompt",
		Messages: []gatewaycontract.Message{
			{Role: "user", Content: []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Text: "hello"}}},
			{Role: "assistant", Content: []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Text: "hi there"}}},
			{Role: "user", Content: []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Text: "bye"}}},
		},
	}
	chain1 := buildGatewayDigestChain(turn1)
	chain2 := buildGatewayDigestChain(turn2)
	if chain1 == "" || chain2 == "" {
		t.Fatalf("expected non-empty chains, got %q and %q", chain1, chain2)
	}
	if !strings.HasPrefix(chain1, sessionaffinitycontract.ChainMarker) {
		t.Fatalf("chain must carry the chain marker, got %q", chain1)
	}
	if !strings.HasPrefix(chain2, chain1) {
		t.Fatalf("turn-2 chain %q must extend turn-1 chain %q (longest-prefix property)", chain2, chain1)
	}
	// The earlier chain must be among the later chain's candidate lookup keys.
	found := false
	for _, candidate := range sessionaffinitycontract.CandidateKeys(chain2) {
		if candidate == chain1 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("turn-1 chain %q not among candidate keys of turn-2 chain", chain1)
	}
}

func TestBuildGatewayDigestChainEmptyForNonConversational(t *testing.T) {
	if chain := buildGatewayDigestChain(gatewaycontract.CanonicalRequest{}); chain != "" {
		t.Fatalf("expected empty chain for empty request, got %q", chain)
	}
}

func TestDeriveGatewaySessionAffinityCascade(t *testing.T) {
	conversational := gatewaycontract.CanonicalRequest{
		Messages: []gatewaycontract.Message{
			{Role: "user", Content: []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Text: "hello"}}},
		},
	}

	t.Run("anthropic metadata wins", func(t *testing.T) {
		req := conversational
		req.RawBody = []byte(`{"metadata":{"user_id":"{\"device_id\":\"d\",\"session_id\":\"s\"}"},"messages":[]}`)
		key, source := deriveGatewaySessionAffinity(httptest.NewRequest("POST", "/v1/messages", nil), req)
		if !strings.HasPrefix(key, "sid:auid:") || source != "derived:anthropic_metadata" {
			t.Fatalf("expected anthropic metadata key, got key=%q source=%q", key, source)
		}
	})

	t.Run("prompt_cache_key", func(t *testing.T) {
		req := conversational
		req.RawBody = []byte(`{"prompt_cache_key":"conv-123"}`)
		key, source := deriveGatewaySessionAffinity(httptest.NewRequest("POST", "/v1/chat/completions", nil), req)
		if !strings.HasPrefix(key, "sid:pck:") || source != "derived:prompt_cache_key" {
			t.Fatalf("expected prompt_cache_key, got key=%q source=%q", key, source)
		}
	})

	t.Run("codex client metadata", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			body   string
			source string
		}{
			{
				name:   "turn metadata prompt cache key",
				body:   `{"client_metadata":{"x-codex-turn-metadata":"{\"prompt_cache_key\":\"cache-1\",\"window_id\":\"window-1\"}"}}`,
				source: "derived:codex_turn_metadata_prompt_cache_key",
			},
			{
				name:   "turn metadata window id",
				body:   `{"client_metadata":{"x-codex-turn-metadata":"{\"window_id\":\"window-1\"}"}}`,
				source: "derived:codex_turn_metadata_window_id",
			},
			{
				name:   "window id",
				body:   `{"client_metadata":{"x-codex-window-id":"window-1"}}`,
				source: "derived:codex_window_id",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				req := conversational
				req.RawBody = []byte(tc.body)
				key, source := deriveGatewaySessionAffinity(httptest.NewRequest("POST", "/v1/responses", nil), req)
				if !strings.HasPrefix(key, "sid:") || source != tc.source {
					t.Fatalf("expected %s key, got key=%q source=%q", tc.source, key, source)
				}
			})
		}
	})

	t.Run("previous response id", func(t *testing.T) {
		req := conversational
		req.RawBody = []byte(`{"previous_response_id":"resp_previous"}`)
		key, source := deriveGatewaySessionAffinity(httptest.NewRequest("POST", "/v1/responses", nil), req)
		if !strings.HasPrefix(key, "sid:prev:") || source != "derived:previous_response_id" {
			t.Fatalf("expected previous_response_id key, got key=%q source=%q", key, source)
		}
	})

	t.Run("response input items path id", func(t *testing.T) {
		req := gatewaycontract.CanonicalRequest{SourceEndpoint: string(gatewaycontract.EndpointResponseInputItems)}
		httpReq := httptest.NewRequest("GET", "/v1/responses/resp_previous/input_items?model=gpt-5", nil)
		httpReq.SetPathValue("response_id", "resp_previous")
		key, source := deriveGatewaySessionAffinity(httpReq, req)
		if !strings.HasPrefix(key, "sid:prev:") || source != "derived:response_id_path" {
			t.Fatalf("expected response_id path key, got key=%q source=%q", key, source)
		}
	})

	t.Run("prompt cache key matches codex turn metadata", func(t *testing.T) {
		reqWithPromptCacheKey := conversational
		reqWithPromptCacheKey.RawBody = []byte(`{"prompt_cache_key":"cache-1"}`)
		promptKey, _ := deriveGatewaySessionAffinity(httptest.NewRequest("POST", "/v1/responses", nil), reqWithPromptCacheKey)

		reqWithTurnMetadata := conversational
		reqWithTurnMetadata.RawBody = []byte(`{"client_metadata":{"x-codex-turn-metadata":"{\"prompt_cache_key\":\"cache-1\"}"}}`)
		metadataKey, source := deriveGatewaySessionAffinity(httptest.NewRequest("POST", "/v1/responses", nil), reqWithTurnMetadata)

		if metadataKey != promptKey || source != "derived:codex_turn_metadata_prompt_cache_key" {
			t.Fatalf("expected matching prompt cache key, got prompt=%q metadata=%q source=%q", promptKey, metadataKey, source)
		}
	})

	t.Run("session header", func(t *testing.T) {
		req := conversational
		req.RawBody = []byte(`{}`)
		httpReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		httpReq.Header.Set("X-Session-Id", "abc")
		key, source := deriveGatewaySessionAffinity(httpReq, req)
		if !strings.HasPrefix(key, "sid:hdr:") || source != "derived:x_session_id" {
			t.Fatalf("expected session header key, got key=%q source=%q", key, source)
		}
	})

	t.Run("cli session headers", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			header string
			source string
		}{
			{name: "codex underscore session", header: "Session_id", source: "derived:session_id"},
			{name: "codex lowercase session", header: "session_id", source: "derived:session_id"},
			{name: "amp thread", header: "X-Amp-Thread-Id", source: "derived:amp_thread_id"},
			{name: "client request", header: "X-Client-Request-Id", source: "derived:client_request_id"},
			{name: "conversation underscore", header: "Conversation_id", source: "derived:conversation_id"},
			{name: "thread", header: "Thread-Id", source: "derived:thread_id"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				req := conversational
				req.RawBody = []byte(`{}`)
				httpReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)
				httpReq.Header.Set(tc.header, "cli-session-123")
				key, source := deriveGatewaySessionAffinity(httpReq, req)
				if !strings.HasPrefix(key, "sid:") || source != tc.source {
					t.Fatalf("expected %s key, got key=%q source=%q", tc.source, key, source)
				}
			})
		}
	})

	t.Run("codex metadata headers", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			header string
			value  string
			source string
		}{
			{
				name:   "turn metadata prompt cache key",
				header: "X-Codex-Turn-Metadata",
				value:  `{"prompt_cache_key":"cache-1","window_id":"window-1"}`,
				source: "derived:codex_turn_metadata_prompt_cache_key",
			},
			{
				name:   "turn metadata window id",
				header: "X-Codex-Turn-Metadata",
				value:  `{"window_id":"window-1"}`,
				source: "derived:codex_turn_metadata_window_id",
			},
			{
				name:   "window id",
				header: "X-Codex-Window-Id",
				value:  "window-1",
				source: "derived:codex_window_id",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				req := conversational
				req.RawBody = []byte(`{}`)
				httpReq := httptest.NewRequest("POST", "/v1/responses", nil)
				httpReq.Header.Set(tc.header, tc.value)
				key, source := deriveGatewaySessionAffinity(httpReq, req)
				if !strings.HasPrefix(key, "sid:") || source != tc.source {
					t.Fatalf("expected %s key, got key=%q source=%q", tc.source, key, source)
				}
			})
		}
	})

	t.Run("invalid codex turn metadata falls through", func(t *testing.T) {
		req := conversational
		req.RawBody = []byte(`{"client_metadata":{"x-codex-turn-metadata":"not-json"}}`)
		key, source := deriveGatewaySessionAffinity(httptest.NewRequest("POST", "/v1/responses", nil), req)
		if !strings.HasPrefix(key, sessionaffinitycontract.ChainMarker) || source != "derived:content_digest" {
			t.Fatalf("expected digest chain fallback, got key=%q source=%q", key, source)
		}
	})

	t.Run("digest chain fallback", func(t *testing.T) {
		req := conversational
		req.RawBody = []byte(`{}`)
		key, source := deriveGatewaySessionAffinity(httptest.NewRequest("POST", "/v1/chat/completions", nil), req)
		if !strings.HasPrefix(key, sessionaffinitycontract.ChainMarker) || source != "derived:content_digest" {
			t.Fatalf("expected digest chain fallback, got key=%q source=%q", key, source)
		}
	})

	t.Run("none for non-conversational", func(t *testing.T) {
		key, source := deriveGatewaySessionAffinity(httptest.NewRequest("POST", "/v1/embeddings", nil), gatewaycontract.CanonicalRequest{RawBody: []byte(`{}`)})
		if key != "" || source != "" {
			t.Fatalf("expected no derived key, got key=%q source=%q", key, source)
		}
	})
}
