// FlareSolverr clearance provider — ported from chatgpt2api
// services/proxy_service.FlareSolverrClearanceProvider.
//
// Reference Python:
//
//	payload = {"cmd":"request.get","url":target,"maxTimeout":timeout*1000}
//	if proxy: payload["proxy"] = {"url": proxy}
//	resp = POST flaresolverr_url + "/v1" json=payload
//	solution = resp["solution"]; cookies (filtered by host) + userAgent → bundle.
//
// Defaults read from env (and fall back to chatgpt2api defaults):
//
//	FLARESOLVERR_URL          — empty → ErrClearanceProviderNotConfigured
//	FLARESOLVERR_SESSION_TTL  — seconds; expiry stamped on returned bundle
//	FLARESOLVERR_TIMEOUT      — seconds; per-request maxTimeout
package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultFlareSolverrTimeout matches chatgpt2api's per-request timeout
	// default (60s; _coerce_timeout floor is 1.0s).
	DefaultFlareSolverrTimeout = 60 * time.Second
	// DefaultFlareSolverrEndpoint matches the docker-compose convention from
	// the chatgpt2api docs.
	DefaultFlareSolverrEndpoint = "http://flaresolverr:8191/v1"
)

// FlareSolverrConfig is the construction-time config for a FlareSolverr
// provider.
type FlareSolverrConfig struct {
	// URL is the base "/v1" endpoint of the FlareSolverr container. Trailing
	// "/v1" is allowed and stripped; an empty URL is honoured and causes
	// Resolve to return ErrClearanceProviderNotConfigured.
	URL string
	// Timeout is the per-request budget. Zero → DefaultFlareSolverrTimeout.
	Timeout time.Duration
	// SessionTTL is the validity window applied to returned bundles. Zero
	// → no explicit ExpiresAt (the cache layer applies its own TTL).
	SessionTTL time.Duration
	// HTTPClient is the transport used to talk to FlareSolverr. Zero → a
	// *http.Client with Timeout set to (Timeout + 5s).
	HTTPClient *http.Client
}

// FlareSolverrProvider is the FlareSolverr-backed ClearanceProvider.
type FlareSolverrProvider struct {
	endpoint   string
	timeout    time.Duration
	sessionTTL time.Duration
	client     *http.Client
}

// NewFlareSolverrProvider builds a provider from config.
func NewFlareSolverrProvider(cfg FlareSolverrConfig) *FlareSolverrProvider {
	endpoint := normaliseFlareSolverrURL(cfg.URL)
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultFlareSolverrTimeout
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout + 5*time.Second}
	}
	return &FlareSolverrProvider{
		endpoint:   endpoint,
		timeout:    timeout,
		sessionTTL: cfg.SessionTTL,
		client:     client,
	}
}

// NewFlareSolverrProviderFromEnv builds a provider from the FLARESOLVERR_*
// env vars. An empty FLARESOLVERR_URL produces a provider whose Resolve
// always returns ErrClearanceProviderNotConfigured.
func NewFlareSolverrProviderFromEnv() *FlareSolverrProvider {
	cfg := FlareSolverrConfig{
		URL: strings.TrimSpace(os.Getenv("FLARESOLVERR_URL")),
	}
	if raw := strings.TrimSpace(os.Getenv("FLARESOLVERR_TIMEOUT")); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			cfg.Timeout = time.Duration(secs) * time.Second
		}
	}
	if raw := strings.TrimSpace(os.Getenv("FLARESOLVERR_SESSION_TTL")); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			cfg.SessionTTL = time.Duration(secs) * time.Second
		}
	}
	return NewFlareSolverrProvider(cfg)
}

// Resolve implements ClearanceProvider. ResolveCtx is the preferred entry
// point for hot-path callers that have a context.
func (p *FlareSolverrProvider) Resolve(req ResolveRequest) (*ClearanceBundle, error) {
	return p.ResolveCtx(context.Background(), req)
}

// ResolveCtx is the context-aware Resolve. Cancelling ctx cancels the
// FlareSolverr HTTP call.
func (p *FlareSolverrProvider) ResolveCtx(ctx context.Context, req ResolveRequest) (*ClearanceBundle, error) {
	if p == nil || p.endpoint == "" {
		return nil, ErrClearanceProviderNotConfigured
	}
	if strings.TrimSpace(req.TargetURL) == "" {
		return nil, fmt.Errorf("%w: empty target_url", ErrClearanceResolveFailed)
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = p.timeout
	}
	payload := map[string]any{
		"cmd":        "request.get",
		"url":        strings.TrimSpace(req.TargetURL),
		"maxTimeout": int(timeout / time.Millisecond),
	}
	proxyURL := normalizeProxyURL(req.ProxyURL)
	if proxyURL != "" {
		payload["proxy"] = map[string]string{"url": proxyURL}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: marshal payload: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: post: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("flaresolverr: http %d: %s", resp.StatusCode, TruncateBody(body, 256))
	}
	var decoded struct {
		Status   string `json:"status"`
		Message  string `json:"message"`
		Solution struct {
			URL       string             `json:"url"`
			UserAgent string             `json:"userAgent"`
			Cookies   []flaresolverrCook `json:"cookies"`
		} `json:"solution"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("flaresolverr: decode body: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(decoded.Status), "ok") {
		message := strings.TrimSpace(decoded.Message)
		if message == "" {
			message = "non-ok status"
		}
		return nil, fmt.Errorf("%w: %s", ErrClearanceResolveFailed, message)
	}
	host := HostFromURL(req.TargetURL)
	cookies := filterFlareSolverrCookies(decoded.Solution.Cookies, host)
	userAgent := strings.TrimSpace(decoded.Solution.UserAgent)
	if len(cookies) == 0 && userAgent == "" {
		return nil, fmt.Errorf("%w: empty solution", ErrClearanceResolveFailed)
	}
	now := time.Now()
	bundle := &ClearanceBundle{
		TargetHost: host,
		ProxyURL:   proxyURL,
		Cookies:    cookies,
		UserAgent:  userAgent,
		AcquiredAt: now,
	}
	if p.sessionTTL > 0 {
		bundle.ExpiresAt = now.Add(p.sessionTTL)
	}
	return bundle, nil
}

// Configured reports whether the provider has a URL and will attempt a real
// Resolve. Useful for callers that want to skip detection entirely when no
// provider is configured.
func (p *FlareSolverrProvider) Configured() bool {
	return p != nil && p.endpoint != ""
}

type flaresolverrCook struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
}

func filterFlareSolverrCookies(in []flaresolverrCook, targetHost string) map[string]string {
	out := map[string]string{}
	for _, c := range in {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			continue
		}
		domain := strings.TrimSpace(c.Domain)
		if domain != "" && !domainMatches(targetHost, domain) {
			continue
		}
		out[name] = c.Value
	}
	return out
}

func domainMatches(host, domain string) bool {
	h := normalizeHost(host)
	d := normalizeHost(strings.TrimPrefix(strings.TrimSpace(domain), "."))
	if d == "" {
		return true
	}
	return h == d || strings.HasSuffix(h, "."+d)
}

func normaliseFlareSolverrURL(raw string) string {
	candidate := strings.TrimSpace(raw)
	if candidate == "" {
		return ""
	}
	candidate = strings.TrimRight(candidate, "/")
	if !strings.HasSuffix(candidate, "/v1") {
		candidate = candidate + "/v1"
	}
	return candidate
}

// Compile-time check that the provider satisfies the interface.
var _ ClearanceProvider = (*FlareSolverrProvider)(nil)

// MergeCookieHeader merges a bundle's cookies into an existing Cookie header.
// Existing cookie names take precedence (matches chatgpt2api's _merge_cookie_header).
func MergeCookieHeader(existing string, cookies map[string]string) string {
	existingNames := map[string]struct{}{}
	for _, part := range strings.Split(existing, ";") {
		name, _, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && name != "" {
			existingNames[strings.TrimSpace(name)] = struct{}{}
		}
	}
	additions := make([]string, 0, len(cookies))
	for name, value := range cookies {
		if name == "" {
			continue
		}
		if _, exists := existingNames[name]; exists {
			continue
		}
		additions = append(additions, name+"="+value)
	}
	existing = strings.TrimSpace(existing)
	if existing != "" && len(additions) > 0 {
		return strings.TrimRight(existing, "; ") + "; " + strings.Join(additions, "; ")
	}
	if existing != "" {
		return existing
	}
	return strings.Join(additions, "; ")
}

// ensure imports stay used even when no FlareSolverr call is made
var _ = errors.New
