package service

import (
	"net/http"
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

func TestApplyAccountRequestHeadersMergesSafeHeaders(t *testing.T) {
	svc := &Service{}
	headers := http.Header{
		"Authorization": {"Bearer provider-secret"},
		"Content-Type":  {"application/json"},
		"X-Trace":       {"adapter"},
	}
	svc.applyAccountRequestHeaders(headers, accountcontract.ProviderAccount{
		Metadata: map[string]any{
			"egress_profile": map[string]any{
				"extra_static_headers": map[string]any{
					"X-Egress-Static": "static",
					"X-Trace":         "egress",
					"Authorization":   "Bearer leaked",
				},
			},
			"headers": map[string]any{
				"X-Trace":         "account",
				"X-Custom":        " custom ",
				"Authorization":   "Bearer bad",
				"Content-Type":    "text/plain",
				"X-Forwarded-For": "127.0.0.1",
				"X-Empty":         "   ",
				"X-NonString":     123,
			},
		},
	}, nil)

	if headers.Get("Authorization") != "Bearer provider-secret" {
		t.Fatalf("authorization was overwritten: %q", headers.Get("Authorization"))
	}
	if headers.Get("Content-Type") != "application/json" {
		t.Fatalf("content type was overwritten: %q", headers.Get("Content-Type"))
	}
	if headers.Get("X-Egress-Static") != "static" {
		t.Fatalf("missing egress static header: %+v", headers)
	}
	if headers.Get("X-Trace") != "account" || headers.Get("X-Custom") != "custom" {
		t.Fatalf("unexpected merged custom headers: %+v", headers)
	}
	if headers.Get("X-Forwarded-For") != "" || headers.Get("X-Empty") != "" || headers.Get("X-NonString") != "" {
		t.Fatalf("unsafe or invalid metadata headers leaked: %+v", headers)
	}
}

func TestApplyAccountRequestHeadersIfMissingDoesNotOverrideProbeHeaders(t *testing.T) {
	svc := &Service{}
	headers := http.Header{
		"Accept":           {"application/json"},
		"X-Probe-Scenario": {"probe"},
	}
	svc.applyAccountRequestHeadersIfMissing(headers, accountcontract.ProviderAccount{
		Metadata: map[string]any{
			"egress_profile": map[string]any{
				"extra_static_headers": map[string]any{
					"Accept":          "text/event-stream",
					"X-Egress-Static": "static",
				},
			},
			"headers": map[string]string{
				"X-Probe-Scenario": "account",
				"X-Custom":         "custom",
			},
		},
	}, nil)

	if headers.Get("Accept") != "application/json" || headers.Get("X-Probe-Scenario") != "probe" {
		t.Fatalf("existing probe headers were overwritten: %+v", headers)
	}
	if headers.Get("X-Egress-Static") != "static" || headers.Get("X-Custom") != "custom" {
		t.Fatalf("missing account headers: %+v", headers)
	}
}
