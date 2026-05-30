package service

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

func TestBlockedEgressIPClassifiesPrivateRanges(t *testing.T) {
	blocked := []string{
		"127.0.0.1",       // loopback
		"::1",             // loopback v6
		"0.0.0.0",         // unspecified
		"10.0.0.1",        // RFC1918
		"172.16.5.4",      // RFC1918
		"192.168.1.1",     // RFC1918
		"169.254.169.254", // link-local / cloud metadata
		"169.254.0.1",     // link-local
		"100.64.0.1",      // RFC6598 CGNAT
		"100.127.255.255", // RFC6598 CGNAT upper
		"fc00::1",         // ULA
		"fe80::1",         // link-local v6
		"224.0.0.1",       // multicast
	}
	for _, raw := range blocked {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("bad test IP %q", raw)
		}
		if !blockedEgressIP(ip) {
			t.Fatalf("expected %s to be blocked egress", raw)
		}
	}

	allowed := []string{
		"8.8.8.8",
		"1.1.1.1",
		"100.63.255.255",       // just below CGNAT
		"100.128.0.0",          // just above CGNAT
		"2606:4700:4700::1111", // public v6
	}
	for _, raw := range allowed {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("bad test IP %q", raw)
		}
		if blockedEgressIP(ip) {
			t.Fatalf("expected %s to be allowed egress", raw)
		}
	}
}

func TestEgressGuardBlocksLoopbackDirectDialWhenEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	guarded, err := newIsolatedClient(contract.AccountRuntime{}, true)
	if err != nil {
		t.Fatalf("build guarded client: %v", err)
	}
	if resp, err := guarded.Get(srv.URL); err == nil {
		_ = resp.Body.Close()
		t.Fatalf("expected guarded egress to loopback %s to be blocked", srv.URL)
	} else if !strings.Contains(strings.ToLower(err.Error()), "egress") {
		t.Fatalf("expected egress-blocked dial error, got %v", err)
	}

	open, err := newIsolatedClient(contract.AccountRuntime{}, false)
	if err != nil {
		t.Fatalf("build unguarded client: %v", err)
	}
	resp, err := open.Get(srv.URL)
	if err != nil {
		t.Fatalf("expected unguarded client to reach loopback test server: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "ok" {
		t.Fatalf("expected loopback response body, got %q", string(body))
	}
}
