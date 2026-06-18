// Activation glue for the four ChatGPT-web components ported from chatgpt2api:
//
//  1. cloudflare_clearance        (pkg/httputil)
//  2. chatgpt_web_files           (multimodal upload)
//  3. chatgpt_web_image_slots     (per-account concurrency)
//  4. chatgpt_web_ws_fallback     (always-SSE policy + metrics)
//
// PR-1 & PR-2 shipped components as dead code; this wiring file is the
// explicit contrast. Every helper here has a call site in chatgpt_web.go
// (see the call-graph comments in the function docs).
package service

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/httputil"
)

var (
	defaultClearanceCacheOnce  sync.Once
	defaultClearanceCacheInst  *httputil.ClearanceCache
	defaultClearanceProvOnce   sync.Once
	defaultClearanceProvider   httputil.ClearanceProvider
	defaultClearanceProviderMu sync.RWMutex
)

// chatGPTWebClearanceCache returns the package-global clearance cache. Lazy
// init so tests can rebind the provider before first use.
func chatGPTWebClearanceCache() *httputil.ClearanceCache {
	defaultClearanceCacheOnce.Do(func() {
		defaultClearanceCacheInst = httputil.NewClearanceCache(httputil.ClearanceCacheConfig{})
	})
	return defaultClearanceCacheInst
}

// chatGPTWebClearanceProvider returns the package-global provider. Defaults
// to the env-configured FlareSolverr provider; tests can swap via
// SetChatGPTWebClearanceProvider.
func chatGPTWebClearanceProvider() httputil.ClearanceProvider {
	defaultClearanceProviderMu.RLock()
	p := defaultClearanceProvider
	defaultClearanceProviderMu.RUnlock()
	if p != nil {
		return p
	}
	defaultClearanceProvOnce.Do(func() {
		defaultClearanceProviderMu.Lock()
		if defaultClearanceProvider == nil {
			defaultClearanceProvider = httputil.NewFlareSolverrProviderFromEnv()
		}
		defaultClearanceProviderMu.Unlock()
	})
	defaultClearanceProviderMu.RLock()
	defer defaultClearanceProviderMu.RUnlock()
	return defaultClearanceProvider
}

// SetChatGPTWebClearanceProvider lets tests inject a custom provider.
func SetChatGPTWebClearanceProvider(p httputil.ClearanceProvider) {
	defaultClearanceProviderMu.Lock()
	defaultClearanceProvider = p
	defaultClearanceProviderMu.Unlock()
}

// applyClearanceHeaders injects any cached cookies + UA into the outbound
// headers when a bundle is valid for (host, proxy). The reverse-proxy
// runtime strips raw Cookie / User-Agent caller-headers as part of its
// per-account sanitiser; the clearance values therefore also need to ride
// inside account.Credential so injectAuth (over there) can re-attach them.
// Returns true when a bundle was applied.
func applyClearanceHeaders(headers http.Header, account *reverseproxycontract.AccountRuntime, targetURL, proxyURL string) bool {
	host := httputil.HostFromURL(targetURL)
	bundle, ok := chatGPTWebClearanceCache().Get(host, proxyURL)
	if !ok || bundle == nil {
		return false
	}
	if headers != nil {
		if bundle.UserAgent != "" && headers.Get("User-Agent") == "" {
			headers.Set("User-Agent", bundle.UserAgent)
		}
		if len(bundle.Cookies) > 0 {
			merged := httputil.MergeCookieHeader(headers.Get("Cookie"), bundle.Cookies)
			if merged != "" {
				headers.Set("Cookie", merged)
			}
		}
	}
	if account != nil {
		if account.Credential == nil {
			account.Credential = map[string]any{}
		}
		if len(bundle.Cookies) > 0 {
			account.Credential["cf_clearance_cookie"] = bundle.CookieHeader()
		}
		if bundle.UserAgent != "" {
			account.Credential["cf_clearance_user_agent"] = bundle.UserAgent
		}
	}
	return true
}

// resolveAndCacheClearance hits the provider, stores the result, and
// returns true on success. Errors are converted to (false, nil) so the
// caller can decide whether the original CF challenge was fatal.
func resolveAndCacheClearance(ctx context.Context, targetURL, proxyURL string) (bool, error) {
	provider := chatGPTWebClearanceProvider()
	if provider == nil {
		return false, httputil.ErrClearanceProviderNotConfigured
	}
	// Use the context-aware variant when available.
	var bundle *httputil.ClearanceBundle
	var err error
	if ctxProv, ok := provider.(interface {
		ResolveCtx(context.Context, httputil.ResolveRequest) (*httputil.ClearanceBundle, error)
	}); ok {
		bundle, err = ctxProv.ResolveCtx(ctx, httputil.ResolveRequest{TargetURL: targetURL, ProxyURL: proxyURL})
	} else {
		bundle, err = provider.Resolve(httputil.ResolveRequest{TargetURL: targetURL, ProxyURL: proxyURL})
	}
	if err != nil {
		return false, err
	}
	if bundle == nil {
		return false, httputil.ErrClearanceResolveFailed
	}
	if bundle.TargetHost == "" {
		bundle.TargetHost = httputil.HostFromURL(targetURL)
	}
	if bundle.ProxyURL == "" {
		bundle.ProxyURL = proxyURL
	}
	chatGPTWebClearanceCache().Put(bundle)
	return true, nil
}

// chatGPTWebProxyURLForRequest pulls a configured per-account proxy URL out
// of metadata. The exact format (proxy: "http://...") matches chatgpt2api's
// account-level proxy field.
func chatGPTWebProxyURLForRequest(req contract.ConversationRequest) string {
	for _, values := range []map[string]any{req.Account.Metadata, req.Provider.ConfigSchema} {
		if v := mapString(values, "proxy"); v != "" {
			return v
		}
		if v := mapString(values, "proxy_url"); v != "" {
			return v
		}
	}
	return ""
}

// chatGPTWebAccountConcurrencyCap reads the per-account override for the
// image-slot concurrency cap, defaulting to chatgpt2api's value.
func chatGPTWebAccountConcurrencyCap(req contract.ConversationRequest) int {
	for _, key := range []string{"chatgpt_image_account_concurrency", "image_account_concurrency"} {
		if v := mapString(req.Account.Metadata, key); v != "" {
			if n := parseIntOrZero(v); n > 0 {
				return n
			}
		}
	}
	return DefaultChatGPTWebImageAccountConcurrency
}

func parseIntOrZero(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// chatGPTWebRequestIsImageGeneration reports whether a conversation request
// should consume an image slot. chatgpt2api triggers the slot on image-gen
// flows: presence of "picture_v2" system hint or the system_hint metadata
// key "chatgpt_image_generation" being truthy.
func chatGPTWebRequestIsImageGeneration(req contract.ConversationRequest) bool {
	if mapBool(req.Account.Metadata, "chatgpt_image_generation") {
		return true
	}
	if mapBool(req.RequestSettings, "chatgpt_image_generation") {
		return true
	}
	hint := strings.ToLower(mapString(req.RequestSettings, "chatgpt_system_hint"))
	return hint == "picture_v2"
}
