package service

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func TestManagedEgressClient(t *testing.T) {
	svc, err := New(nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	// No egress config: not managed, caller keeps its own client.
	if client, managed, err := svc.ManagedEgressClient(contract.AccountRuntime{AccountID: 1, RuntimeClass: "api_key"}); err != nil || managed || client != nil {
		t.Fatalf("plain account should not be managed, got client!=nil=%v managed=%v err=%v", client != nil, managed, err)
	}

	// Proxy configured: managed egress client.
	proxy := "http://proxy.example:8080"
	client, managed, err := svc.ManagedEgressClient(contract.AccountRuntime{AccountID: 2, RuntimeClass: "api_key", ProxyID: &proxy})
	if err != nil || !managed || client == nil {
		t.Fatalf("proxy account should be managed, got managed=%v client!=nil=%v err=%v", managed, client != nil, err)
	}

	// TLS-fingerprint profile configured: managed egress client.
	client, managed, err = svc.ManagedEgressClient(contract.AccountRuntime{
		AccountID:    3,
		RuntimeClass: "api_key",
		Metadata:     map[string]any{"egress_profile": map[string]any{"tls_template": "chrome_120"}},
	})
	if err != nil || !managed || client == nil {
		t.Fatalf("tls-profile account should be managed, got managed=%v client!=nil=%v err=%v", managed, client != nil, err)
	}
}
