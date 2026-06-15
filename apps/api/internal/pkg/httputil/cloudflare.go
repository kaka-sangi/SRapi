// Package httputil holds small, dependency-free HTTP helpers shared across the
// API. The Cloudflare helpers here detect interactive JS "challenge" responses
// (HTTP 403/429) so callers can classify them distinctly from genuine auth
// failures or rate limits and surface the cf-ray identifier for debugging.
package httputil

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var (
	cfRayPattern  = regexp.MustCompile(`(?i)cf-ray[:\s=]+([a-z0-9-]+)`)
	cRayPattern   = regexp.MustCompile(`(?i)cRay:\s*'([a-z0-9-]+)'`)
	htmlChallenge = []string{
		"window._cf_chl_opt",
		"just a moment",
		"enable javascript and cookies to continue",
		"__cf_chl_",
		"challenge-platform",
	}
)

// IsCloudflareChallengeResponse reports whether the upstream response matches
// Cloudflare challenge behavior. It only considers 403/429 responses, treating
// the "cf-mitigated: challenge" header, well-known challenge HTML markers, or a
// Cloudflare/challenge text-HTML body as a positive match.
func IsCloudflareChallengeResponse(statusCode int, headers http.Header, body []byte) bool {
	if statusCode != http.StatusForbidden && statusCode != http.StatusTooManyRequests {
		return false
	}

	if headers != nil && strings.EqualFold(strings.TrimSpace(headers.Get("cf-mitigated")), "challenge") {
		return true
	}

	preview := strings.ToLower(TruncateBody(body, 4096))
	for _, marker := range htmlChallenge {
		if strings.Contains(preview, marker) {
			return true
		}
	}

	contentType := ""
	if headers != nil {
		contentType = strings.ToLower(strings.TrimSpace(headers.Get("content-type")))
	}
	if strings.Contains(contentType, "text/html") &&
		(strings.Contains(preview, "<html") || strings.Contains(preview, "<!doctype html")) &&
		(strings.Contains(preview, "cloudflare") || strings.Contains(preview, "challenge")) {
		return true
	}

	return false
}

// ExtractCloudflareRayID extracts the cf-ray identifier from headers or, failing
// that, from the response body.
func ExtractCloudflareRayID(headers http.Header, body []byte) string {
	if headers != nil {
		rayID := strings.TrimSpace(headers.Get("cf-ray"))
		if rayID != "" {
			return rayID
		}
		rayID = strings.TrimSpace(headers.Get("Cf-Ray"))
		if rayID != "" {
			return rayID
		}
	}

	preview := TruncateBody(body, 8192)
	if matches := cfRayPattern.FindStringSubmatch(preview); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	if matches := cRayPattern.FindStringSubmatch(preview); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// FormatCloudflareChallengeMessage appends cf-ray info to base when available.
func FormatCloudflareChallengeMessage(base string, headers http.Header, body []byte) string {
	rayID := ExtractCloudflareRayID(headers, body)
	if rayID == "" {
		return base
	}
	return fmt.Sprintf("%s (cf-ray: %s)", base, rayID)
}

// TruncateBody truncates body text for logging/inspection. A non-positive max
// falls back to 512 bytes.
func TruncateBody(body []byte, max int) string {
	if max <= 0 {
		max = 512
	}
	raw := strings.TrimSpace(string(body))
	if len(raw) <= max {
		return raw
	}
	return raw[:max] + "...(truncated)"
}
