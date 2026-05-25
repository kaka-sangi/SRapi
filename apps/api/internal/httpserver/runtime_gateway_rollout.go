package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func (rt *runtimeState) applyGatewayStrategyRollout(ctx context.Context, req *schedulercontract.ScheduleRequest, apiKey apikeycontract.APIKey) {
	if req == nil || rt.adminControl == nil {
		return
	}
	settings, err := rt.adminControl.GetAdminSettings(ctx)
	if err != nil {
		rt.logger.Warn("failed to load gateway scheduler rollout settings", "error", err)
		return
	}
	gateway := settings.Gateway
	if !gateway.SchedulerStrategyRolloutEnabled || gateway.SchedulerStrategyRolloutPercent <= 0 {
		return
	}
	shadowStrategy := schedulerStrategyName(gateway.SchedulerStrategyShadowStrategy)
	if shadowStrategy == "" {
		rt.logger.Warn("ignoring invalid scheduler rollout shadow strategy", "strategy", gateway.SchedulerStrategyShadowStrategy)
		return
	}
	if !gatewayRolloutModelMatches(req, gateway.SchedulerStrategyRolloutModels) {
		return
	}
	apiKeyPrefixHash := apiKeyPrefixHash(apiKey)
	if !gatewayRolloutAPIKeyMatches(apiKeyPrefixHash, gateway.SchedulerStrategyRolloutAPIKeyHashes) {
		return
	}
	rolloutKey := fmt.Sprintf("api_key_prefix:%s:model:%s", apiKeyPrefixHash, strings.TrimSpace(req.Model))
	req.StrategyRollout = schedulercontract.StrategyRollout{
		Enabled:        true,
		ShadowStrategy: shadowStrategy,
		Percent:        gateway.SchedulerStrategyRolloutPercent,
		Key:            rolloutKey,
	}
}

func gatewayRolloutModelMatches(req *schedulercontract.ScheduleRequest, models []string) bool {
	if len(models) == 0 {
		return true
	}
	candidates := []string{req.Model, req.ModelAlias}
	candidates = append(candidates, req.FallbackModels...)
	return intersectsStringFold(candidates, models)
}

func gatewayRolloutAPIKeyMatches(hash string, hashes []string) bool {
	if len(hashes) == 0 {
		return true
	}
	if strings.TrimSpace(hash) == "" {
		return false
	}
	for _, expected := range hashes {
		if strings.EqualFold(strings.TrimSpace(expected), hash) {
			return true
		}
	}
	return false
}

func apiKeyPrefixHash(apiKey apikeycontract.APIKey) string {
	prefix := strings.TrimSpace(apiKey.Prefix)
	if prefix == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(prefix))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func intersectsStringFold(left []string, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	seen := map[string]struct{}{}
	for _, value := range left {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed != "" {
			seen[trimmed] = struct{}{}
		}
	}
	for _, value := range right {
		if _, ok := seen[strings.ToLower(strings.TrimSpace(value))]; ok {
			return true
		}
	}
	return false
}
