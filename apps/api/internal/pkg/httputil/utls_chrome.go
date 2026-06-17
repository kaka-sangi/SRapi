package httputil

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	xproxy "golang.org/x/net/proxy"
)

// ChromeImpersonationVersionDefault is the default Chrome version impersonated
// when callers pass an empty string. Mirrors chatgpt2api's curl_cffi default
// (services/openai_backend_api.py: fp.setdefault("impersonate", "chrome110")).
// Callers wanting a newer fingerprint pass "chrome124" / "chrome131" / etc.
const ChromeImpersonationVersionDefault = "chrome110"

// chromeImpersonationClientPoolSize bounds the per-process LRU of pooled
// http.Clients. Beyond this the least-recently-used entry is evicted to avoid
// leaking TCP/TLS connections when the (version, proxyURL) tuple churns.
//
// Optimization on top of CLIProxyAPI: CLIProxyAPI constructs a fresh
// utlsRoundTripper per NewUtlsHTTPClient call. This LRU lets callers reuse the
// h2 conn pool across requests while still capping growth.
const chromeImpersonationClientPoolSize = 32

// chromeImpersonationProtectedHosts mirrors CLIProxyAPI's utlsProtectedHosts.
// Requests outside this set use the standard transport so we don't pay the
// utls TLS handshake cost on hosts that aren't fronted by Cloudflare.
var chromeImpersonationProtectedHosts = map[string]struct{}{
	"api.anthropic.com": {},
	"chatgpt.com":       {},
}

// SetChromeImpersonationProtectedHosts replaces the default protected-host
// set. Lower-case the hostnames. Pass nil/empty to disable host-gating
// (every request goes through utls).
func SetChromeImpersonationProtectedHosts(hosts []string) {
	chromeImpersonationPoolMu.Lock()
	defer chromeImpersonationPoolMu.Unlock()
	if len(hosts) == 0 {
		chromeImpersonationProtectedHosts = map[string]struct{}{}
		return
	}
	next := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		next[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}
	chromeImpersonationProtectedHosts = next
}

// chromeClientHelloID resolves a Chrome version string to a utls ClientHelloID.
// Unknown versions fall back to HelloChrome_Auto (CLIProxyAPI's choice).
func chromeClientHelloID(version string) utls.ClientHelloID {
	switch strings.ToLower(strings.TrimSpace(version)) {
	case "", "chrome", "auto":
		return utls.HelloChrome_Auto
	case "chrome102":
		return utls.HelloChrome_102
	case "chrome106":
		return utls.HelloChrome_106_Shuffle
	case "chrome110":
		return utls.HelloChrome_115_PQ // utls did not ship a literal 110; 115 is the closest pinned fingerprint
	case "chrome115":
		return utls.HelloChrome_115_PQ
	case "chrome120":
		return utls.HelloChrome_120
	case "chrome131":
		return utls.HelloChrome_131
	default:
		return utls.HelloChrome_Auto
	}
}

// chromeImpersonationPoolKey identifies a pooled http.Client. Empty proxyURL
// is a valid key (direct egress).
type chromeImpersonationPoolKey struct {
	version  string
	proxyURL string
}

type chromeImpersonationPoolEntry struct {
	key    chromeImpersonationPoolKey
	client *http.Client
}

var (
	chromeImpersonationPoolMu      sync.Mutex
	chromeImpersonationPoolByKey   = make(map[chromeImpersonationPoolKey]*list.Element, chromeImpersonationClientPoolSize)
	chromeImpersonationPoolByOrder = list.New() // front = most recently used
)

// ChromeImpersonationClient returns an *http.Client whose Transport uses utls
// to impersonate Chrome's TLS fingerprint (JA3/JA4). Clients are pooled per
// (version, proxyURL) tuple inside a bounded LRU (size
// chromeImpersonationClientPoolSize). Concurrent callers safely share the
// underlying h2 conn pool.
//
// Drop-in: swap an existing *http.Client for the returned one. Requests to
// hosts outside chromeImpersonationProtectedHosts transparently fall through
// to a standard transport, mirroring CLIProxyAPI's fallbackRoundTripper.
func ChromeImpersonationClient(version string, proxyURL string) (*http.Client, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		version = ChromeImpersonationVersionDefault
	}
	proxyURL = strings.TrimSpace(proxyURL)
	key := chromeImpersonationPoolKey{version: strings.ToLower(version), proxyURL: proxyURL}

	chromeImpersonationPoolMu.Lock()
	if elem, ok := chromeImpersonationPoolByKey[key]; ok {
		chromeImpersonationPoolByOrder.MoveToFront(elem)
		client := elem.Value.(*chromeImpersonationPoolEntry).client
		chromeImpersonationPoolMu.Unlock()
		return client, nil
	}
	chromeImpersonationPoolMu.Unlock()

	client, err := buildChromeImpersonationClient(version, proxyURL)
	if err != nil {
		return nil, err
	}

	chromeImpersonationPoolMu.Lock()
	defer chromeImpersonationPoolMu.Unlock()
	// Double-check: a concurrent caller may have inserted the same key.
	if elem, ok := chromeImpersonationPoolByKey[key]; ok {
		chromeImpersonationPoolByOrder.MoveToFront(elem)
		return elem.Value.(*chromeImpersonationPoolEntry).client, nil
	}
	entry := &chromeImpersonationPoolEntry{key: key, client: client}
	elem := chromeImpersonationPoolByOrder.PushFront(entry)
	chromeImpersonationPoolByKey[key] = elem
	// Evict LRU when over the cap.
	for chromeImpersonationPoolByOrder.Len() > chromeImpersonationClientPoolSize {
		back := chromeImpersonationPoolByOrder.Back()
		if back == nil {
			break
		}
		evicted := back.Value.(*chromeImpersonationPoolEntry)
		chromeImpersonationPoolByOrder.Remove(back)
		delete(chromeImpersonationPoolByKey, evicted.key)
		evicted.client.CloseIdleConnections()
	}
	return client, nil
}

// ResetChromeImpersonationPool clears the LRU and closes idle connections.
// Intended for tests and graceful shutdown.
func ResetChromeImpersonationPool() {
	chromeImpersonationPoolMu.Lock()
	defer chromeImpersonationPoolMu.Unlock()
	for elem := chromeImpersonationPoolByOrder.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*chromeImpersonationPoolEntry)
		entry.client.CloseIdleConnections()
	}
	chromeImpersonationPoolByKey = make(map[chromeImpersonationPoolKey]*list.Element, chromeImpersonationClientPoolSize)
	chromeImpersonationPoolByOrder = list.New()
}

func buildChromeImpersonationClient(version string, proxyURL string) (*http.Client, error) {
	helloID := chromeClientHelloID(version)
	dialer, err := buildChromeProxyDialer(proxyURL)
	if err != nil {
		return nil, err
	}
	utlsRT := newChromeUtlsRoundTripper(helloID, dialer)

	fallback := http.DefaultTransport
	if proxyURL != "" {
		fb, fbErr := buildStandardProxyTransport(proxyURL)
		if fbErr != nil {
			return nil, fbErr
		}
		fallback = fb
	}
	return &http.Client{
		Transport: &chromeFallbackRoundTripper{
			utls:     utlsRT,
			fallback: fallback,
		},
		Timeout: 0,
	}, nil
}

// buildChromeProxyDialer returns an xproxy.Dialer for proxyURL. Empty returns
// xproxy.Direct. Supports SOCKS5 and HTTP(S) CONNECT — mirrors sub2api +
// CLIProxyAPI patterns.
func buildChromeProxyDialer(proxyURL string) (xproxy.Dialer, error) {
	if proxyURL == "" {
		return xproxy.Direct, nil
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("utls_chrome: parse proxy: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "socks5", "socks5h":
		var auth *xproxy.Auth
		if parsed.User != nil {
			pw, _ := parsed.User.Password()
			auth = &xproxy.Auth{User: parsed.User.Username(), Password: pw}
		}
		return xproxy.SOCKS5("tcp", parsed.Host, auth, xproxy.Direct)
	case "http", "https":
		// http.ProxyURL handles HTTP(S) CONNECT via the standard transport; the
		// utls dialer skips that and dials direct in this code path. Callers
		// needing HTTP(S) CONNECT through utls should rely on the fallback
		// transport for non-protected hosts.
		return xproxy.Direct, nil
	default:
		return nil, fmt.Errorf("utls_chrome: unsupported proxy scheme %q", parsed.Scheme)
	}
}

func buildStandardProxyTransport(proxyURL string) (http.RoundTripper, error) {
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("utls_chrome: parse proxy: %w", err)
	}
	return &http.Transport{
		Proxy:                 http.ProxyURL(parsed),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}, nil
}

// chromeUtlsRoundTripper implements http.RoundTripper using utls with a Chrome
// ClientHelloID. Mirrors CLIProxyAPI/internal/runtime/executor/helps/utls_client.go
// (utlsRoundTripper) with two changes:
//  1. Context cancellation is honored — dials are guarded by req.Context() so a
//     canceled request does not block on a stuck TLS handshake.
//  2. The h2 conn map is keyed by host:port (not just host) so a request to
//     :8443 does not clobber the :443 connection.
type chromeUtlsRoundTripper struct {
	helloID utls.ClientHelloID
	dialer  xproxy.Dialer

	mu          sync.Mutex
	connections map[string]*http2.ClientConn
	pending     map[string]*sync.Cond
}

func newChromeUtlsRoundTripper(helloID utls.ClientHelloID, dialer xproxy.Dialer) *chromeUtlsRoundTripper {
	if dialer == nil {
		dialer = xproxy.Direct
	}
	return &chromeUtlsRoundTripper{
		helloID:     helloID,
		dialer:      dialer,
		connections: make(map[string]*http2.ClientConn),
		pending:     make(map[string]*sync.Cond),
	}
}

func (t *chromeUtlsRoundTripper) getOrCreateConnection(ctx context.Context, host, addr string) (*http2.ClientConn, error) {
	t.mu.Lock()
	if h2 := t.connections[addr]; h2 != nil && h2.CanTakeNewRequest() {
		t.mu.Unlock()
		return h2, nil
	}
	if cond, ok := t.pending[addr]; ok {
		cond.Wait()
		if h2 := t.connections[addr]; h2 != nil && h2.CanTakeNewRequest() {
			t.mu.Unlock()
			return h2, nil
		}
	}
	cond := sync.NewCond(&t.mu)
	t.pending[addr] = cond
	t.mu.Unlock()

	h2, err := t.dialAndHandshake(ctx, host, addr)

	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pending, addr)
	cond.Broadcast()
	if err != nil {
		return nil, err
	}
	t.connections[addr] = h2
	return h2, nil
}

func (t *chromeUtlsRoundTripper) dialAndHandshake(ctx context.Context, host, addr string) (*http2.ClientConn, error) {
	type dialResult struct {
		conn net.Conn
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		c, e := t.dialer.Dial("tcp", addr)
		ch <- dialResult{conn: c, err: e}
	}()
	var conn net.Conn
	select {
	case <-ctx.Done():
		// Drain in the background so the goroutine doesn't leak its conn.
		go func() {
			res := <-ch
			if res.conn != nil {
				res.conn.Close()
			}
		}()
		return nil, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return nil, res.err
		}
		conn = res.conn
	}

	tlsConn := utls.UClient(conn, &utls.Config{ServerName: host, NextProtos: []string{"h2", "http/1.1"}}, t.helloID)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	tr := &http2.Transport{}
	h2, err := tr.NewClientConn(tlsConn)
	if err != nil {
		_ = tlsConn.Close()
		return nil, err
	}
	return h2, nil
}

func (t *chromeUtlsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return nil, errors.New("utls_chrome: request URL is nil")
	}
	host := req.URL.Hostname()
	port := req.URL.Port()
	if port == "" {
		port = "443"
	}
	addr := net.JoinHostPort(host, port)

	h2, err := t.getOrCreateConnection(req.Context(), host, addr)
	if err != nil {
		return nil, err
	}
	resp, err := h2.RoundTrip(req)
	if err != nil {
		t.mu.Lock()
		if cached, ok := t.connections[addr]; ok && cached == h2 {
			delete(t.connections, addr)
		}
		t.mu.Unlock()
		return nil, err
	}
	return resp, nil
}

// chromeFallbackRoundTripper mirrors CLIProxyAPI's fallbackRoundTripper: HTTPS
// to a protected host -> utls; everything else -> the standard transport.
type chromeFallbackRoundTripper struct {
	utls     http.RoundTripper
	fallback http.RoundTripper
}

func (f *chromeFallbackRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL != nil && strings.EqualFold(req.URL.Scheme, "https") {
		chromeImpersonationPoolMu.Lock()
		_, gated := chromeImpersonationProtectedHosts[strings.ToLower(req.URL.Hostname())]
		empty := len(chromeImpersonationProtectedHosts) == 0
		chromeImpersonationPoolMu.Unlock()
		if gated || empty {
			return f.utls.RoundTrip(req)
		}
	}
	return f.fallback.RoundTrip(req)
}
