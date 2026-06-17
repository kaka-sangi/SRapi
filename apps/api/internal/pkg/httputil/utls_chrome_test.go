package httputil

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestChromeImpersonationClient_PooledByKey(t *testing.T) {
	ResetChromeImpersonationPool()
	defer ResetChromeImpersonationPool()

	c1, err := ChromeImpersonationClient("chrome120", "")
	if err != nil {
		t.Fatalf("ChromeImpersonationClient err = %v", err)
	}
	c2, err := ChromeImpersonationClient("chrome120", "")
	if err != nil {
		t.Fatalf("ChromeImpersonationClient err = %v", err)
	}
	if c1 != c2 {
		t.Fatalf("expected pooled client, got different instances")
	}

	c3, err := ChromeImpersonationClient("chrome131", "")
	if err != nil {
		t.Fatalf("ChromeImpersonationClient err = %v", err)
	}
	if c3 == c1 {
		t.Fatalf("distinct version should yield distinct client")
	}
}

func TestChromeImpersonationClient_LRUBounded(t *testing.T) {
	ResetChromeImpersonationPool()
	defer ResetChromeImpersonationPool()

	// Insert pool-size + 4 distinct entries; the LRU must stay <= pool size.
	versions := []string{"chrome58", "chrome62", "chrome70", "chrome72", "chrome83", "chrome87", "chrome96", "chrome100", "chrome102", "chrome106", "chrome115", "chrome120", "chrome131"}
	// Pad with synthetic version tags (unknown -> falls back to HelloChrome_Auto
	// but distinct cache keys).
	for i := 0; i < 40; i++ {
		v := versions[i%len(versions)]
		if _, err := ChromeImpersonationClient(v, ""); err != nil {
			t.Fatalf("insert %d (%s) err = %v", i, v, err)
		}
	}
	chromeImpersonationPoolMu.Lock()
	size := chromeImpersonationPoolByOrder.Len()
	chromeImpersonationPoolMu.Unlock()
	if size > chromeImpersonationClientPoolSize {
		t.Fatalf("pool size %d exceeds cap %d", size, chromeImpersonationClientPoolSize)
	}
}

func TestChromeImpersonationClient_DefaultVersion(t *testing.T) {
	ResetChromeImpersonationPool()
	defer ResetChromeImpersonationPool()

	// "" and the documented default must yield the same pooled client.
	c1, err := ChromeImpersonationClient("", "")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	c2, err := ChromeImpersonationClient(ChromeImpersonationVersionDefault, "")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if c1 != c2 {
		t.Fatalf("default version must alias %q", ChromeImpersonationVersionDefault)
	}
}

func TestChromeImpersonationClient_FallbackToStandardForUnprotectedHost(t *testing.T) {
	ResetChromeImpersonationPool()
	defer ResetChromeImpersonationPool()
	SetChromeImpersonationProtectedHosts([]string{"api.anthropic.com"})
	defer SetChromeImpersonationProtectedHosts([]string{"api.anthropic.com", "chatgpt.com"})

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client, err := ChromeImpersonationClient("chrome120", "")
	if err != nil {
		t.Fatalf("ChromeImpersonationClient err = %v", err)
	}
	// httptest TLS server uses a self-signed cert; install its cert via the
	// fallback transport so the non-utls path can complete the handshake.
	fb, ok := client.Transport.(*chromeFallbackRoundTripper)
	if !ok {
		t.Fatalf("transport not a chromeFallbackRoundTripper")
	}
	tr, ok := fb.fallback.(*http.Transport)
	if !ok {
		// http.DefaultTransport is *http.Transport.
		tr, ok = http.DefaultTransport.(*http.Transport)
		if !ok {
			t.Skip("DefaultTransport not *http.Transport; skipping fallback verification")
		}
	}
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test-only
	fb.fallback = tr

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestChromeImpersonationClient_ConcurrentPoolSafe(t *testing.T) {
	ResetChromeImpersonationPool()
	defer ResetChromeImpersonationPool()

	var wg sync.WaitGroup
	const workers = 32
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if _, err := ChromeImpersonationClient("chrome120", ""); err != nil {
				t.Errorf("concurrent err = %v", err)
			}
		}()
	}
	wg.Wait()

	chromeImpersonationPoolMu.Lock()
	defer chromeImpersonationPoolMu.Unlock()
	// All workers used the same key — pool must hold exactly one entry.
	if got := chromeImpersonationPoolByOrder.Len(); got != 1 {
		t.Fatalf("pool size = %d, want 1", got)
	}
}

func TestChromeImpersonationClient_InvalidProxyURL(t *testing.T) {
	ResetChromeImpersonationPool()
	defer ResetChromeImpersonationPool()
	_, err := ChromeImpersonationClient("chrome120", "ftp://nope")
	if err == nil {
		t.Fatalf("expected error for unsupported proxy scheme")
	}
}
