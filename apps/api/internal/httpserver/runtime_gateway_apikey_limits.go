package httpserver

import (
	"errors"
	"net/netip"
	"strconv"
	"strings"
	"time"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	"github.com/srapi/srapi/apps/api/internal/platform/ratelimit"
)

// errGatewayKeyIPNotAllowed is returned by gateway auth when the client IP is
// not permitted by the API key's allow/deny lists.
var errGatewayKeyIPNotAllowed = errors.New("api key not permitted from client ip")

// gatewayKeyIPAllowed enforces an API key's IP allow/deny lists against the
// resolved client IP. Deny entries take precedence; a non-empty allow list is
// default-deny. Empty lists mean "no restriction". When the client IP cannot be
// parsed and an allow list is configured, the request is denied (fail closed).
//
// NOTE: the client IP is derived from X-Forwarded-For/X-Real-IP (see clientIP),
// which any client can forge unless a trusted reverse proxy overwrites those
// headers. IP allowlisting is only as trustworthy as that ingress guarantee.
func gatewayKeyIPAllowed(key apikeycontract.APIKey, ip string) error {
	if len(key.AllowedIPs) == 0 && len(key.DeniedIPs) == 0 {
		return nil
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		if len(key.AllowedIPs) > 0 {
			return errGatewayKeyIPNotAllowed
		}
		return nil
	}
	addr = addr.Unmap()
	if ipMatchesAny(addr, key.DeniedIPs) {
		return errGatewayKeyIPNotAllowed
	}
	if len(key.AllowedIPs) > 0 && !ipMatchesAny(addr, key.AllowedIPs) {
		return errGatewayKeyIPNotAllowed
	}
	return nil
}

func ipMatchesAny(addr netip.Addr, entries []string) bool {
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if prefix, err := netip.ParsePrefix(entry); err == nil && prefix.Contains(addr) {
				return true
			}
			continue
		}
		if parsed, err := netip.ParseAddr(entry); err == nil && parsed.Unmap() == addr {
			return true
		}
	}
	return false
}

// gatewayAPIKeyWindowChecks builds the per-key multi-window request-rate checks
// (5h / 1d / 7d) for an API key, complementing the per-minute RPM check. Only
// configured (positive) windows produce a check. These are fixed-window
// counters keyed per API key.
func gatewayAPIKeyWindowChecks(key apikeycontract.APIKey) []ratelimit.Check {
	windows := []struct {
		name   string
		limit  *int
		window time.Duration
	}{
		{name: "request_limit_5h", limit: key.RequestLimit5h, window: 5 * time.Hour},
		{name: "request_limit_1d", limit: key.RequestLimit1d, window: 24 * time.Hour},
		{name: "request_limit_7d", limit: key.RequestLimit7d, window: 7 * 24 * time.Hour},
	}
	checks := make([]ratelimit.Check, 0, len(windows))
	for _, w := range windows {
		if limit := positiveLimit(w.limit); limit > 0 {
			checks = append(checks, ratelimit.Check{
				Name:   w.name,
				Key:    apiKeyWindowRateLimitKey(key.ID, w.name),
				Limit:  limit,
				Cost:   1,
				Window: w.window,
			})
		}
	}
	return checks
}

func apiKeyWindowRateLimitKey(keyID int, window string) string {
	return "apikey:" + strconv.Itoa(keyID) + ":" + window
}
