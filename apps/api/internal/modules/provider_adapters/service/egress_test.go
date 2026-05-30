package service

import (
	"net/http"
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	reverseproxyservice "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/service"
)

func TestEgressHTTPClientGating(t *testing.T) {
	reverseProxy, err := reverseproxyservice.New(nil)
	if err != nil {
		t.Fatalf("new reverse proxy: %v", err)
	}
	shared := &http.Client{}
	svc, err := NewWithReverseProxy(shared, reverseProxy)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	// An account with no egress config keeps the shared client (byte-identical path).
	plain := accountcontract.ProviderAccount{ID: 1, RuntimeClass: accountcontract.RuntimeClassAPIKey}
	if got := svc.egressHTTPClient(plain, nil); got != shared {
		t.Fatalf("account without egress config must use the shared client")
	}

	// A proxy-configured account routes through the managed egress client.
	proxy := "http://proxy.example:8080"
	proxied := accountcontract.ProviderAccount{ID: 2, RuntimeClass: accountcontract.RuntimeClassAPIKey, ProxyID: &proxy}
	if got := svc.egressHTTPClient(proxied, nil); got == shared {
		t.Fatalf("proxy-configured account must use the managed egress client, not the shared one")
	}

	// A TLS-fingerprint account routes through the managed egress client.
	tls := accountcontract.ProviderAccount{
		ID:           3,
		RuntimeClass: accountcontract.RuntimeClassAPIKey,
		Metadata:     map[string]any{"egress_profile": map[string]any{"tls_template": "chrome_120"}},
	}
	if got := svc.egressHTTPClient(tls, nil); got == shared {
		t.Fatalf("tls-profile account must use the managed egress client, not the shared one")
	}
}
