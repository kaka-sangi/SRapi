package httpserver

import (
	"context"
	"net/http"
	"strings"
)

// hopByHopResponseHeaders are connection-management headers that are meaningful
// only for a single transport hop and must never be forwarded from the upstream
// response to the downstream client (RFC 7230 §6.1 plus the headers SRapi's own
// response framing owns). They are dropped even when explicitly allowlisted.
var hopByHopResponseHeaders = map[string]struct{}{
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailer":             {},
	"transfer-encoding":   {},
	"upgrade":             {},
	// Body framing / encoding headers: SRapi re-frames the response body, so a
	// forwarded value would describe the upstream body, not what the client
	// receives.
	"content-length":   {},
	"content-encoding": {},
	"content-type":     {},
}

// gatewayPassthroughHeaderConfig is the resolved, hot-path-friendly view of the
// admin gateway header-passthrough settings.
type gatewayPassthroughHeaderConfig struct {
	enabled   bool
	allowlist []string // canonical lowercase entries; may contain "x-ratelimit-*" prefixes
}

// gatewayPassthroughHeaderConfig loads the header-passthrough settings. It
// returns a disabled config (and forwards nothing) on any error or when the
// feature is off, so the hot response path never fails because of settings.
func (s *Server) gatewayPassthroughHeaderConfig(ctx context.Context) gatewayPassthroughHeaderConfig {
	if s == nil || s.runtime == nil || s.runtime.adminControl == nil {
		return gatewayPassthroughHeaderConfig{}
	}
	settings, err := s.runtime.adminControl.GetAdminSettings(ctx)
	if err != nil {
		return gatewayPassthroughHeaderConfig{}
	}
	gateway := settings.Gateway
	if !gateway.PassthroughUpstreamHeaders || len(gateway.PassthroughHeaderAllowlist) == 0 {
		return gatewayPassthroughHeaderConfig{}
	}
	return gatewayPassthroughHeaderConfig{enabled: true, allowlist: gateway.PassthroughHeaderAllowlist}
}

// forwardBufferedPassthroughHeaders forwards allowlisted upstream response
// headers on the buffered same-protocol passthrough paths. It loads the gateway
// settings once and is a no-op when the feature is disabled or the adapter
// exposed no upstream headers. Call it before writing the response status.
func (s *Server) forwardBufferedPassthroughHeaders(w http.ResponseWriter, r *http.Request, upstream http.Header) {
	if len(upstream) == 0 {
		return
	}
	forwardUpstreamResponseHeaders(w, upstream, s.gatewayPassthroughHeaderConfig(r.Context()))
}

// forwardUpstreamResponseHeaders copies allowlisted upstream response headers
// onto w, before w.WriteHeader is called. It is a no-op unless the feature is
// enabled. Hop-by-hop headers are always dropped, and a header SRapi has
// already set on the response is never overridden.
func forwardUpstreamResponseHeaders(w http.ResponseWriter, upstream http.Header, cfg gatewayPassthroughHeaderConfig) {
	if !cfg.enabled || w == nil || len(upstream) == 0 {
		return
	}
	dst := w.Header()
	for name, values := range upstream {
		canonical := strings.ToLower(strings.TrimSpace(name))
		if canonical == "" {
			continue
		}
		if _, hop := hopByHopResponseHeaders[canonical]; hop {
			continue
		}
		if !headerAllowlistMatches(canonical, cfg.allowlist) {
			continue
		}
		// Never override what SRapi already set on the response.
		if dst.Get(name) != "" {
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}

// headerAllowlistMatches reports whether a canonical (lowercased) header name is
// permitted by the allowlist. Entries are matched case-insensitively; a trailing
// "*" is treated as a prefix wildcard (e.g. "x-ratelimit-*" matches
// "x-ratelimit-remaining").
func headerAllowlistMatches(canonical string, allowlist []string) bool {
	for _, entry := range allowlist {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if strings.HasSuffix(entry, "*") {
			if strings.HasPrefix(canonical, strings.TrimSuffix(entry, "*")) {
				return true
			}
			continue
		}
		if canonical == entry {
			return true
		}
	}
	return false
}
