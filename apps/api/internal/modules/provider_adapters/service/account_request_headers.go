package service

import (
	"fmt"
	"net/http"
	"strings"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

type accountEgressHeaderResolver interface {
	AccountEgressRequestHeaders(account reverseproxycontract.AccountRuntime) (http.Header, error)
}

func (s *Service) applyAccountRequestHeaders(headers http.Header, account accountcontract.ProviderAccount, credential map[string]any) {
	if headers == nil {
		return
	}
	mergeMissingAccountHeaders(headers, s.accountEgressRequestHeaders(account, credential))
	mergeAccountHeaders(headers, accountMetadataHeaders(account.Metadata))
}

func (s *Service) applyAccountRequestHeadersIfMissing(headers http.Header, account accountcontract.ProviderAccount, credential map[string]any) {
	if headers == nil {
		return
	}
	mergeMissingAccountHeaders(headers, s.accountEgressRequestHeaders(account, credential))
	mergeMissingAccountHeaders(headers, accountMetadataHeaders(account.Metadata))
}

func (s *Service) accountEgressRequestHeaders(account accountcontract.ProviderAccount, credential map[string]any) http.Header {
	if s != nil && s.reverseProxy != nil {
		if resolver, ok := s.reverseProxy.(accountEgressHeaderResolver); ok {
			headers, err := resolver.AccountEgressRequestHeaders(egressAccountRuntime(account, credential))
			if err == nil && len(headers) > 0 {
				return headers
			}
		}
	}
	return accountEgressStaticHeaders(account.Metadata)
}

func accountMetadataHeaders(metadata map[string]any) http.Header {
	return parseStringAccountHeaderValue(firstMapValue([]map[string]any{metadata}, "headers"))
}

func accountEgressStaticHeaders(metadata map[string]any) http.Header {
	if len(metadata) == 0 {
		return nil
	}
	headers := http.Header{}
	for _, value := range []any{
		firstNestedOrMetadataValue(metadata, "header_set_template", "egress_header_set_template"),
		firstNestedOrMetadataValue(metadata, "extra_static_headers", "egress_extra_static_headers"),
	} {
		mergeAccountHeaders(headers, parseAccountHeaderValue(value, false))
	}
	if ua := firstNestedOrMetadataString(metadata, "user_agent", "egress_user_agent"); ua != "" && !strings.HasPrefix(strings.ToLower(ua), "srapi/") {
		headers.Set("User-Agent", ua)
	}
	return headers
}

func firstNestedOrMetadataValue(metadata map[string]any, keys ...string) any {
	nested, _ := metadata["egress_profile"].(map[string]any)
	for _, values := range []map[string]any{nested, metadata} {
		for _, key := range keys {
			if value, ok := values[key]; ok && value != nil {
				return value
			}
		}
	}
	return nil
}

func firstNestedOrMetadataString(metadata map[string]any, keys ...string) string {
	value := firstNestedOrMetadataValue(metadata, keys...)
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func parseStringAccountHeaderValue(value any) http.Header {
	headers := http.Header{}
	switch typed := value.(type) {
	case map[string]string:
		for key, value := range typed {
			addAccountHeader(headers, key, value, true)
		}
	case map[string]any:
		for key, value := range typed {
			text, ok := value.(string)
			if !ok {
				continue
			}
			addAccountHeader(headers, key, text, true)
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func parseAccountHeaderValue(value any, allowUserAgent bool) http.Header {
	headers := http.Header{}
	switch typed := value.(type) {
	case map[string]string:
		for key, value := range typed {
			addAccountHeader(headers, key, value, allowUserAgent)
		}
	case map[string]any:
		for key, value := range typed {
			switch values := value.(type) {
			case []any:
				for _, item := range values {
					addAccountHeader(headers, key, item, allowUserAgent)
				}
			case []string:
				for _, item := range values {
					addAccountHeader(headers, key, item, allowUserAgent)
				}
			default:
				addAccountHeader(headers, key, value, allowUserAgent)
			}
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func addAccountHeader(headers http.Header, key string, value any, allowUserAgent bool) {
	key = strings.TrimSpace(key)
	valueText := cleanAccountHeaderValue(value)
	if key == "" || valueText == "" || accountHeaderForbidden(key, []string{valueText}, allowUserAgent) {
		return
	}
	headers.Add(key, valueText)
}

func cleanAccountHeaderValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(typed))
	default:
		return strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(fmt.Sprint(value)))
	}
}

func mergeMissingAccountHeaders(target, source http.Header) {
	for key, values := range source {
		if target.Get(key) != "" {
			continue
		}
		for _, value := range values {
			target.Add(key, value)
		}
	}
}

func mergeAccountHeaders(target, source http.Header) {
	for key, values := range source {
		target.Del(key)
		for _, value := range values {
			target.Add(key, value)
		}
	}
}

func accountHeaderForbidden(key string, values []string, allowUserAgent bool) bool {
	lower := strings.ToLower(http.CanonicalHeaderKey(strings.TrimSpace(key)))
	if lower == "" {
		return true
	}
	switch lower {
	case "authorization", "cookie", "set-cookie", "host", "content-length", "content-type",
		"connection", "keep-alive", "upgrade", "te", "trailer", "transfer-encoding",
		"proxy-authorization", "proxy-authenticate", "proxy-connection",
		"x-api-key", "x-goog-api-key":
		return true
	case "accept-encoding":
		for _, value := range values {
			if !strings.EqualFold(strings.TrimSpace(value), "identity") {
				return true
			}
		}
		return false
	case "x-request-id", "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto",
		"x-forwarded-port", "x-real-ip", "forwarded", "via", "server":
		return true
	case "http-referer", "referer", "priority", "x-title":
		return true
	}
	if strings.HasPrefix(lower, "sec-websocket-") || strings.HasPrefix(lower, "sec-fetch-") {
		return true
	}
	if strings.HasPrefix(lower, "x-srapi-") || strings.HasPrefix(lower, "x-gateway-") {
		return true
	}
	if lower == "user-agent" {
		if !allowUserAgent {
			return true
		}
		for _, value := range values {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "srapi/") {
				return true
			}
		}
	}
	return false
}
