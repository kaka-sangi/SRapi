package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

// playgroundKeyName marks the per-user API key the 交界地 playground bills against.
const playgroundKeyName = "交界地 Playground"

// handleMePlaygroundModels lists the active models a user can pick in the
// playground. The gateway still enforces per-user entitlement at send time.
func (s *Server) handleMePlaygroundModels(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireConsoleSession(r); err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	models, err := s.runtime.models.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list models", requestID)
		return
	}
	out := make([]apiopenapi.PlaygroundModel, 0, len(models))
	for _, m := range models {
		if m.Status != modelcontract.StatusActive {
			continue
		}
		name := m.DisplayName
		if strings.TrimSpace(name) == "" {
			name = m.CanonicalName
		}
		out = append(out, apiopenapi.PlaygroundModel{Id: m.CanonicalName, Name: name})
	}
	writeJSONAny(w, http.StatusOK, apiopenapi.PlaygroundModelsResponse{Data: out, RequestId: requestID})
}

// handleCurrentUserAvailableModels is the console source of truth for model
// pickers that need channel availability and current pricing. It is read-only:
// gateway admission still enforces balance, quota, and per-request scheduling.
func (s *Server) handleCurrentUserAvailableModels(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	models, err := s.runtime.models.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list models", requestID)
		return
	}
	providers, err := s.runtime.providers.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list providers", requestID)
		return
	}
	accounts, err := s.runtime.accounts.List(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list accounts", requestID)
		return
	}
	pricingRules, err := s.runtime.billing.ListPricingRules(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list pricing", requestID)
		return
	}
	visibility, err := s.currentUserModelVisibility(r.Context(), session.User.ID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to resolve model visibility", requestID)
		return
	}

	catalog := availableModelCatalog{
		providers:    providersByID(providers),
		accounts:     providerAccountCounts(accounts),
		pricingRules: pricingRules,
		generatedAt:  time.Now().UTC(),
	}
	out := make([]apiopenapi.AvailableModel, 0, len(models))
	for _, model := range models {
		if model.Status != modelcontract.StatusActive || !visibility.modelVisible(model.CanonicalName) {
			continue
		}
		item, ok, err := s.availableModelItem(r.Context(), model, catalog)
		if err != nil {
			writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list model mappings", requestID)
			return
		}
		if ok {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	writeJSONAny(w, http.StatusOK, apiopenapi.AvailableModelListResponse{
		Data:        out,
		GeneratedAt: catalog.generatedAt,
		RequestId:   requestID,
	})
}

// handleMePlaygroundChat streams a billed chat completion for the signed-in user.
// It builds a normal OpenAI gateway request (no tools) and runs the shared
// serveChatCompletion core, so balance, subscription, quota, RPM, entitlement,
// and metering all apply exactly like an API request — but session-authenticated.
func (s *Server) handleMePlaygroundChat(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	startedAt := time.Now()
	session, err := s.requireConsoleSession(r)
	if err != nil {
		writeStandardError(w, http.StatusUnauthorized, apiopenapi.UNAUTHORIZED, "unauthorized", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	var req apiopenapi.PlaygroundChatRequest
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid playground request", requestID)
		return
	}
	if strings.TrimSpace(req.Model) == "" || len(req.Messages) == 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "model and messages are required", requestID)
		return
	}
	authed, err := s.ensurePlaygroundAuth(r.Context(), session.User.ID)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to prepare playground", requestID)
		return
	}
	rawBody, body, err := buildPlaygroundChatBody(req)
	if err != nil {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid playground request", requestID)
		return
	}
	s.serveChatCompletion(w, r, authed, body, rawBody, "/api/v1/me/playground", "", startedAt)
}

// ensurePlaygroundAuth resolves (find-or-create) the user's playground API key
// and returns an auth result the gateway core bills against. The key has no
// AllowedModels restriction so the user's subscription/group entitlements (which
// admission enforces) govern access; its plaintext is never exposed.
func (s *Server) ensurePlaygroundAuth(ctx context.Context, userID int) (apikeycontract.AuthResult, error) {
	keys, err := s.runtime.apiKeys.ListByUser(ctx, userID)
	if err != nil {
		return apikeycontract.AuthResult{}, err
	}
	for _, k := range keys {
		if k.Name == playgroundKeyName && k.Status == apikeycontract.StatusActive {
			return apikeycontract.AuthResult{Key: k, UserID: userID}, nil
		}
	}
	created, err := s.runtime.apiKeys.Create(ctx, apikeycontract.CreateRequest{
		UserID: userID,
		Name:   playgroundKeyName,
	})
	if err != nil {
		return apikeycontract.AuthResult{}, err
	}
	return apikeycontract.AuthResult{Key: created.Key, UserID: userID}, nil
}

// buildPlaygroundChatBody converts the playground request into a streaming
// OpenAI ChatCompletionRequest (raw bytes + decoded struct), mapping image
// attachments into multimodal content parts. No tools are ever included.
func buildPlaygroundChatBody(req apiopenapi.PlaygroundChatRequest) ([]byte, apiopenapi.ChatCompletionRequest, error) {
	messages := make([]map[string]any, 0, len(req.Messages)+1)
	if req.System != nil {
		if system := strings.TrimSpace(*req.System); system != "" {
			messages = append(messages, map[string]any{"role": "system", "content": system})
		}
	}
	for _, m := range req.Messages {
		text := ""
		if m.Content != nil {
			text = *m.Content
		}
		if m.Images != nil && len(*m.Images) > 0 {
			parts := make([]map[string]any, 0, len(*m.Images)+1)
			if strings.TrimSpace(text) != "" {
				parts = append(parts, map[string]any{"type": "text", "text": text})
			}
			for _, img := range *m.Images {
				parts = append(parts, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": "data:" + img.MimeType + ";base64," + img.Data},
				})
			}
			messages = append(messages, map[string]any{"role": string(m.Role), "content": parts})
			continue
		}
		messages = append(messages, map[string]any{"role": string(m.Role), "content": text})
	}
	payload := map[string]any{
		"model":    req.Model,
		"stream":   true,
		"messages": messages,
	}
	if req.ReasoningEffort != nil {
		if effort := string(*req.ReasoningEffort); effort != "" && effort != "off" {
			payload["reasoning_effort"] = effort
		}
	}
	if req.Temperature != nil && *req.Temperature >= 0 && *req.Temperature <= 2 {
		payload["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		payload["max_tokens"] = *req.MaxTokens
	}
	rawBody, err := json.Marshal(payload)
	if err != nil {
		return nil, apiopenapi.ChatCompletionRequest{}, err
	}
	var body apiopenapi.ChatCompletionRequest
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return nil, apiopenapi.ChatCompletionRequest{}, err
	}
	return rawBody, body, nil
}

type availableModelCatalog struct {
	providers    map[int]providercontract.Provider
	accounts     map[int]availableProviderAccountCounts
	pricingRules []billingcontract.PricingRule
	generatedAt  time.Time
}

type availableProviderAccountCounts struct {
	active int
	total  int
}

type currentUserModelVisibility struct {
	restricted bool
	allowed    map[string]struct{}
}

func (s *Server) currentUserModelVisibility(ctx context.Context, userID int) (currentUserModelVisibility, error) {
	decision, err := s.runtime.subscriptions.CheckEntitlement(ctx, subscriptioncontract.EntitlementCheckRequest{
		UserID:      userID,
		RequestTime: time.Now().UTC(),
	})
	if err != nil {
		return currentUserModelVisibility{}, err
	}
	values := entitlementStringSet(decision.Entitlements["allowed_models"])
	if len(values) == 0 {
		return currentUserModelVisibility{}, nil
	}
	return currentUserModelVisibility{restricted: true, allowed: values}, nil
}

func (v currentUserModelVisibility) modelVisible(canonicalName string) bool {
	if !v.restricted {
		return true
	}
	_, ok := v.allowed[strings.ToLower(strings.TrimSpace(canonicalName))]
	return ok
}

func entitlementStringSet(value any) map[string]struct{} {
	out := map[string]struct{}{}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if normalized := strings.ToLower(strings.TrimSpace(item)); normalized != "" {
				out[normalized] = struct{}{}
			}
		}
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				if normalized := strings.ToLower(strings.TrimSpace(text)); normalized != "" {
					out[normalized] = struct{}{}
				}
			}
		}
	}
	return out
}

func (s *Server) availableModelItem(ctx context.Context, model modelcontract.Model, catalog availableModelCatalog) (apiopenapi.AvailableModel, bool, error) {
	mappings, err := s.runtime.models.ListMappingsByModel(ctx, model.ID)
	if err != nil {
		return apiopenapi.AvailableModel{}, false, err
	}
	channels := make([]apiopenapi.AvailableModelChannel, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.Status != modelcontract.StatusActive {
			continue
		}
		provider, ok := catalog.providers[mapping.ProviderID]
		if !ok || provider.Status == providercontract.StatusArchived {
			continue
		}
		counts := catalog.accounts[provider.ID]
		status := availableChannelStatus(provider, counts)
		channels = append(channels, apiopenapi.AvailableModelChannel{
			ActiveAccountCount:  counts.active,
			AdapterType:         provider.AdapterType,
			Pricing:             availableChannelPricing(model, mapping, provider.ID, catalog.pricingRules, catalog.generatedAt),
			Protocol:            provider.Protocol,
			ProviderDisplayName: provider.DisplayName,
			ProviderId:          apiopenapi.Id(strconv.Itoa(provider.ID)),
			ProviderName:        provider.Name,
			Status:              status,
			TotalAccountCount:   counts.total,
			UpstreamModel:       mapping.UpstreamModelName,
		})
	}
	if len(channels) == 0 {
		return apiopenapi.AvailableModel{}, false, nil
	}
	sort.Slice(channels, func(i, j int) bool {
		if channels[i].Status == channels[j].Status {
			if channels[i].ProviderDisplayName == channels[j].ProviderDisplayName {
				return channels[i].UpstreamModel < channels[j].UpstreamModel
			}
			return channels[i].ProviderDisplayName < channels[j].ProviderDisplayName
		}
		return availableStatusRank(channels[i].Status) < availableStatusRank(channels[j].Status)
	})
	return apiopenapi.AvailableModel{
		Channels:        channels,
		ContextWindow:   cloneIntPtr(model.ContextWindow),
		Family:          cloneStringPtr(model.Family),
		Id:              model.CanonicalName,
		MaxOutputTokens: cloneIntPtr(model.MaxOutputTokens),
		Name:            availableModelDisplayName(model),
		Status:          availableModelStatus(channels),
	}, true, nil
}

func providersByID(providers []providercontract.Provider) map[int]providercontract.Provider {
	out := make(map[int]providercontract.Provider, len(providers))
	for _, provider := range providers {
		out[provider.ID] = provider
	}
	return out
}

func providerAccountCounts(accounts []accountcontract.ProviderAccount) map[int]availableProviderAccountCounts {
	out := map[int]availableProviderAccountCounts{}
	for _, account := range accounts {
		if account.Status == accountcontract.StatusArchived {
			continue
		}
		counts := out[account.ProviderID]
		counts.total++
		if account.Status == accountcontract.StatusActive {
			counts.active++
		}
		out[account.ProviderID] = counts
	}
	return out
}

func availableChannelStatus(provider providercontract.Provider, counts availableProviderAccountCounts) apiopenapi.AvailableModelStatus {
	if provider.Status != providercontract.StatusActive {
		return apiopenapi.AvailableModelStatusUnavailable
	}
	if counts.active > 0 {
		return apiopenapi.AvailableModelStatusAvailable
	}
	if counts.total > 0 {
		return apiopenapi.AvailableModelStatusLimited
	}
	return apiopenapi.AvailableModelStatusUnavailable
}

func availableModelStatus(channels []apiopenapi.AvailableModelChannel) apiopenapi.AvailableModelStatus {
	best := apiopenapi.AvailableModelStatusUnavailable
	for _, channel := range channels {
		if availableStatusRank(channel.Status) < availableStatusRank(best) {
			best = channel.Status
		}
	}
	return best
}

func availableStatusRank(status apiopenapi.AvailableModelStatus) int {
	switch status {
	case apiopenapi.AvailableModelStatusAvailable:
		return 0
	case apiopenapi.AvailableModelStatusLimited:
		return 1
	default:
		return 2
	}
}

func availableChannelPricing(model modelcontract.Model, mapping modelcontract.ModelProviderMapping, providerID int, rules []billingcontract.PricingRule, at time.Time) apiopenapi.AvailableModelPricing {
	if len(mapping.PricingOverride) > 0 {
		if pricing, ok := availablePricingFromOverride(mapping.PricingOverride); ok {
			return pricing
		}
	}
	if rule, ok := selectAvailablePricingRule(rules, model.ID, optionalStringValue(model.Family), providerID, at); ok {
		return availablePricingFromRule(rule, apiopenapi.AvailableModelPricingSourcePricingRule)
	}
	return apiopenapi.AvailableModelPricing{
		BillingMode:                     apiopenapi.Token,
		CacheReadPricePerMillionTokens:  money.ZeroAmount,
		CacheWritePricePerMillionTokens: money.ZeroAmount,
		Currency:                        money.DefaultCurrency,
		InputPricePerMillionTokens:      money.ZeroAmount,
		OutputPricePerMillionTokens:     money.ZeroAmount,
		PerRequestPrice:                 money.ZeroAmount,
		Source:                          apiopenapi.AvailableModelPricingSourceDefaultZero,
	}
}

func availablePricingFromOverride(payload map[string]any) (apiopenapi.AvailableModelPricing, bool) {
	input := availablePayloadString(payload, "input_price_per_million_tokens", "input_price_per_million")
	output := availablePayloadString(payload, "output_price_per_million_tokens", "output_price_per_million")
	cacheRead := availablePayloadString(payload, "cache_read_price_per_million_tokens", "cache_read_price_per_million")
	cacheWrite := availablePayloadString(payload, "cache_write_price_per_million_tokens", "cache_write_price_per_million")
	perRequest := availablePayloadString(payload, "per_request_price", "per_image_price")
	if input == "" && output == "" && cacheRead == "" && cacheWrite == "" && perRequest == "" {
		return apiopenapi.AvailableModelPricing{}, false
	}
	mode := apiopenapi.BillingMode(strings.TrimSpace(availablePayloadString(payload, "billing_mode")))
	if mode != apiopenapi.PerRequest && mode != apiopenapi.Image {
		mode = apiopenapi.Token
	}
	return apiopenapi.AvailableModelPricing{
		BillingMode:                     mode,
		CacheReadPricePerMillionTokens:  money.NormalizeAmount(cacheRead),
		CacheWritePricePerMillionTokens: money.NormalizeAmount(cacheWrite),
		Currency:                        money.NormalizeCurrency(availablePayloadString(payload, "currency")),
		InputPricePerMillionTokens:      money.NormalizeAmount(input),
		OutputPricePerMillionTokens:     money.NormalizeAmount(output),
		PerRequestPrice:                 money.NormalizeAmount(perRequest),
		Source:                          apiopenapi.AvailableModelPricingSourceMappingOverride,
	}, true
}

func availablePayloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := payload[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case float64:
			return strconv.FormatFloat(value, 'f', -1, 64)
		case int:
			return strconv.Itoa(value)
		case json.Number:
			return value.String()
		}
	}
	return ""
}

func selectAvailablePricingRule(rules []billingcontract.PricingRule, modelID int, modelFamily string, providerID int, at time.Time) (billingcontract.PricingRule, bool) {
	if rule, ok := selectAvailableModelPricingRule(rules, modelID, providerID, at); ok {
		return rule, true
	}
	return selectAvailableFamilyPricingRule(rules, modelFamily, providerID, at)
}

func selectAvailableModelPricingRule(rules []billingcontract.PricingRule, modelID int, providerID int, at time.Time) (billingcontract.PricingRule, bool) {
	var selected billingcontract.PricingRule
	found := false
	for _, rule := range rules {
		if rule.ModelID != modelID || (rule.ProviderID != providerID && rule.ProviderID != 0) || !availablePricingRuleActive(rule, at) {
			continue
		}
		if !found || availablePricingRuleMoreSpecific(rule, selected) {
			selected = rule
			found = true
		}
	}
	return selected, found
}

func selectAvailableFamilyPricingRule(rules []billingcontract.PricingRule, modelFamily string, providerID int, at time.Time) (billingcontract.PricingRule, bool) {
	modelFamily = strings.ToLower(strings.TrimSpace(modelFamily))
	if modelFamily == "" {
		return billingcontract.PricingRule{}, false
	}
	var selected billingcontract.PricingRule
	found := false
	for _, rule := range rules {
		ruleFamily := strings.ToLower(strings.TrimSpace(rule.ModelFamily))
		if ruleFamily == "" || (ruleFamily != modelFamily && !strings.Contains(modelFamily, ruleFamily) && !strings.Contains(ruleFamily, modelFamily)) {
			continue
		}
		if rule.ProviderID != providerID && rule.ProviderID != 0 {
			continue
		}
		if !availablePricingRuleActive(rule, at) {
			continue
		}
		if !found || availablePricingRuleMoreSpecific(rule, selected) {
			selected = rule
			found = true
		}
	}
	return selected, found
}

func availablePricingRuleActive(rule billingcontract.PricingRule, at time.Time) bool {
	if rule.EffectiveFrom != nil && at.Before(*rule.EffectiveFrom) {
		return false
	}
	if rule.EffectiveTo != nil && !at.Before(*rule.EffectiveTo) {
		return false
	}
	return true
}

func availablePricingRuleMoreSpecific(candidate billingcontract.PricingRule, current billingcontract.PricingRule) bool {
	if candidate.ProviderID != 0 && current.ProviderID == 0 {
		return true
	}
	if candidate.ProviderID == current.ProviderID && candidate.ID > current.ID {
		return true
	}
	return false
}

func availablePricingFromRule(rule billingcontract.PricingRule, source apiopenapi.AvailableModelPricingSource) apiopenapi.AvailableModelPricing {
	mode := apiopenapi.BillingMode(rule.BillingMode)
	if mode != apiopenapi.PerRequest && mode != apiopenapi.Image {
		mode = apiopenapi.Token
	}
	return apiopenapi.AvailableModelPricing{
		BillingMode:                     mode,
		CacheReadPricePerMillionTokens:  money.NormalizeAmount(rule.CacheReadPricePerMillionTokens),
		CacheWritePricePerMillionTokens: money.NormalizeAmount(rule.CacheWritePricePerMillionTokens),
		Currency:                        money.NormalizeCurrency(rule.Currency),
		InputPricePerMillionTokens:      money.NormalizeAmount(rule.InputPricePerMillionTokens),
		OutputPricePerMillionTokens:     money.NormalizeAmount(rule.OutputPricePerMillionTokens),
		PerRequestPrice:                 money.NormalizeAmount(rule.PerRequestPrice),
		Source:                          source,
	}
}

func availableModelDisplayName(model modelcontract.Model) string {
	if name := strings.TrimSpace(model.DisplayName); name != "" {
		return name
	}
	return model.CanonicalName
}
