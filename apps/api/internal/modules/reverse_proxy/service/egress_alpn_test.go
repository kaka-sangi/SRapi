package service

import (
	"reflect"
	"testing"

	utls "github.com/refraction-networking/utls"

	"github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

// TestDefaultALPNForPolicy pins the ALPN defaulting ported from sub2api: prefer_h2
// (and the implicit default/auto) offers HTTP/2 ahead of HTTP/1.1, while explicit
// HTTP/1 policies advertise only http/1.1.
func TestDefaultALPNForPolicy(t *testing.T) {
	cases := []struct {
		policy string
		want   []string
	}{
		{"", []string{"h2", "http/1.1"}},
		{"auto", []string{"h2", "http/1.1"}},
		{"prefer_h2", []string{"h2", "http/1.1"}},
		{"prefer_http2", []string{"h2", "http/1.1"}},
		{"prefer_h1", []string{"http/1.1"}},
		{"prefer_http1", []string{"http/1.1"}},
		{"require_h1", []string{"http/1.1"}},
		{"require_http1", []string{"http/1.1"}},
	}
	for _, tc := range cases {
		if got := defaultALPNForPolicy(tc.policy); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("defaultALPNForPolicy(%q) = %v, want %v", tc.policy, got, tc.want)
		}
	}
}

// TestResolveEgressProfileDefaultsALPN confirms resolveEgressProfile derives the
// ALPN list from the (possibly defaulted) HTTP version policy.
func TestResolveEgressProfileDefaultsALPN(t *testing.T) {
	cases := []struct {
		name     string
		metadata map[string]any
		want     []string
	}{
		{
			name:     "no override defaults to prefer_h2 ALPN",
			metadata: map[string]any{"egress_tls_template": "chrome"},
			want:     []string{"h2", "http/1.1"},
		},
		{
			name:     "prefer_h1 advertises http/1.1 only",
			metadata: map[string]any{"egress_tls_template": "chrome", "egress_http_version_policy": "prefer_h1"},
			want:     []string{"http/1.1"},
		},
		{
			name:     "require_h1 advertises http/1.1 only",
			metadata: map[string]any{"egress_http_version_policy": "require_h1"},
			want:     []string{"http/1.1"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile, err := resolveEgressProfile(contract.AccountRuntime{Metadata: tc.metadata})
			if err != nil {
				t.Fatalf("resolveEgressProfile: %v", err)
			}
			if !reflect.DeepEqual(profile.ALPNProtocols, tc.want) {
				t.Fatalf("ALPNProtocols = %v, want %v", profile.ALPNProtocols, tc.want)
			}
		})
	}
}

// TestForbidsHTTP2 verifies only require_h1 bans HTTP/2 outright; prefer_* keep it
// available and express preference through ALPN ordering.
func TestForbidsHTTP2(t *testing.T) {
	cases := []struct {
		policy string
		want   bool
	}{
		{"", false},
		{"prefer_h2", false},
		{"prefer_h1", false},
		{"require_h1", true},
		{"require_http1", true},
	}
	for _, tc := range cases {
		got := egressProfile{HTTPVersionPolicy: tc.policy}.forbidsHTTP2()
		if got != tc.want {
			t.Errorf("forbidsHTTP2(%q) = %v, want %v", tc.policy, got, tc.want)
		}
	}
}

// TestAlpnProtocolsFallback confirms the effective getter falls back to http/1.1
// when unset, mirroring sub2api's empty-means-default behavior.
func TestAlpnProtocolsFallback(t *testing.T) {
	if got := (egressProfile{}).alpnProtocols(); !reflect.DeepEqual(got, []string{"http/1.1"}) {
		t.Fatalf("empty profile alpnProtocols() = %v, want [http/1.1]", got)
	}
	custom := []string{"h2", "http/1.1"}
	if got := (egressProfile{ALPNProtocols: custom}).alpnProtocols(); !reflect.DeepEqual(got, custom) {
		t.Fatalf("custom profile alpnProtocols() = %v, want %v", got, custom)
	}
}

// TestClientHelloSpecForHTTP1AppliesALPN verifies the ClientHello ALPN extension is
// rewritten with the provided protocol list instead of a hardcoded http/1.1.
func TestClientHelloSpecForHTTP1AppliesALPN(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"h2 first", []string{"h2", "http/1.1"}, []string{"h2", "http/1.1"}},
		{"h1 only", []string{"http/1.1"}, []string{"http/1.1"}},
		{"empty falls back", nil, []string{"http/1.1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := clientHelloSpecForHTTP1(utls.HelloChrome_Auto, tc.in)
			if err != nil {
				t.Fatalf("clientHelloSpecForHTTP1: %v", err)
			}
			found := false
			for _, extension := range spec.Extensions {
				if alpn, ok := extension.(*utls.ALPNExtension); ok {
					found = true
					if !reflect.DeepEqual(alpn.AlpnProtocols, tc.want) {
						t.Fatalf("ALPN extension = %v, want %v", alpn.AlpnProtocols, tc.want)
					}
				}
			}
			if !found {
				t.Fatal("expected an ALPN extension in the Chrome ClientHello spec")
			}
		})
	}
}

// TestUTLSConfigForHTTP1NextProtos verifies the uTLS config advertises the supplied
// ALPN list (with the http/1.1 fallback) in NextProtos.
func TestUTLSConfigForHTTP1NextProtos(t *testing.T) {
	if got := utlsConfigForHTTP1("example.com", []string{"h2", "http/1.1"}, nil).NextProtos; !reflect.DeepEqual(got, []string{"h2", "http/1.1"}) {
		t.Fatalf("NextProtos = %v, want [h2 http/1.1]", got)
	}
	if got := utlsConfigForHTTP1("example.com", nil, nil).NextProtos; !reflect.DeepEqual(got, []string{"http/1.1"}) {
		t.Fatalf("fallback NextProtos = %v, want [http/1.1]", got)
	}
}
