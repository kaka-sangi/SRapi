package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"sort"
	"strings"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
)

const (
	settingsKeyAnnouncements = "admin_control.announcements"
	settingsKeyRedeemCodes   = "admin_control.redeem_codes"
	settingsKeyPromoCodes    = "admin_control.promo_codes"
	settingsKeySystemLogs    = "admin_control.system_logs"
	settingsKeyRiskLogs      = "admin_control.risk_logs"
	settingsKeyRiskConfig    = "admin_control.risk_config"
	settingsKeyOpsSettings   = "admin_control.ops_settings"
	settingsKeyAdminSettings = "admin_control.admin_settings"

	defaultPageSize = 20
	maxPageSize     = 1000
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store admincontrol.Store
	clock Clock
}

func New(store admincontrol.Store, clock Clock) (*Service, error) {
	if store == nil {
		return nil, admincontrol.ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, clock: clock}, nil
}

func (s *Service) GetAdminSettings(ctx context.Context) (admincontrol.AdminSettings, error) {
	settings := defaultAdminSettings(s.clock.Now())
	if err := s.loadTyped(ctx, settingsKeyAdminSettings, &settings); err != nil {
		return admincontrol.AdminSettings{}, err
	}
	return settings, nil
}

func (s *Service) UpdateAdminSettings(ctx context.Context, settings admincontrol.AdminSettings, actorUserID int) (admincontrol.AdminSettings, error) {
	normalized, err := normalizeAdminSettings(settings)
	if err != nil {
		return admincontrol.AdminSettings{}, err
	}
	if err := s.saveTyped(ctx, settingsKeyAdminSettings, normalized, actorUserID); err != nil {
		return admincontrol.AdminSettings{}, err
	}
	return normalized, nil
}

func (s *Service) GetOpsSettings(ctx context.Context) (admincontrol.OpsSettings, error) {
	settings := defaultOpsSettings()
	if err := s.loadTyped(ctx, settingsKeyOpsSettings, &settings); err != nil {
		return admincontrol.OpsSettings{}, err
	}
	return settings, nil
}

func (s *Service) UpdateOpsSettings(ctx context.Context, settings admincontrol.OpsSettings, actorUserID int) (admincontrol.OpsSettings, error) {
	if err := validateOpsSettings(settings); err != nil {
		return admincontrol.OpsSettings{}, err
	}
	if err := s.saveTyped(ctx, settingsKeyOpsSettings, settings, actorUserID); err != nil {
		return admincontrol.OpsSettings{}, err
	}
	return settings, nil
}

func (s *Service) ListAnnouncements(ctx context.Context, opts admincontrol.ListOptions) (admincontrol.AnnouncementList, error) {
	var collection announcementCollection
	if err := s.loadTyped(ctx, settingsKeyAnnouncements, &collection); err != nil {
		return admincontrol.AnnouncementList{}, err
	}
	items := make([]admincontrol.Announcement, 0, len(collection.Items))
	for _, item := range collection.Items {
		if opts.Status != "" && string(item.Status) != opts.Status {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	paged := pageItems(items, opts)
	return admincontrol.AnnouncementList{Items: paged, Total: len(items)}, nil
}

func (s *Service) CreateAnnouncement(ctx context.Context, req admincontrol.AnnouncementRequest, actorUserID int) (admincontrol.Announcement, error) {
	var collection announcementCollection
	if err := s.loadTyped(ctx, settingsKeyAnnouncements, &collection); err != nil {
		return admincontrol.Announcement{}, err
	}
	now := s.clock.Now()
	item, err := announcementFromCreateRequest(req, nextID(collection.NextID, len(collection.Items)), now)
	if err != nil {
		return admincontrol.Announcement{}, err
	}
	collection.Items = append(collection.Items, item)
	collection.NextID = item.ID + 1
	if err := s.saveTyped(ctx, settingsKeyAnnouncements, collection, actorUserID); err != nil {
		return admincontrol.Announcement{}, err
	}
	return item, nil
}

func (s *Service) UpdateAnnouncement(ctx context.Context, id int, req admincontrol.AnnouncementRequest, actorUserID int) (admincontrol.Announcement, error) {
	var collection announcementCollection
	if err := s.loadTyped(ctx, settingsKeyAnnouncements, &collection); err != nil {
		return admincontrol.Announcement{}, err
	}
	for idx, item := range collection.Items {
		if item.ID != id {
			continue
		}
		updated, err := announcementFromCreateRequest(req, id, s.clock.Now())
		if err != nil {
			return admincontrol.Announcement{}, err
		}
		updated.CreatedAt = item.CreatedAt
		collection.Items[idx] = updated
		if err := s.saveTyped(ctx, settingsKeyAnnouncements, collection, actorUserID); err != nil {
			return admincontrol.Announcement{}, err
		}
		return updated, nil
	}
	return admincontrol.Announcement{}, admincontrol.ErrNotFound
}

func (s *Service) DeleteAnnouncement(ctx context.Context, id int, actorUserID int) (admincontrol.Announcement, error) {
	var collection announcementCollection
	if err := s.loadTyped(ctx, settingsKeyAnnouncements, &collection); err != nil {
		return admincontrol.Announcement{}, err
	}
	for idx, item := range collection.Items {
		if item.ID != id {
			continue
		}
		collection.Items = append(collection.Items[:idx], collection.Items[idx+1:]...)
		if err := s.saveTyped(ctx, settingsKeyAnnouncements, collection, actorUserID); err != nil {
			return admincontrol.Announcement{}, err
		}
		return item, nil
	}
	return admincontrol.Announcement{}, admincontrol.ErrNotFound
}

func (s *Service) ListRedeemCodes(ctx context.Context, opts admincontrol.ListOptions) (admincontrol.RedeemCodeList, error) {
	var collection redeemCodeCollection
	if err := s.loadTyped(ctx, settingsKeyRedeemCodes, &collection); err != nil {
		return admincontrol.RedeemCodeList{}, err
	}
	now := s.clock.Now()
	items := make([]admincontrol.RedeemCode, 0, len(collection.Items))
	for _, item := range collection.Items {
		item = redeemCodeWithDerivedStatus(item, now)
		if opts.Status != "" && string(item.Status) != opts.Status {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return admincontrol.RedeemCodeList{Items: pageItems(items, opts), Total: len(items)}, nil
}

func (s *Service) CreateRedeemCode(ctx context.Context, req admincontrol.CreateRedeemCodeRequest, actorUserID int) (admincontrol.RedeemCode, error) {
	var collection redeemCodeCollection
	if err := s.loadTyped(ctx, settingsKeyRedeemCodes, &collection); err != nil {
		return admincontrol.RedeemCode{}, err
	}
	code, err := redeemCodeFromCreateRequest(req, nextID(collection.NextID, len(collection.Items)), s.clock.Now())
	if err != nil {
		return admincontrol.RedeemCode{}, err
	}
	if redeemCodeExists(collection.Items, code.Code) {
		return admincontrol.RedeemCode{}, admincontrol.ErrConflict
	}
	collection.Items = append(collection.Items, code)
	collection.NextID = code.ID + 1
	if err := s.saveTyped(ctx, settingsKeyRedeemCodes, collection, actorUserID); err != nil {
		return admincontrol.RedeemCode{}, err
	}
	return code, nil
}

func (s *Service) BatchGenerateRedeemCodes(ctx context.Context, req admincontrol.BatchGenerateRedeemCodesRequest, actorUserID int) ([]admincontrol.RedeemCode, error) {
	if req.Count <= 0 || req.Count > 1000 {
		return nil, admincontrol.ErrInvalidInput
	}
	var collection redeemCodeCollection
	if err := s.loadTyped(ctx, settingsKeyRedeemCodes, &collection); err != nil {
		return nil, err
	}
	now := s.clock.Now()
	created := make([]admincontrol.RedeemCode, 0, req.Count)
	next := nextID(collection.NextID, len(collection.Items))
	for len(created) < req.Count {
		generatedCode, err := randomCode(defaultString(req.Prefix, "SR"))
		if err != nil {
			return nil, err
		}
		if redeemCodeExists(collection.Items, generatedCode) || redeemCodeExists(created, generatedCode) {
			continue
		}
		code, err := redeemCodeFromBatchRequest(req, next, generatedCode, now)
		if err != nil {
			return nil, err
		}
		created = append(created, code)
		next++
	}
	collection.Items = append(collection.Items, created...)
	collection.NextID = next
	if err := s.saveTyped(ctx, settingsKeyRedeemCodes, collection, actorUserID); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Service) BatchDisableRedeemCodes(ctx context.Context, ids []int, actorUserID int) (admincontrol.BatchOperationResult, error) {
	if len(ids) == 0 || len(ids) > 1000 {
		return admincontrol.BatchOperationResult{}, admincontrol.ErrInvalidInput
	}
	var collection redeemCodeCollection
	if err := s.loadTyped(ctx, settingsKeyRedeemCodes, &collection); err != nil {
		return admincontrol.BatchOperationResult{}, err
	}
	idSet := map[int]bool{}
	for _, id := range ids {
		if id <= 0 {
			return admincontrol.BatchOperationResult{}, admincontrol.ErrInvalidInput
		}
		idSet[id] = true
	}
	now := s.clock.Now()
	var succeeded int
	for idx, item := range collection.Items {
		if !idSet[item.ID] {
			continue
		}
		item.Status = admincontrol.RedeemCodeStatusDisabled
		item.UpdatedAt = now
		collection.Items[idx] = item
		succeeded++
		delete(idSet, item.ID)
	}
	if succeeded > 0 {
		if err := s.saveTyped(ctx, settingsKeyRedeemCodes, collection, actorUserID); err != nil {
			return admincontrol.BatchOperationResult{}, err
		}
	}
	failedIDs := make([]int, 0, len(idSet))
	for id := range idSet {
		failedIDs = append(failedIDs, id)
	}
	sort.Ints(failedIDs)
	return admincontrol.BatchOperationResult{
		Requested: len(ids),
		Succeeded: succeeded,
		Failed:    len(failedIDs),
		FailedIDs: failedIDs,
	}, nil
}

func (s *Service) RedeemCodeStats(ctx context.Context) (admincontrol.RedeemCodeStats, error) {
	var collection redeemCodeCollection
	if err := s.loadTyped(ctx, settingsKeyRedeemCodes, &collection); err != nil {
		return admincontrol.RedeemCodeStats{}, err
	}
	now := s.clock.Now()
	stats := admincontrol.RedeemCodeStats{Total: len(collection.Items)}
	for _, item := range collection.Items {
		switch redeemCodeWithDerivedStatus(item, now).Status {
		case admincontrol.RedeemCodeStatusActive:
			stats.Active++
		case admincontrol.RedeemCodeStatusRedeemed:
			stats.Redeemed++
		case admincontrol.RedeemCodeStatusDisabled:
			stats.Disabled++
		case admincontrol.RedeemCodeStatusExpired:
			stats.Expired++
		}
	}
	return stats, nil
}

func (s *Service) ListPromoCodes(ctx context.Context, opts admincontrol.ListOptions) (admincontrol.PromoCodeList, error) {
	var collection promoCodeCollection
	if err := s.loadTyped(ctx, settingsKeyPromoCodes, &collection); err != nil {
		return admincontrol.PromoCodeList{}, err
	}
	now := s.clock.Now()
	items := make([]admincontrol.PromoCode, 0, len(collection.Items))
	for _, item := range collection.Items {
		item = promoCodeWithDerivedStatus(item, now)
		if opts.Status != "" && string(item.Status) != opts.Status {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return admincontrol.PromoCodeList{Items: pageItems(items, opts), Total: len(items)}, nil
}

func (s *Service) CreatePromoCode(ctx context.Context, req admincontrol.PromoCodeRequest, actorUserID int) (admincontrol.PromoCode, error) {
	var collection promoCodeCollection
	if err := s.loadTyped(ctx, settingsKeyPromoCodes, &collection); err != nil {
		return admincontrol.PromoCode{}, err
	}
	item, err := promoCodeFromRequest(req, nextID(collection.NextID, len(collection.Items)), s.clock.Now(), nil)
	if err != nil {
		return admincontrol.PromoCode{}, err
	}
	if promoCodeExists(collection.Items, item.Code) {
		return admincontrol.PromoCode{}, admincontrol.ErrConflict
	}
	collection.Items = append(collection.Items, item)
	collection.NextID = item.ID + 1
	if err := s.saveTyped(ctx, settingsKeyPromoCodes, collection, actorUserID); err != nil {
		return admincontrol.PromoCode{}, err
	}
	return item, nil
}

func (s *Service) UpdatePromoCode(ctx context.Context, id int, req admincontrol.PromoCodeRequest, actorUserID int) (admincontrol.PromoCode, error) {
	var collection promoCodeCollection
	if err := s.loadTyped(ctx, settingsKeyPromoCodes, &collection); err != nil {
		return admincontrol.PromoCode{}, err
	}
	for idx, item := range collection.Items {
		if item.ID != id {
			continue
		}
		updated, err := promoCodeFromRequest(req, id, s.clock.Now(), &item)
		if err != nil {
			return admincontrol.PromoCode{}, err
		}
		if !strings.EqualFold(item.Code, updated.Code) && promoCodeExists(collection.Items, updated.Code) {
			return admincontrol.PromoCode{}, admincontrol.ErrConflict
		}
		collection.Items[idx] = updated
		if err := s.saveTyped(ctx, settingsKeyPromoCodes, collection, actorUserID); err != nil {
			return admincontrol.PromoCode{}, err
		}
		return updated, nil
	}
	return admincontrol.PromoCode{}, admincontrol.ErrNotFound
}

func (s *Service) DeletePromoCode(ctx context.Context, id int, actorUserID int) (admincontrol.PromoCode, error) {
	var collection promoCodeCollection
	if err := s.loadTyped(ctx, settingsKeyPromoCodes, &collection); err != nil {
		return admincontrol.PromoCode{}, err
	}
	for idx, item := range collection.Items {
		if item.ID != id {
			continue
		}
		collection.Items = append(collection.Items[:idx], collection.Items[idx+1:]...)
		if err := s.saveTyped(ctx, settingsKeyPromoCodes, collection, actorUserID); err != nil {
			return admincontrol.PromoCode{}, err
		}
		return item, nil
	}
	return admincontrol.PromoCode{}, admincontrol.ErrNotFound
}

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

func (s *Service) ListSystemLogs(ctx context.Context, opts admincontrol.ListOptions) (admincontrol.SystemLogList, error) {
	var collection systemLogCollection
	if err := s.loadTyped(ctx, settingsKeySystemLogs, &collection); err != nil {
		return admincontrol.SystemLogList{}, err
	}
	items := make([]admincontrol.OpsSystemLog, 0, len(collection.Items))
	for _, item := range collection.Items {
		if opts.Level != "" && string(item.Level) != opts.Level {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return admincontrol.SystemLogList{Items: pageItems(items, opts), Total: len(items)}, nil
}

func (s *Service) loadTyped(ctx context.Context, key string, dst any) error {
	raw, ok, err := s.store.Get(ctx, key)
	if err != nil || !ok {
		return err
	}
	return mapToTyped(raw, dst)
}

func (s *Service) saveTyped(ctx context.Context, key string, value any, actorUserID int) error {
	raw, err := typedToMap(value)
	if err != nil {
		return err
	}
	return s.store.Set(ctx, key, raw, &actorUserID)
}

type announcementCollection struct {
	NextID int                         `json:"next_id"`
	Items  []admincontrol.Announcement `json:"items"`
}

type redeemCodeCollection struct {
	NextID int                       `json:"next_id"`
	Items  []admincontrol.RedeemCode `json:"items"`
}

type promoCodeCollection struct {
	NextID int                      `json:"next_id"`
	Items  []admincontrol.PromoCode `json:"items"`
}

type systemLogCollection struct {
	NextID int                         `json:"next_id"`
	Items  []admincontrol.OpsSystemLog `json:"items"`
}

type riskLogCollection struct {
	NextID int                           `json:"next_id"`
	Items  []admincontrol.RiskControlLog `json:"items"`
}

func announcementFromCreateRequest(req admincontrol.AnnouncementRequest, id int, now time.Time) (admincontrol.Announcement, error) {
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)
	if title == "" || content == "" {
		return admincontrol.Announcement{}, admincontrol.ErrInvalidInput
	}
	status := req.Status
	if status == "" {
		status = admincontrol.AnnouncementStatusDraft
	}
	severity := req.Severity
	if severity == "" {
		severity = admincontrol.AnnouncementSeverityInfo
	}
	audience := req.Audience
	if audience == "" {
		audience = admincontrol.AnnouncementAudienceAll
	}
	if !status.Valid() || !severity.Valid() || !audience.Valid() || !validTimeRange(req.StartsAt, req.EndsAt) {
		return admincontrol.Announcement{}, admincontrol.ErrInvalidInput
	}
	return admincontrol.Announcement{
		ID:        id,
		Title:     title,
		Content:   content,
		Status:    status,
		Severity:  severity,
		Audience:  audience,
		StartsAt:  req.StartsAt,
		EndsAt:    req.EndsAt,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func redeemCodeFromCreateRequest(req admincontrol.CreateRedeemCodeRequest, id int, now time.Time) (admincontrol.RedeemCode, error) {
	if !req.Type.Valid() || strings.TrimSpace(req.Code) == "" || !validDecimal(req.Value) {
		return admincontrol.RedeemCode{}, admincontrol.ErrInvalidInput
	}
	maxRedemptions := req.MaxRedemptions
	if maxRedemptions == 0 {
		maxRedemptions = 1
	}
	if maxRedemptions <= 0 {
		return admincontrol.RedeemCode{}, admincontrol.ErrInvalidInput
	}
	return admincontrol.RedeemCode{
		ID:             id,
		Code:           normalizeCode(req.Code),
		Type:           req.Type,
		Status:         admincontrol.RedeemCodeStatusActive,
		Value:          strings.TrimSpace(req.Value),
		Currency:       normalizeCurrency(req.Currency),
		MaxRedemptions: maxRedemptions,
		RedeemedCount:  0,
		ExpiresAt:      req.ExpiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func redeemCodeFromBatchRequest(req admincontrol.BatchGenerateRedeemCodesRequest, id int, code string, now time.Time) (admincontrol.RedeemCode, error) {
	if !req.Type.Valid() || !validDecimal(req.Value) {
		return admincontrol.RedeemCode{}, admincontrol.ErrInvalidInput
	}
	maxRedemptions := req.MaxRedemptions
	if maxRedemptions == 0 {
		maxRedemptions = 1
	}
	if maxRedemptions <= 0 {
		return admincontrol.RedeemCode{}, admincontrol.ErrInvalidInput
	}
	return admincontrol.RedeemCode{
		ID:             id,
		Code:           normalizeCode(code),
		Type:           req.Type,
		Status:         admincontrol.RedeemCodeStatusActive,
		Value:          strings.TrimSpace(req.Value),
		Currency:       normalizeCurrency(req.Currency),
		MaxRedemptions: maxRedemptions,
		RedeemedCount:  0,
		ExpiresAt:      req.ExpiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func promoCodeFromRequest(req admincontrol.PromoCodeRequest, id int, now time.Time, existing *admincontrol.PromoCode) (admincontrol.PromoCode, error) {
	if strings.TrimSpace(req.Code) == "" || !req.DiscountType.Valid() || !validDecimal(req.DiscountValue) || !validTimeRange(req.StartsAt, req.ExpiresAt) {
		return admincontrol.PromoCode{}, admincontrol.ErrInvalidInput
	}
	if req.DiscountType == admincontrol.PromoDiscountTypePercent && !validPercentDecimal(req.DiscountValue) {
		return admincontrol.PromoCode{}, admincontrol.ErrInvalidInput
	}
	maxUses := req.MaxUses
	if maxUses == 0 {
		maxUses = 1
	}
	if maxUses <= 0 {
		return admincontrol.PromoCode{}, admincontrol.ErrInvalidInput
	}
	status := req.Status
	if status == "" {
		status = admincontrol.PromoCodeStatusActive
	}
	if !status.Valid() {
		return admincontrol.PromoCode{}, admincontrol.ErrInvalidInput
	}
	createdAt := now
	usedCount := 0
	if existing != nil {
		createdAt = existing.CreatedAt
		usedCount = existing.UsedCount
	}
	return admincontrol.PromoCode{
		ID:            id,
		Code:          normalizeCode(req.Code),
		Status:        status,
		DiscountType:  req.DiscountType,
		DiscountValue: strings.TrimSpace(req.DiscountValue),
		Currency:      normalizeCurrency(req.Currency),
		MaxUses:       maxUses,
		UsedCount:     usedCount,
		StartsAt:      req.StartsAt,
		ExpiresAt:     req.ExpiresAt,
		CreatedAt:     createdAt,
		UpdatedAt:     now,
	}, nil
}

func defaultAdminSettings(now time.Time) admincontrol.AdminSettings {
	return admincontrol.AdminSettings{
		General: admincontrol.AdminSettingsGeneral{
			SiteName:     "SRapi",
			LogoURL:      "",
			VersionLabel: "",
			CustomMenus:  []map[string]any{},
		},
		Agreement: admincontrol.AdminSettingsAgreement{},
		Features: admincontrol.AdminSettingsFeatures{
			EnabledChannels:          []string{},
			ChannelMonitoringEnabled: true,
			InvitationRebateEnabled:  false,
			PaymentsEnabled:          false,
		},
		Security: admincontrol.AdminSettingsSecurity{
			AdminAPIKey:         admincontrol.SecretConfigured{Configured: false},
			RegistrationEnabled: true,
			OAuthEnabled:        false,
			OAuthProviders:      []string{},
		},
		Users: admincontrol.AdminSettingsUsers{
			DefaultBalance:        "0",
			DefaultGroup:          "default",
			UserSelfDeleteEnabled: false,
			RPMLimitDefault:       0,
		},
		Gateway: admincontrol.AdminSettingsGateway{
			OverloadCooldownSeconds:              30,
			RateLimitCooldownSeconds:             30,
			StreamTimeoutSeconds:                 600,
			RequestShaperEnabled:                 true,
			BetaStrategy:                         "allow_configured",
			SchedulerStrategyRolloutEnabled:      false,
			SchedulerStrategyShadowStrategy:      "",
			SchedulerStrategyRolloutPercent:      0,
			SchedulerStrategyRolloutModels:       []string{},
			SchedulerStrategyRolloutAPIKeyHashes: []string{},
		},
		Payment: admincontrol.AdminSettingsPayment{
			Enabled:                  false,
			Providers:                []string{},
			SubscriptionPlansEnabled: false,
		},
		Email: admincontrol.AdminSettingsEmail{
			SMTPConfigured: false,
			Templates:      map[string]string{},
		},
		Backup: admincontrol.AdminSettingsBackup{
			Enabled:       false,
			LastBackupAt:  &now,
			RetentionDays: 30,
		},
	}
}

func normalizeAdminSettings(settings admincontrol.AdminSettings) (admincontrol.AdminSettings, error) {
	settings.General.SiteName = strings.TrimSpace(settings.General.SiteName)
	settings.General.LogoURL = strings.TrimSpace(settings.General.LogoURL)
	settings.General.VersionLabel = strings.TrimSpace(settings.General.VersionLabel)
	settings.Users.DefaultBalance = strings.TrimSpace(settings.Users.DefaultBalance)
	settings.Users.DefaultGroup = strings.TrimSpace(settings.Users.DefaultGroup)
	settings.Gateway.BetaStrategy = strings.TrimSpace(settings.Gateway.BetaStrategy)
	settings.Gateway.SchedulerStrategyShadowStrategy = strings.TrimSpace(settings.Gateway.SchedulerStrategyShadowStrategy)
	settings.Gateway.SchedulerStrategyRolloutModels = uniqueTrimmedStrings(settings.Gateway.SchedulerStrategyRolloutModels)
	settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes = uniqueTrimmedStrings(settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes)
	if settings.General.SiteName == "" || !validDecimal(settings.Users.DefaultBalance) || settings.Users.RPMLimitDefault < 0 || settings.Gateway.StreamTimeoutSeconds <= 0 || settings.Backup.RetentionDays <= 0 {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Gateway.SchedulerStrategyRolloutPercent < 0 || settings.Gateway.SchedulerStrategyRolloutPercent > 100 {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Gateway.SchedulerStrategyRolloutEnabled && (settings.Gateway.SchedulerStrategyShadowStrategy == "" || settings.Gateway.SchedulerStrategyRolloutPercent <= 0) {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.General.CustomMenus == nil {
		settings.General.CustomMenus = []map[string]any{}
	}
	if settings.Features.EnabledChannels == nil {
		settings.Features.EnabledChannels = []string{}
	}
	if settings.Security.OAuthProviders == nil {
		settings.Security.OAuthProviders = []string{}
	}
	if settings.Payment.Providers == nil {
		settings.Payment.Providers = []string{}
	}
	if settings.Email.Templates == nil {
		settings.Email.Templates = map[string]string{}
	}
	if settings.Gateway.SchedulerStrategyRolloutModels == nil {
		settings.Gateway.SchedulerStrategyRolloutModels = []string{}
	}
	if settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes == nil {
		settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes = []string{}
	}
	return settings, nil
}

func uniqueTrimmedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func defaultOpsSettings() admincontrol.OpsSettings {
	return admincontrol.OpsSettings{
		AutoRefreshEnabled:     true,
		RefreshIntervalSeconds: 15,
		ErrorRateThreshold:     0.05,
		LatencyP95ThresholdMS:  5000,
		AlertRetentionDays:     30,
	}
}

func validateOpsSettings(settings admincontrol.OpsSettings) error {
	if settings.RefreshIntervalSeconds < 5 || settings.RefreshIntervalSeconds > 3600 || settings.ErrorRateThreshold < 0 || settings.ErrorRateThreshold > 1 || settings.LatencyP95ThresholdMS <= 0 || settings.AlertRetentionDays <= 0 || settings.AlertRetentionDays > 365 {
		return admincontrol.ErrInvalidInput
	}
	return nil
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

func mapToTyped(raw map[string]any, dst any) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, dst)
}

func typedToMap(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func pageItems[T any](items []T, opts admincontrol.ListOptions) []T {
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []T{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func nextID(current, itemCount int) int {
	if current > 0 {
		return current
	}
	return itemCount + 1
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizeCurrency(value string) string {
	currency := strings.ToUpper(strings.TrimSpace(value))
	if currency == "" {
		return "USD"
	}
	return currency
}

func normalizeCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func validDecimal(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	_, ok := new(big.Rat).SetString(value)
	return ok
}

func validPercentDecimal(value string) bool {
	ratio, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok {
		return false
	}
	return ratio.Sign() >= 0 && ratio.Cmp(big.NewRat(1, 1)) <= 0
}

func validTimeRange(start, end *time.Time) bool {
	return start == nil || end == nil || end.After(*start)
}

func redeemCodeExists(items []admincontrol.RedeemCode, code string) bool {
	code = normalizeCode(code)
	for _, item := range items {
		if normalizeCode(item.Code) == code {
			return true
		}
	}
	return false
}

func promoCodeExists(items []admincontrol.PromoCode, code string) bool {
	code = normalizeCode(code)
	for _, item := range items {
		if normalizeCode(item.Code) == code {
			return true
		}
	}
	return false
}

func redeemCodeWithDerivedStatus(item admincontrol.RedeemCode, now time.Time) admincontrol.RedeemCode {
	if item.Status == admincontrol.RedeemCodeStatusActive && item.ExpiresAt != nil && item.ExpiresAt.Before(now) {
		item.Status = admincontrol.RedeemCodeStatusExpired
	}
	return item
}

func promoCodeWithDerivedStatus(item admincontrol.PromoCode, now time.Time) admincontrol.PromoCode {
	if item.Status == admincontrol.PromoCodeStatusActive && item.ExpiresAt != nil && item.ExpiresAt.Before(now) {
		item.Status = admincontrol.PromoCodeStatusExpired
	}
	return item
}

func randomCode(prefix string) (string, error) {
	prefix = normalizeCode(prefix)
	if prefix == "" {
		prefix = "SR"
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + "-" + strings.ToUpper(hex.EncodeToString(buf)), nil
}
