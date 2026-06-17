// Cloudflare clearance provider — ported from chatgpt2api services/proxy_service.py
// (ClearanceBundle + FlareSolverrClearanceProvider + ProxySettingsStore caching
// behaviour).
//
// chatgpt2api uses an unbounded dict keyed on (proxy_url, target_host) for its
// clearance cache. The Go port uses a bounded LRU (256 entries) per the port
// directive's allowed Go-side optimization. TTL semantics match the Python
// reference (a bundle is "valid" iff cookies/UA present, expiry not reached,
// and the (host,proxy) tuple matches).
//
// The ChatGPT web hot path consults this cache when an upstream response is
// detected as a Cloudflare challenge (IsCloudflareChallengeResponse). On a
// cache hit the cookies / UA are injected; on a miss Resolve hits the
// provider (FlareSolverr) and caches the result.
package httputil

import (
	"container/list"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrClearanceProviderNotConfigured is returned by Resolve when the underlying
// FlareSolverr (or other) provider has no URL configured. Callers should treat
// this as a hard configuration error and surface it clearly to the operator.
var ErrClearanceProviderNotConfigured = errors.New("cloudflare clearance provider not configured")

// ErrClearanceResolveFailed is returned by Resolve when the upstream provider
// responded but did not yield usable cookies / user-agent.
var ErrClearanceResolveFailed = errors.New("cloudflare clearance resolution failed")

// ClearanceBundle is the resolved set of cookies + UA + expiry the upstream
// challenge solver returned. Cookies is host-scoped to TargetHost.
type ClearanceBundle struct {
	TargetHost string
	ProxyURL   string
	Cookies    map[string]string
	UserAgent  string
	AcquiredAt time.Time
	ExpiresAt  time.Time
}

// IsValidFor mirrors chatgpt2api ClearanceBundle.is_valid_for.
func (b *ClearanceBundle) IsValidFor(targetHost, proxyURL string, now time.Time) bool {
	if b == nil {
		return false
	}
	host := normalizeHost(targetHost)
	if b.TargetHost != "" && host != "" && normalizeHost(b.TargetHost) != host {
		return false
	}
	if normalizeProxyURL(b.ProxyURL) != normalizeProxyURL(proxyURL) {
		return false
	}
	if !b.ExpiresAt.IsZero() && !now.Before(b.ExpiresAt) {
		return false
	}
	return len(b.Cookies) > 0 || strings.TrimSpace(b.UserAgent) != ""
}

// CookieHeader serialises the Cookies map to a single Cookie header value.
func (b *ClearanceBundle) CookieHeader() string {
	if b == nil {
		return ""
	}
	out := make([]string, 0, len(b.Cookies))
	for name, value := range b.Cookies {
		if name == "" {
			continue
		}
		out = append(out, name+"="+value)
	}
	return strings.Join(out, "; ")
}

// ResolveRequest is the input to ClearanceProvider.Resolve.
type ResolveRequest struct {
	TargetURL string
	ProxyURL  string
	// Timeout overrides the provider default when non-zero.
	Timeout time.Duration
}

// ClearanceProvider is the abstract interface for an upstream CF clearance
// solver. The FlareSolverr-backed implementation is the only concrete one
// shipped today; a manual / cookie-only provider can be plugged in by
// implementing the same interface.
type ClearanceProvider interface {
	Resolve(req ResolveRequest) (*ClearanceBundle, error)
}

// ClearanceCacheConfig controls the bounded LRU cache. Defaults match the
// chatgpt2api Python reference except for the bound (Go-side optimization;
// the Python reference is unbounded).
type ClearanceCacheConfig struct {
	// MaxEntries caps the LRU. Zero → DefaultClearanceCacheMaxEntries.
	MaxEntries int
	// TTL is the validity window for cache entries that don't carry an
	// explicit ExpiresAt. Zero → DefaultClearanceCacheTTL.
	TTL time.Duration
	// Now is the time source. Zero → time.Now.
	Now func() time.Time
}

const (
	// DefaultClearanceCacheMaxEntries is the LRU bound (Go-side optimization;
	// chatgpt2api's dict is unbounded). 256 hosts × proxies is more than any
	// real account fleet needs.
	DefaultClearanceCacheMaxEntries = 256
	// DefaultClearanceCacheTTL matches chatgpt2api's refresh_interval default
	// (60 minutes when no explicit interval is configured). A bundle without
	// an explicit ExpiresAt is treated as valid for this window.
	DefaultClearanceCacheTTL = 60 * time.Minute
)

// ClearanceCache is an in-memory bounded LRU keyed on (target_host, proxy_url).
// It is safe for concurrent use.
type ClearanceCache struct {
	mu      sync.Mutex
	entries map[clearanceCacheKey]*list.Element
	order   *list.List
	max     int
	ttl     time.Duration
	now     func() time.Time
}

type clearanceCacheKey struct {
	host  string
	proxy string
}

type clearanceCacheValue struct {
	key    clearanceCacheKey
	bundle *ClearanceBundle
}

// NewClearanceCache constructs a cache with the supplied config. A nil config
// uses the package defaults.
func NewClearanceCache(cfg ClearanceCacheConfig) *ClearanceCache {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = DefaultClearanceCacheMaxEntries
	}
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultClearanceCacheTTL
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &ClearanceCache{
		entries: make(map[clearanceCacheKey]*list.Element),
		order:   list.New(),
		max:     cfg.MaxEntries,
		ttl:     cfg.TTL,
		now:     cfg.Now,
	}
}

// Get returns a cached bundle iff one matches the (host, proxy) tuple AND is
// still valid (cookies/UA present, not expired). Cache misses and expired
// entries both return (nil, false).
func (c *ClearanceCache) Get(targetHost, proxyURL string) (*ClearanceBundle, bool) {
	if c == nil {
		return nil, false
	}
	key := clearanceCacheKey{host: normalizeHost(targetHost), proxy: normalizeProxyURL(proxyURL)}
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	val := elem.Value.(*clearanceCacheValue)
	now := c.now()
	if !val.bundle.IsValidFor(targetHost, proxyURL, now) {
		c.order.Remove(elem)
		delete(c.entries, key)
		return nil, false
	}
	c.order.MoveToFront(elem)
	return val.bundle, true
}

// Put stores a bundle, applying the cache TTL when the bundle does not carry
// its own ExpiresAt.
func (c *ClearanceCache) Put(bundle *ClearanceBundle) {
	if c == nil || bundle == nil {
		return
	}
	key := clearanceCacheKey{
		host:  normalizeHost(bundle.TargetHost),
		proxy: normalizeProxyURL(bundle.ProxyURL),
	}
	now := c.now()
	if bundle.AcquiredAt.IsZero() {
		bundle.AcquiredAt = now
	}
	if bundle.ExpiresAt.IsZero() {
		bundle.ExpiresAt = now.Add(c.ttl)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[key]; ok {
		val := elem.Value.(*clearanceCacheValue)
		val.bundle = bundle
		c.order.MoveToFront(elem)
		return
	}
	val := &clearanceCacheValue{key: key, bundle: bundle}
	elem := c.order.PushFront(val)
	c.entries[key] = elem
	if c.order.Len() > c.max {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.entries, oldest.Value.(*clearanceCacheValue).key)
		}
	}
}

// Invalidate removes any cached bundle for (host, proxy).
func (c *ClearanceCache) Invalidate(targetHost, proxyURL string) {
	if c == nil {
		return
	}
	key := clearanceCacheKey{host: normalizeHost(targetHost), proxy: normalizeProxyURL(proxyURL)}
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[key]; ok {
		c.order.Remove(elem)
		delete(c.entries, key)
	}
}

// Len reports the current entry count. Useful in tests.
func (c *ClearanceCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Clear empties the cache.
func (c *ClearanceCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[clearanceCacheKey]*list.Element)
	c.order = list.New()
}

// HostFromURL extracts and normalises the host component of a URL. Exported
// so callers building a ResolveRequest can derive the host the same way the
// cache does.
func HostFromURL(rawURL string) string {
	candidate := strings.TrimSpace(rawURL)
	if candidate == "" {
		return ""
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Hostname() == "" {
		if !strings.Contains(candidate, "://") {
			parsed, err = url.Parse("https://" + candidate)
			if err != nil {
				return ""
			}
		}
	}
	if parsed == nil {
		return ""
	}
	return normalizeHost(parsed.Hostname())
}

// IsCloudflareChallenge is a convenience wrapper over IsCloudflareChallengeResponse
// that takes a *http.Response and a body buffer. It exists so the chatgpt_web
// hot path can detect a challenge with one call regardless of whether it has
// the resp or the (status, headers, body) triple.
func IsCloudflareChallenge(resp *http.Response, body []byte) bool {
	if resp == nil {
		return false
	}
	return IsCloudflareChallengeResponse(resp.StatusCode, resp.Header, body)
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(host), "."))
}

func normalizeProxyURL(raw string) string {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return ""
	}
	lowered := strings.ToLower(candidate)
	if strings.HasPrefix(lowered, "socks://") {
		return "socks5h://" + candidate[len("socks://"):]
	}
	if strings.HasPrefix(lowered, "socks5://") {
		return "socks5h://" + candidate[len("socks5://"):]
	}
	return candidate
}
