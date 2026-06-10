package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

func (s *Service) CreatePricingRule(ctx context.Context, req contract.CreatePricingRuleRequest) (contract.PricingRule, error) {
	if s == nil || s.pricing == nil {
		return contract.PricingRule{}, ErrInvalidInput
	}
	rule, err := PricingRuleFromRequest(req)
	if err != nil {
		return contract.PricingRule{}, err
	}
	return s.pricing.CreatePricingRule(ctx, rule)
}

// ValidatePricingRule validates a pricing-rule request without persisting it.
func (s *Service) ValidatePricingRule(req contract.CreatePricingRuleRequest) error {
	if s == nil || s.pricing == nil {
		return ErrInvalidInput
	}
	_, err := PricingRuleFromRequest(req)
	return err
}

func (s *Service) UpdatePricingRule(ctx context.Context, id int, req contract.UpdatePricingRuleRequest) (contract.PricingRule, error) {
	if s == nil || s.pricing == nil {
		return contract.PricingRule{}, ErrInvalidInput
	}
	if id <= 0 {
		return contract.PricingRule{}, ErrInvalidInput
	}
	if req.BillingMode != nil {
		mode, ok := normalizeBillingMode(*req.BillingMode)
		if !ok {
			return contract.PricingRule{}, ErrInvalidInput
		}
		req.BillingMode = &mode
	}
	if req.InputPricePerMillionTokens != nil {
		v, ok := normalizeMoney(*req.InputPricePerMillionTokens)
		if !ok {
			return contract.PricingRule{}, ErrInvalidInput
		}
		req.InputPricePerMillionTokens = &v
	}
	if req.OutputPricePerMillionTokens != nil {
		v, ok := normalizeMoney(*req.OutputPricePerMillionTokens)
		if !ok {
			return contract.PricingRule{}, ErrInvalidInput
		}
		req.OutputPricePerMillionTokens = &v
	}
	if req.CacheReadPricePerMillionTokens != nil {
		v, ok := normalizeMoney(*req.CacheReadPricePerMillionTokens)
		if !ok {
			return contract.PricingRule{}, ErrInvalidInput
		}
		req.CacheReadPricePerMillionTokens = &v
	}
	if req.CacheWritePricePerMillionTokens != nil {
		v, ok := normalizeMoney(*req.CacheWritePricePerMillionTokens)
		if !ok {
			return contract.PricingRule{}, ErrInvalidInput
		}
		req.CacheWritePricePerMillionTokens = &v
	}
	if req.CacheWrite5mPricePerMillionTokens != nil {
		v, ok := normalizeMoney(*req.CacheWrite5mPricePerMillionTokens)
		if !ok {
			return contract.PricingRule{}, ErrInvalidInput
		}
		req.CacheWrite5mPricePerMillionTokens = &v
	}
	if req.CacheWrite1hPricePerMillionTokens != nil {
		v, ok := normalizeMoney(*req.CacheWrite1hPricePerMillionTokens)
		if !ok {
			return contract.PricingRule{}, ErrInvalidInput
		}
		req.CacheWrite1hPricePerMillionTokens = &v
	}
	if req.ImageOutputPricePerMillionTokens != nil {
		v, ok := normalizeMoney(*req.ImageOutputPricePerMillionTokens)
		if !ok {
			return contract.PricingRule{}, ErrInvalidInput
		}
		req.ImageOutputPricePerMillionTokens = &v
	}
	if req.PerRequestPrice != nil {
		v, ok := normalizeMoney(*req.PerRequestPrice)
		if !ok {
			return contract.PricingRule{}, ErrInvalidInput
		}
		req.PerRequestPrice = &v
	}
	if req.LongContextMultiplier != nil {
		v, ok := normalizeMoney(*req.LongContextMultiplier)
		if !ok {
			return contract.PricingRule{}, ErrInvalidInput
		}
		req.LongContextMultiplier = &v
	}
	if req.Intervals != nil {
		intervals, err := normalizePricingIntervals(*req.Intervals)
		if err != nil {
			return contract.PricingRule{}, err
		}
		req.Intervals = &intervals
	}
	if req.Currency != nil {
		v := money.NormalizeCurrency(*req.Currency)
		req.Currency = &v
	}
	return s.pricing.UpdatePricingRule(ctx, id, req)
}

func (s *Service) FindPricingRuleByID(ctx context.Context, id int) (contract.PricingRule, error) {
	if s == nil || s.pricing == nil {
		return contract.PricingRule{}, ErrInvalidInput
	}
	return s.pricing.FindPricingRuleByID(ctx, id)
}

func (s *Service) DeletePricingRule(ctx context.Context, id int) error {
	if s == nil || s.pricing == nil {
		return ErrInvalidInput
	}
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.pricing.DeletePricingRule(ctx, id)
}

func (s *Service) ListPricingRules(ctx context.Context) ([]contract.PricingRule, error) {
	if s == nil || s.pricing == nil {
		return nil, ErrInvalidInput
	}
	return s.pricing.ListPricingRules(ctx)
}

func (s *Service) EstimatePrice(ctx context.Context, req contract.PricingRequest) (contract.PricingResult, error) {
	if s == nil || s.pricing == nil {
		return contract.PricingResult{}, ErrInvalidInput
	}
	if req.ModelID <= 0 || req.ProviderID < 0 {
		return contract.PricingResult{}, ErrInvalidInput
	}
	at := req.At
	if at.IsZero() {
		at = s.clock.Now()
	}
	if len(req.PricingOverride) > 0 {
		req = applyPricingOverrideRequestOptions(req)
		result, ok := priceFromPayload(req.PricingOverride, req, nil)
		if ok {
			return result, nil
		}
	}
	rules, err := s.pricing.QueryPricingRules(ctx, contract.PricingRuleQuery{
		ModelID:            req.ModelID,
		ModelFamily:        req.ModelFamily,
		RequestedModel:     req.RequestedModel,
		UpstreamModel:      req.UpstreamModel,
		BillingModelSource: billingModelSource(req),
		ProviderID:         req.ProviderID,
		At:                 at,
	})
	if err != nil {
		return contract.PricingResult{}, err
	}
	rule, ok := selectPricingRuleForRequest(rules, req, at)
	if !ok {
		rule, ok = selectFamilyPricingRule(rules, req.ModelFamily, req.ProviderID, at)
		if !ok {
			return contract.PricingResult{Amount: money.ZeroAmount, Currency: money.DefaultCurrency}, nil
		}
	}
	ruleID := rule.ID
	return priceFromRule(rule, req, &ruleID), nil
}

func (s *Service) PriceGatewayUsage(ctx context.Context, req contract.GatewayPricingRequest) (contract.GatewayPricingResult, error) {
	pricing, err := s.EstimatePrice(ctx, req.PricingRequest)
	if err != nil {
		return contract.GatewayPricingResult{}, err
	}
	source := "default_zero"
	if len(req.PricingOverride) > 0 {
		source = "mapping_override"
	} else if pricing.PricingRuleID != nil {
		source = "pricing_rule"
	}
	return priceGatewayCost(contract.GatewayCostRequest{
		Amount:               pricing.Amount,
		Currency:             money.NormalizeCurrency(pricing.Currency),
		PricingRuleID:        cloneIntPtr(pricing.PricingRuleID),
		BillingMode:          pricing.BillingMode,
		InputCost:            pricing.InputCost,
		OutputCost:           pricing.OutputCost,
		CacheReadCost:        pricing.CacheReadCost,
		CacheWriteCost:       pricing.CacheWriteCost,
		Source:               source,
		Estimated:            req.Estimated,
		RateMultiplier:       req.RateMultiplier,
		Success:              req.Success,
		AllowanceMode:        req.AllowanceMode,
		DailyAllowanceQuota:  req.DailyAllowanceQuota,
		WeeklyAllowanceQuota: req.WeeklyAllowanceQuota,
		AllowanceQuota:       req.AllowanceQuota,
		DailyUsedCost:        req.DailyUsedCost,
		WeeklyUsedCost:       req.WeeklyUsedCost,
		UsedCost:             req.UsedCost,
	}), nil
}

func (s *Service) PriceGatewayCost(req contract.GatewayCostRequest) contract.GatewayPricingResult {
	return priceGatewayCost(req)
}

func priceGatewayCost(req contract.GatewayCostRequest) contract.GatewayPricingResult {
	actualCost := applyRateMultiplier(req.Amount, req.RateMultiplier)
	billableCost := actualCost
	if req.Success && strings.EqualFold(strings.TrimSpace(req.AllowanceMode), "allowance") {
		billableCost = BillableOverageForWindows(actualCost, []AllowanceWindow{
			{UsedCost: req.DailyUsedCost, Quota: req.DailyAllowanceQuota},
			{UsedCost: req.WeeklyUsedCost, Quota: req.WeeklyAllowanceQuota},
			{UsedCost: req.UsedCost, Quota: req.AllowanceQuota},
		})
	}
	return contract.GatewayPricingResult{
		Amount:         money.NormalizeAmount(req.Amount),
		Currency:       money.NormalizeCurrency(req.Currency),
		PricingRuleID:  cloneIntPtr(req.PricingRuleID),
		BillingMode:    billingModeOrToken(req.BillingMode),
		InputCost:      money.NormalizeAmount(req.InputCost),
		OutputCost:     money.NormalizeAmount(req.OutputCost),
		CacheReadCost:  money.NormalizeAmount(req.CacheReadCost),
		CacheWriteCost: money.NormalizeAmount(req.CacheWriteCost),
		Source:         strings.TrimSpace(req.Source),
		Estimated:      req.Estimated,
		ActualCost:     actualCost,
		BillableCost:   billableCost,
	}
}

func PricingRuleFromRequest(req contract.CreatePricingRuleRequest) (contract.PricingRule, error) {
	if req.ModelID <= 0 || req.ProviderID < 0 {
		return contract.PricingRule{}, ErrInvalidInput
	}
	mode, ok := normalizeBillingMode(req.BillingMode)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	input, ok := normalizeMoney(req.InputPricePerMillionTokens)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	output, ok := normalizeMoney(req.OutputPricePerMillionTokens)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	cacheRead, ok := normalizeMoney(req.CacheReadPricePerMillionTokens)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	cacheWrite, ok := normalizeMoney(req.CacheWritePricePerMillionTokens)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	cacheWrite5m, ok := normalizeMoney(req.CacheWrite5mPricePerMillionTokens)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	cacheWrite1h, ok := normalizeMoney(req.CacheWrite1hPricePerMillionTokens)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	imageOutput, ok := normalizeMoney(req.ImageOutputPricePerMillionTokens)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	longContextMultiplier, ok := normalizeMoney(req.LongContextMultiplier)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	perRequestPrice, ok := normalizeMoney(req.PerRequestPrice)
	if !ok {
		return contract.PricingRule{}, ErrInvalidInput
	}
	intervals, err := normalizePricingIntervals(req.Intervals)
	if err != nil {
		return contract.PricingRule{}, err
	}
	if req.EffectiveFrom != nil && req.EffectiveTo != nil && !req.EffectiveTo.After(*req.EffectiveFrom) {
		return contract.PricingRule{}, ErrInvalidInput
	}
	if req.LongContextThresholdTokens != nil && *req.LongContextThresholdTokens <= 0 {
		return contract.PricingRule{}, ErrInvalidInput
	}
	return contract.PricingRule{
		ModelID:                           req.ModelID,
		ProviderID:                        req.ProviderID,
		BillingMode:                       mode,
		InputPricePerMillionTokens:        input,
		OutputPricePerMillionTokens:       output,
		CacheReadPricePerMillionTokens:    cacheRead,
		CacheWritePricePerMillionTokens:   cacheWrite,
		CacheWrite5mPricePerMillionTokens: cacheWrite5m,
		CacheWrite1hPricePerMillionTokens: cacheWrite1h,
		ImageOutputPricePerMillionTokens:  imageOutput,
		PerRequestPrice:                   perRequestPrice,
		ServiceTierMultipliers:            cloneStringMap(req.ServiceTierMultipliers),
		LongContextThresholdTokens:        cloneIntPtr(req.LongContextThresholdTokens),
		LongContextMultiplier:             longContextMultiplier,
		Intervals:                         intervals,
		Currency:                          money.NormalizeCurrency(req.Currency),
		EffectiveFrom:                     cloneTime(req.EffectiveFrom),
		EffectiveTo:                       cloneTime(req.EffectiveTo),
	}, nil
}

func selectPricingRule(rules []contract.PricingRule, modelID int, providerID int, at time.Time) (contract.PricingRule, bool) {
	var selected contract.PricingRule
	found := false
	for _, rule := range rules {
		if rule.ModelID != modelID {
			continue
		}
		if rule.ProviderID != providerID && rule.ProviderID != 0 {
			continue
		}
		if !pricingRuleActive(rule, at) {
			continue
		}
		if !found || moreSpecificPricingRule(rule, selected) {
			selected = rule
			found = true
		}
	}
	return selected, found
}

func selectPricingRuleForRequest(rules []contract.PricingRule, req contract.PricingRequest, at time.Time) (contract.PricingRule, bool) {
	source := billingModelSource(req)
	switch source {
	case "requested":
		if modelID := modelIDForName(rules, req.RequestedModel, req.ProviderID, at); modelID > 0 {
			return selectPricingRule(rules, modelID, req.ProviderID, at)
		}
	case "upstream":
		if modelID := modelIDForName(rules, req.UpstreamModel, req.ProviderID, at); modelID > 0 {
			return selectPricingRule(rules, modelID, req.ProviderID, at)
		}
	}
	return selectPricingRule(rules, req.ModelID, req.ProviderID, at)
}

func billingModelSource(req contract.PricingRequest) string {
	source := strings.ToLower(strings.TrimSpace(req.BillingModelSource))
	if source == "" && req.PricingOverride != nil {
		source = strings.ToLower(strings.TrimSpace(payloadString(req.PricingOverride, "billing_model_source")))
	}
	switch source {
	case "requested", "upstream", "channel_mapped":
		return source
	default:
		return "channel_mapped"
	}
}

func modelIDForName(rules []contract.PricingRule, modelName string, providerID int, at time.Time) int {
	modelName = normalizePricingFamily(modelName)
	if modelName == "" {
		return 0
	}
	var selected contract.PricingRule
	found := false
	for _, rule := range rules {
		if rule.ModelID <= 0 || !pricingRuleActive(rule, at) {
			continue
		}
		if rule.ProviderID != providerID && rule.ProviderID != 0 {
			continue
		}
		family := normalizePricingFamily(rule.ModelFamily)
		if family == "" || family != modelName {
			continue
		}
		if !found || moreSpecificPricingRule(rule, selected) {
			selected = rule
			found = true
		}
	}
	if !found {
		return 0
	}
	return selected.ModelID
}

func selectFamilyPricingRule(rules []contract.PricingRule, modelFamily string, providerID int, at time.Time) (contract.PricingRule, bool) {
	modelFamily = normalizePricingFamily(modelFamily)
	if modelFamily == "" {
		return contract.PricingRule{}, false
	}
	var selected contract.PricingRule
	found := false
	for _, rule := range rules {
		if !pricingFamilyMatches(modelFamily, rule.ModelFamily) {
			continue
		}
		if rule.ProviderID != providerID && rule.ProviderID != 0 {
			continue
		}
		if !pricingRuleActive(rule, at) {
			continue
		}
		if !found || moreSpecificPricingRule(rule, selected) {
			selected = rule
			found = true
		}
	}
	return selected, found
}

func pricingFamilyMatches(requestFamily string, ruleFamily string) bool {
	ruleFamily = normalizePricingFamily(ruleFamily)
	if requestFamily == "" || ruleFamily == "" {
		return false
	}
	return requestFamily == ruleFamily || strings.Contains(requestFamily, ruleFamily) || strings.Contains(ruleFamily, requestFamily)
}

func normalizePricingFamily(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func pricingRuleActive(rule contract.PricingRule, at time.Time) bool {
	if rule.EffectiveFrom != nil && at.Before(*rule.EffectiveFrom) {
		return false
	}
	if rule.EffectiveTo != nil && !at.Before(*rule.EffectiveTo) {
		return false
	}
	return true
}

func moreSpecificPricingRule(candidate contract.PricingRule, current contract.PricingRule) bool {
	if candidate.ProviderID != 0 && current.ProviderID == 0 {
		return true
	}
	if candidate.ProviderID == current.ProviderID && candidate.ID > current.ID {
		return true
	}
	return false
}

func priceFromRule(rule contract.PricingRule, req contract.PricingRequest, ruleID *int) contract.PricingResult {
	switch billingModeOrToken(rule.BillingMode) {
	case contract.BillingModePerRequest:
		return perRequestPriceFromRule(rule, ruleID)
	case contract.BillingModeImage:
		return imagePriceFromRule(rule, req, ruleID)
	default:
		return tokenPriceFromRule(rule, req, ruleID)
	}
}

func tokenPriceFromRule(rule contract.PricingRule, req contract.PricingRequest, ruleID *int) contract.PricingResult {
	pricedRule := rule
	_, hasInterval := selectTokenPricingInterval(rule.Intervals, req.InputTokens+req.CacheReadTokens)
	if interval, ok := selectTokenPricingInterval(rule.Intervals, req.InputTokens+req.CacheReadTokens); ok {
		pricedRule.InputPricePerMillionTokens = interval.InputPricePerMillionTokens
		pricedRule.OutputPricePerMillionTokens = interval.OutputPricePerMillionTokens
		pricedRule.CacheReadPricePerMillionTokens = interval.CacheReadPricePerMillionTokens
		pricedRule.CacheWritePricePerMillionTokens = interval.CacheWritePricePerMillionTokens
	}
	req = normalizeCacheWriteBuckets(req)
	inputCost := usagePrice(req.InputTokens, pricedRule.InputPricePerMillionTokens)
	textOutputTokens := req.OutputTokens - maxInt(req.ImageOutputTokens, 0)
	if textOutputTokens < 0 {
		textOutputTokens = 0
	}
	outputCost := usagePrice(textOutputTokens, pricedRule.OutputPricePerMillionTokens)
	outputCost = money.AddMoney(outputCost, usagePrice(req.ImageOutputTokens, imageOutputRateOrOutput(pricedRule)))
	cacheReadCost := usagePrice(req.CacheReadTokens, pricedRule.CacheReadPricePerMillionTokens)
	cacheWriteCost := cacheWriteCostFromRule(pricedRule, req)
	if !hasInterval && longContextApplies(rule, req) {
		multiplier := rule.LongContextMultiplier
		inputCost = applyRateMultiplier(inputCost, multiplier)
		outputCost = applyRateMultiplier(outputCost, multiplier)
		cacheReadCost = applyRateMultiplier(cacheReadCost, multiplier)
		cacheWriteCost = applyRateMultiplier(cacheWriteCost, multiplier)
	}
	multiplier := serviceTierMultiplier(pricedRule, req.ServiceTier)
	inputCost = applyRateMultiplier(inputCost, multiplier)
	outputCost = applyRateMultiplier(outputCost, multiplier)
	cacheReadCost = applyRateMultiplier(cacheReadCost, multiplier)
	cacheWriteCost = applyRateMultiplier(cacheWriteCost, multiplier)
	return pricingResult(pricedRule, ruleID, contract.BillingModeToken, inputCost, outputCost, cacheReadCost, cacheWriteCost)
}

func perRequestPriceFromRule(rule contract.PricingRule, ruleID *int) contract.PricingResult {
	return pricingResult(rule, ruleID, contract.BillingModePerRequest, rule.PerRequestPrice, money.ZeroAmount, money.ZeroAmount, money.ZeroAmount)
}

func imagePriceFromRule(rule contract.PricingRule, req contract.PricingRequest, ruleID *int) contract.PricingResult {
	count := req.ImageCount
	if count <= 0 {
		count = req.OutputTokens
	}
	if count <= 0 {
		count = 1
	}
	price := rule.PerRequestPrice
	if interval, ok := selectImagePricingInterval(rule.Intervals, req.ImageSize); ok {
		price = interval.PerImagePrice
	}
	outputCost := multiplyMoneyByInt(price, count)
	return pricingResult(rule, ruleID, contract.BillingModeImage, money.ZeroAmount, outputCost, money.ZeroAmount, money.ZeroAmount)
}

func pricingResult(rule contract.PricingRule, ruleID *int, mode contract.BillingMode, inputCost, outputCost, cacheReadCost, cacheWriteCost string) contract.PricingResult {
	inputCost = money.NormalizeAmount(inputCost)
	outputCost = money.NormalizeAmount(outputCost)
	cacheReadCost = money.NormalizeAmount(cacheReadCost)
	cacheWriteCost = money.NormalizeAmount(cacheWriteCost)
	amount := money.AddMoney(inputCost, outputCost)
	amount = money.AddMoney(amount, cacheReadCost)
	amount = money.AddMoney(amount, cacheWriteCost)
	return contract.PricingResult{
		Amount:         amount,
		Currency:       money.NormalizeCurrency(rule.Currency),
		PricingRuleID:  cloneIntPtr(ruleID),
		BillingMode:    mode,
		InputCost:      inputCost,
		OutputCost:     outputCost,
		CacheReadCost:  cacheReadCost,
		CacheWriteCost: cacheWriteCost,
	}
}

func selectTokenPricingInterval(intervals []contract.PricingInterval, tokens int) (contract.PricingInterval, bool) {
	for _, interval := range intervals {
		if tokens < interval.MinTokens {
			continue
		}
		if interval.MaxTokens != nil && tokens > *interval.MaxTokens {
			continue
		}
		return interval, true
	}
	return contract.PricingInterval{}, false
}

func selectImagePricingInterval(intervals []contract.PricingInterval, imageSize string) (contract.PricingInterval, bool) {
	imageSize = normalizeImageTier(imageSize)
	var defaultInterval contract.PricingInterval
	hasDefault := false
	for _, interval := range intervals {
		tier := normalizeImageTier(interval.ImageSize)
		if tier == "" && !hasDefault {
			defaultInterval = interval
			hasDefault = true
		}
		if tier != "" && tier == imageSize {
			return interval, true
		}
	}
	if hasDefault {
		return defaultInterval, true
	}
	return contract.PricingInterval{}, false
}

// cacheWriteRateOrInput returns the configured cache-write rate, falling back to
// the input rate when no positive cache-write rate is set. Prompt-cache writes
// cost at least as much as normal input tokens, so an unset write rate must not
// bill them at zero (a revenue leak); it bills them at the input rate instead.
func cacheWriteRateOrInput(rule contract.PricingRule) string {
	if rate, ok := money.DecimalRat(rule.CacheWritePricePerMillionTokens); ok && rate.Sign() > 0 {
		return rule.CacheWritePricePerMillionTokens
	}
	return rule.InputPricePerMillionTokens
}

func priceFromPayload(payload map[string]any, req contract.PricingRequest, ruleID *int) (contract.PricingResult, bool) {
	if hasLegacyPricingOverrideKey(payload) {
		return contract.PricingResult{}, false
	}
	mode, _ := normalizeBillingMode(contract.BillingMode(payloadString(payload, "billing_mode")))
	input := payloadMoney(payload, "input_price_per_million_tokens")
	output := payloadMoney(payload, "output_price_per_million_tokens")
	cacheRead := payloadMoney(payload, "cache_read_price_per_million_tokens")
	cacheWrite := payloadMoney(payload, "cache_write_price_per_million_tokens")
	cacheWrite5m := payloadMoney(payload, "cache_write_5m_price_per_million_tokens")
	cacheWrite1h := payloadMoney(payload, "cache_write_1h_price_per_million_tokens")
	imageOutput := payloadMoney(payload, "image_output_price_per_million_tokens")
	perRequest := payloadMoney(payload, "per_request_price")
	if input == "" && output == "" && cacheRead == "" && cacheWrite == "" && cacheWrite5m == "" && cacheWrite1h == "" && imageOutput == "" && perRequest == "" {
		return contract.PricingResult{}, false
	}
	rule := contract.PricingRule{
		BillingMode:                       mode,
		InputPricePerMillionTokens:        money.NormalizeAmount(input),
		OutputPricePerMillionTokens:       money.NormalizeAmount(output),
		CacheReadPricePerMillionTokens:    money.NormalizeAmount(cacheRead),
		CacheWritePricePerMillionTokens:   money.NormalizeAmount(cacheWrite),
		CacheWrite5mPricePerMillionTokens: money.NormalizeAmount(cacheWrite5m),
		CacheWrite1hPricePerMillionTokens: money.NormalizeAmount(cacheWrite1h),
		ImageOutputPricePerMillionTokens:  money.NormalizeAmount(imageOutput),
		PerRequestPrice:                   money.NormalizeAmount(perRequest),
		ServiceTierMultipliers:            payloadStringMap(payload, "service_tier_multipliers"),
		LongContextThresholdTokens:        payloadIntPtr(payload, "long_context_threshold_tokens", "long_context_threshold"),
		LongContextMultiplier:             money.NormalizeAmount(payloadMoney(payload, "long_context_multiplier")),
		Currency:                          payloadString(payload, "currency"),
	}
	return priceFromRule(rule, req, ruleID), true
}

func hasLegacyPricingOverrideKey(payload map[string]any) bool {
	for _, key := range []string{
		"input_price_per_million",
		"output_price_per_million",
		"cache_read_price_per_million",
		"cache_write_price_per_million",
		"cache_write_5m_price_per_million",
		"cache_write_1h_price_per_million",
		"image_output_price_per_million",
		"per_image_price",
	} {
		if _, ok := payload[key]; ok {
			return true
		}
	}
	return false
}

func applyPricingOverrideRequestOptions(req contract.PricingRequest) contract.PricingRequest {
	if req.BillingModelSource == "" {
		req.BillingModelSource = payloadString(req.PricingOverride, "billing_model_source")
	}
	if req.ServiceTier == "" {
		req.ServiceTier = payloadString(req.PricingOverride, "service_tier")
	}
	return req
}

func usagePrice(tokens int, pricePerMillion string) string {
	if tokens <= 0 {
		return money.ZeroAmount
	}
	price, ok := money.DecimalRat(pricePerMillion)
	if !ok {
		return money.ZeroAmount
	}
	price.Mul(price, big.NewRat(int64(tokens), 1000000))
	return money.FormatRatFixed(price, 8)
}

func normalizeMoney(value string) (string, bool) {
	rat, ok := money.DecimalRat(money.NormalizeAmount(value))
	if !ok || rat.Sign() < 0 {
		return "", false
	}
	return money.FormatRatFixed(rat, 8), true
}

func normalizeBillingMode(value contract.BillingMode) (contract.BillingMode, bool) {
	switch contract.BillingMode(strings.TrimSpace(string(value))) {
	case "", contract.BillingModeToken:
		return contract.BillingModeToken, true
	case contract.BillingModePerRequest:
		return contract.BillingModePerRequest, true
	case contract.BillingModeImage:
		return contract.BillingModeImage, true
	default:
		return "", false
	}
}

func billingModeOrToken(value contract.BillingMode) contract.BillingMode {
	mode, ok := normalizeBillingMode(value)
	if !ok {
		return contract.BillingModeToken
	}
	return mode
}

func normalizePricingIntervals(values []contract.PricingInterval) ([]contract.PricingInterval, error) {
	out := make([]contract.PricingInterval, 0, len(values))
	for _, value := range values {
		if value.MinTokens < 0 {
			return nil, ErrInvalidInput
		}
		if value.MaxTokens != nil && *value.MaxTokens < value.MinTokens {
			return nil, ErrInvalidInput
		}
		input, ok := normalizeMoney(value.InputPricePerMillionTokens)
		if !ok {
			return nil, ErrInvalidInput
		}
		output, ok := normalizeMoney(value.OutputPricePerMillionTokens)
		if !ok {
			return nil, ErrInvalidInput
		}
		cacheRead, ok := normalizeMoney(value.CacheReadPricePerMillionTokens)
		if !ok {
			return nil, ErrInvalidInput
		}
		cacheWrite, ok := normalizeMoney(value.CacheWritePricePerMillionTokens)
		if !ok {
			return nil, ErrInvalidInput
		}
		perImage, ok := normalizeMoney(value.PerImagePrice)
		if !ok {
			return nil, ErrInvalidInput
		}
		normalized := contract.PricingInterval{
			ID:                              value.ID,
			PricingRuleID:                   value.PricingRuleID,
			MinTokens:                       value.MinTokens,
			MaxTokens:                       cloneIntPtr(value.MaxTokens),
			TierLabel:                       strings.TrimSpace(value.TierLabel),
			ImageSize:                       normalizeImageTier(value.ImageSize),
			InputPricePerMillionTokens:      input,
			OutputPricePerMillionTokens:     output,
			CacheReadPricePerMillionTokens:  cacheRead,
			CacheWritePricePerMillionTokens: cacheWrite,
			PerImagePrice:                   perImage,
			CreatedAt:                       value.CreatedAt,
			UpdatedAt:                       value.UpdatedAt,
		}
		out = append(out, normalized)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].MinTokens == out[j].MinTokens {
			return maxTokenSortValue(out[i].MaxTokens) < maxTokenSortValue(out[j].MaxTokens)
		}
		return out[i].MinTokens < out[j].MinTokens
	})
	return out, nil
}

func maxTokenSortValue(value *int) int {
	if value == nil {
		return int(^uint(0) >> 1)
	}
	return *value
}

func normalizeImageTier(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func multiplyMoneyByInt(amount string, count int) string {
	if count <= 0 {
		return money.ZeroAmount
	}
	rat, ok := money.DecimalRat(amount)
	if !ok {
		return money.ZeroAmount
	}
	rat.Mul(rat, big.NewRat(int64(count), 1))
	return money.FormatRatFixed(rat, 8)
}

func normalizeCacheWriteBuckets(req contract.PricingRequest) contract.PricingRequest {
	if req.CacheWriteTokens <= 0 {
		return req
	}
	if req.CacheWrite5mTokens <= 0 && req.CacheWrite1hTokens <= 0 {
		req.CacheWrite5mTokens = req.CacheWriteTokens
		return req
	}
	total := req.CacheWrite5mTokens + req.CacheWrite1hTokens
	if total < req.CacheWriteTokens {
		req.CacheWrite5mTokens += req.CacheWriteTokens - total
	}
	return req
}

func cacheWriteCostFromRule(rule contract.PricingRule, req contract.PricingRequest) string {
	if req.CacheWrite5mTokens > 0 || req.CacheWrite1hTokens > 0 {
		cost := usagePrice(req.CacheWrite5mTokens, cacheWrite5mRateOrDefault(rule))
		return money.AddMoney(cost, usagePrice(req.CacheWrite1hTokens, cacheWrite1hRateOrDefault(rule)))
	}
	return usagePrice(req.CacheWriteTokens, cacheWriteRateOrInput(rule))
}

func cacheWrite5mRateOrDefault(rule contract.PricingRule) string {
	if rate, ok := money.DecimalRat(rule.CacheWrite5mPricePerMillionTokens); ok && rate.Sign() > 0 {
		return rule.CacheWrite5mPricePerMillionTokens
	}
	return cacheWriteRateOrInput(rule)
}

func cacheWrite1hRateOrDefault(rule contract.PricingRule) string {
	if rate, ok := money.DecimalRat(rule.CacheWrite1hPricePerMillionTokens); ok && rate.Sign() > 0 {
		return rule.CacheWrite1hPricePerMillionTokens
	}
	return cacheWrite5mRateOrDefault(rule)
}

func imageOutputRateOrOutput(rule contract.PricingRule) string {
	if rate, ok := money.DecimalRat(rule.ImageOutputPricePerMillionTokens); ok && rate.Sign() > 0 {
		return rule.ImageOutputPricePerMillionTokens
	}
	return rule.OutputPricePerMillionTokens
}

func longContextApplies(rule contract.PricingRule, req contract.PricingRequest) bool {
	if rule.LongContextThresholdTokens == nil || *rule.LongContextThresholdTokens <= 0 {
		return false
	}
	if req.InputTokens+req.OutputTokens+req.CacheReadTokens+req.CacheWriteTokens < *rule.LongContextThresholdTokens {
		return false
	}
	rate, ok := money.DecimalRat(rule.LongContextMultiplier)
	return ok && rate.Sign() > 0
}

func serviceTierMultiplier(rule contract.PricingRule, tier string) string {
	tier = strings.ToLower(strings.TrimSpace(tier))
	if tier == "" || tier == "auto" || tier == "default" || tier == "standard" {
		return "1.00000000"
	}
	if rule.ServiceTierMultipliers != nil {
		if value := strings.TrimSpace(rule.ServiceTierMultipliers[tier]); value != "" {
			return value
		}
	}
	switch tier {
	case "priority", "fast":
		return "2.00000000"
	case "flex":
		return "0.50000000"
	default:
		return "1.00000000"
	}
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func applyRateMultiplier(cost string, rateMultiplier string) string {
	costRat, ok := money.DecimalRat(cost)
	if !ok {
		return cost
	}
	rateRat, ok := money.DecimalRat(rateMultiplier)
	if !ok || rateRat.Sign() < 0 {
		rateRat = big.NewRat(1, 1)
	}
	return money.FormatRatFixed(costRat.Mul(costRat, rateRat), 8)
}

// BillableOverage returns the portion of cost billed to balance given the cost
// already spent this period and the included allowance ceiling. The covered
// portion (cost - billable) is absorbed by the subscription allowance. It is
// pure and clamps the result to [0, cost].
func BillableOverage(cost, usedBefore, allowance string) string {
	costRat, ok := money.DecimalRat(cost)
	if !ok || costRat.Sign() <= 0 {
		return cost
	}
	allowRat, ok := money.DecimalRat(allowance)
	if !ok {
		return cost
	}
	usedRat, ok := money.DecimalRat(usedBefore)
	if !ok {
		usedRat = new(big.Rat)
	}
	overage := new(big.Rat).Sub(new(big.Rat).Add(usedRat, costRat), allowRat)
	if overage.Sign() <= 0 {
		return money.ZeroAmount
	}
	if overage.Cmp(costRat) >= 0 {
		return money.FormatRatFixed(costRat, 8)
	}
	return money.FormatRatFixed(overage, 8)
}

// AllowanceWindow carries one cost allowance window. Nil quotas are ignored.
type AllowanceWindow struct {
	UsedCost string
	Quota    *string
}

// BillableOverageForWindows returns the highest overage across all active
// allowance windows, so daily/weekly/monthly ceilings all constrain the covered
// portion of a request.
func BillableOverageForWindows(cost string, windows []AllowanceWindow) string {
	if _, ok := money.DecimalRat(cost); !ok {
		return cost
	}
	maxOverage := money.ZeroAmount
	found := false
	for _, window := range windows {
		if window.Quota == nil {
			continue
		}
		overage := BillableOverage(cost, window.UsedCost, *window.Quota)
		if compareMoney(overage, maxOverage) > 0 {
			maxOverage = overage
		}
		found = true
	}
	if !found {
		return cost
	}
	return maxOverage
}

func compareMoney(left string, right string) int {
	leftRat, leftOK := money.DecimalRat(money.NormalizeAmount(left))
	rightRat, rightOK := money.DecimalRat(money.NormalizeAmount(right))
	if !leftOK || !rightOK {
		return strings.Compare(money.NormalizeAmount(left), money.NormalizeAmount(right))
	}
	return leftRat.Cmp(rightRat)
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	out := make(map[string]string, len(value))
	for key, item := range value {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		normalizedValue := strings.TrimSpace(item)
		if normalizedKey != "" && normalizedValue != "" {
			out[normalizedKey] = normalizedValue
		}
	}
	return out
}

func payloadMoney(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		normalized, ok := normalizeMoney(toString(value))
		if ok {
			return normalized
		}
	}
	return ""
}

func payloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	return toString(value)
}

func payloadIntPtr(payload map[string]any, keys ...string) *int {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		var parsed int
		switch typed := value.(type) {
		case int:
			parsed = typed
		case int64:
			parsed = int(typed)
		case float64:
			parsed = int(typed)
		case json.Number:
			n, err := typed.Int64()
			if err != nil {
				continue
			}
			parsed = int(n)
		default:
			if _, err := fmt.Sscan(toString(typed), &parsed); err != nil {
				continue
			}
		}
		if parsed > 0 {
			return &parsed
		}
	}
	return nil
}

func payloadStringMap(payload map[string]any, key string) map[string]string {
	value, ok := payload[key]
	if !ok || value == nil {
		return nil
	}
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		normalizedKey := strings.ToLower(strings.TrimSpace(k))
		normalizedValue := strings.TrimSpace(toString(v))
		if normalizedKey != "" && normalizedValue != "" {
			out[normalizedKey] = normalizedValue
		}
	}
	return out
}

func toString(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	default:
		return strings.TrimSpace(strings.Trim(fmt.Sprint(value), "\""))
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
