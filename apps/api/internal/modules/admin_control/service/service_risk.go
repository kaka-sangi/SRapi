package service

import (
	"context"
	"sort"
	"strings"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
)

const (
	settingsKeyRiskLogs      = "admin_control.risk_logs"
	settingsKeyRiskConfig    = "admin_control.risk_config"
	settingsKeyContentSafety = "admin_control.content_safety_config"
)

func (s *Service) GetRiskConfig(ctx context.Context) (admincontrol.RiskControlConfig, error) {
	config := defaultRiskConfig()
	if err := s.loadTyped(ctx, settingsKeyRiskConfig, &config); err != nil {
		return admincontrol.RiskControlConfig{}, err
	}
	return config, nil
}

func (s *Service) UpdateRiskConfig(ctx context.Context, config admincontrol.RiskControlConfig, actorUserID int) (admincontrol.RiskControlConfig, error) {
	normalized, err := normalizeRiskConfig(config)
	if err != nil {
		return admincontrol.RiskControlConfig{}, err
	}
	if err := s.saveTyped(ctx, settingsKeyRiskConfig, normalized, actorUserID); err != nil {
		return admincontrol.RiskControlConfig{}, err
	}
	return normalized, nil
}

func (s *Service) GetContentSafetyConfig(ctx context.Context) (admincontrol.ContentSafetyConfig, error) {
	config := defaultContentSafetyConfig()
	if err := s.loadTyped(ctx, settingsKeyContentSafety, &config); err != nil {
		return admincontrol.ContentSafetyConfig{}, err
	}
	normalized, err := normalizeContentSafetyConfig(config)
	if err != nil {
		return admincontrol.ContentSafetyConfig{}, err
	}
	return normalized, nil
}

func (s *Service) UpdateContentSafetyConfig(ctx context.Context, config admincontrol.ContentSafetyConfig, actorUserID int) (admincontrol.ContentSafetyConfig, error) {
	normalized, err := normalizeContentSafetyConfig(config)
	if err != nil {
		return admincontrol.ContentSafetyConfig{}, err
	}
	if err := s.saveTyped(ctx, settingsKeyContentSafety, normalized, actorUserID); err != nil {
		return admincontrol.ContentSafetyConfig{}, err
	}
	return normalized, nil
}

func (s *Service) RiskStatus(ctx context.Context) (admincontrol.RiskControlStatus, error) {
	config, err := s.GetRiskConfig(ctx)
	if err != nil {
		return admincontrol.RiskControlStatus{}, err
	}
	var logs riskLogCollection
	if err := s.loadTyped(ctx, settingsKeyRiskLogs, &logs); err != nil {
		return admincontrol.RiskControlStatus{}, err
	}
	recentCutoff := s.clock.Now().Add(-24 * time.Hour)
	var activeBlocks, recent int
	for _, item := range logs.Items {
		if item.Level == admincontrol.RiskControlLogLevelBlock {
			activeBlocks++
		}
		if !item.CreatedAt.Before(recentCutoff) {
			recent++
		}
	}
	return admincontrol.RiskControlStatus{
		Enabled:      config.Enabled,
		Mode:         config.Mode,
		ActiveBlocks: activeBlocks,
		RecentEvents: recent,
		EvaluatedAt:  s.clock.Now(),
	}, nil
}

func (s *Service) ListRiskLogs(ctx context.Context, opts admincontrol.ListOptions) (admincontrol.RiskLogList, error) {
	var collection riskLogCollection
	if err := s.loadTyped(ctx, settingsKeyRiskLogs, &collection); err != nil {
		return admincontrol.RiskLogList{}, err
	}
	items := make([]admincontrol.RiskControlLog, 0, len(collection.Items))
	for _, item := range collection.Items {
		if opts.Level != "" && string(item.Level) != opts.Level {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return admincontrol.RiskLogList{Items: pageItems(items, opts), Total: len(items)}, nil
}

func (s *Service) RecordRiskLog(ctx context.Context, req admincontrol.RecordRiskLogRequest) (admincontrol.RiskControlLog, error) {
	item, err := riskLogFromRecordRequest(req, s.clock.Now())
	if err != nil {
		return admincontrol.RiskControlLog{}, err
	}
	var collection riskLogCollection
	if err := s.loadTyped(ctx, settingsKeyRiskLogs, &collection); err != nil {
		return admincontrol.RiskControlLog{}, err
	}
	item.ID = nextID(collection.NextID, len(collection.Items))
	collection.Items = append(collection.Items, item)
	collection.NextID = item.ID + 1
	if len(collection.Items) > 1000 {
		collection.Items = collection.Items[len(collection.Items)-1000:]
	}
	if err := s.saveTyped(ctx, settingsKeyRiskLogs, collection, 0); err != nil {
		return admincontrol.RiskControlLog{}, err
	}
	return item, nil
}

type riskLogCollection struct {
	NextID int                           `json:"next_id"`
	Items  []admincontrol.RiskControlLog `json:"items"`
}

func defaultRiskConfig() admincontrol.RiskControlConfig {
	return admincontrol.RiskControlConfig{
		Enabled:                    false,
		Mode:                       admincontrol.RiskControlModeMonitor,
		MaxFailedRequestsPerMinute: 0,
		MaxCostPerDay:              "0",
		CooldownSeconds:            0,
		BlockedCountries:           []string{},
		BlockedIPs:                 []string{},
	}
}

func defaultContentSafetyConfig() admincontrol.ContentSafetyConfig {
	return admincontrol.ContentSafetyConfig{
		Enabled:              true,
		Mode:                 admincontrol.ContentSafetyModeMonitor,
		RedactPII:            true,
		BlockPII:             false,
		BlockPromptInjection: false,
		BlockCustomKeywords:  false,
		CustomKeywords:       []string{},
		ModelScopes:          []string{},
	}
}

func normalizeContentSafetyConfig(config admincontrol.ContentSafetyConfig) (admincontrol.ContentSafetyConfig, error) {
	if !config.Mode.Valid() {
		return admincontrol.ContentSafetyConfig{}, admincontrol.ErrInvalidInput
	}
	config.CustomKeywords = lowerUniqueTrimmedStrings(config.CustomKeywords)
	config.ModelScopes = lowerUniqueTrimmedStrings(config.ModelScopes)
	return config, nil
}

func normalizeRiskConfig(config admincontrol.RiskControlConfig) (admincontrol.RiskControlConfig, error) {
	if !config.Mode.Valid() || config.MaxFailedRequestsPerMinute < 0 || config.CooldownSeconds < 0 || !validDecimal(config.MaxCostPerDay) {
		return admincontrol.RiskControlConfig{}, admincontrol.ErrInvalidInput
	}
	if config.BlockedCountries == nil {
		config.BlockedCountries = []string{}
	}
	if config.BlockedIPs == nil {
		config.BlockedIPs = []string{}
	}
	return config, nil
}

func riskLogFromRecordRequest(req admincontrol.RecordRiskLogRequest, now time.Time) (admincontrol.RiskControlLog, error) {
	level := req.Level
	if level == "" {
		level = admincontrol.RiskControlLogLevelInfo
	}
	action := strings.TrimSpace(req.Action)
	reason := strings.TrimSpace(req.Reason)
	if !validRiskLogLevel(level) || action == "" || reason == "" {
		return admincontrol.RiskControlLog{}, admincontrol.ErrInvalidInput
	}
	createdAt := req.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = now.UTC()
	}
	var subject *string
	if req.Subject != nil {
		trimmed := strings.TrimSpace(*req.Subject)
		if trimmed != "" {
			subject = &trimmed
		}
	}
	return admincontrol.RiskControlLog{
		Level:     level,
		Action:    action,
		Reason:    reason,
		Subject:   subject,
		Metadata:  cloneAnyMap(req.Metadata),
		CreatedAt: createdAt,
	}, nil
}

func validRiskLogLevel(level admincontrol.RiskControlLogLevel) bool {
	switch level {
	case admincontrol.RiskControlLogLevelInfo, admincontrol.RiskControlLogLevelWarn, admincontrol.RiskControlLogLevelBlock:
		return true
	default:
		return false
	}
}
