package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/ttlcache"
)

const (
	settingsKeyAnnouncements = "admin_control.announcements"
	settingsKeyRedeemCodes   = "admin_control.redeem_codes"
	settingsKeyPromoCodes    = "admin_control.promo_codes"
	settingsKeySystemLogs    = "admin_control.system_logs"
	settingsKeyRiskLogs      = "admin_control.risk_logs"
	settingsKeyRiskConfig    = "admin_control.risk_config"
	settingsKeyContentSafety = "admin_control.content_safety_config"
	settingsKeyOpsSettings   = "admin_control.ops_settings"
	settingsKeyAdminSettings = "admin_control.admin_settings"

	defaultPageSize = 20
	maxPageSize     = 1000

	defaultSystemLogCleanupMax = 1000
	maxSystemLogCleanupMax     = 10000

	oauthTokenAuthMethodNone = "none"

	// adminSettingsCacheTTL bounds how long a cached admin-settings read may be
	// served. The gateway consults these settings several times per request
	// (retry policy, channel filter, request shaper, passthrough headers), so
	// reads come from this cache instead of the settings store. Same-instance
	// updates invalidate immediately; cross-instance updates converge within
	// the TTL.
	adminSettingsCacheTTL = 3 * time.Second
)

var emailSuffixDomainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

var defaultGatewayPassthroughHeaderAllowlist = []string{
	"retry-after",
	"request-id",
	"x-request-id",
	"x-upstream-request-id",
	"cache-control",
	"content-language",
	"date",
	"etag",
	"expires",
	"last-modified",
	"location",
	"vary",
	"www-authenticate",
	"x-ratelimit-*",
	"ratelimit-*",
	"anthropic-ratelimit-*",
	"x-codex-primary-used-percent",
	"x-codex-primary-reset-after-seconds",
	"x-codex-primary-window-minutes",
	"x-codex-secondary-used-percent",
	"x-codex-secondary-reset-after-seconds",
	"x-codex-secondary-window-minutes",
	"x-codex-primary-over-secondary-limit-percent",
}

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store admincontrol.Store
	clock Clock
	// settingsCache holds the last loaded AdminSettings. The cached value is
	// shared across goroutines and must be treated as immutable — every write
	// path already builds new nested maps/slices instead of mutating in place.
	settingsCache *ttlcache.Value[admincontrol.AdminSettings]
}

func New(store admincontrol.Store, clock Clock) (*Service, error) {
	if store == nil {
		return nil, admincontrol.ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	svc := &Service{store: store, clock: clock}
	svc.settingsCache = ttlcache.New[admincontrol.AdminSettings](adminSettingsCacheTTL, svc.clock.Now)
	return svc, nil
}

func (s *Service) GetAdminSettings(ctx context.Context) (admincontrol.AdminSettings, error) {
	settings, err := s.settingsCache.Get(ctx, s.loadAdminSettings)
	if err != nil {
		return admincontrol.AdminSettings{}, err
	}
	return cloneAdminSettings(settings), nil
}

func (s *Service) loadAdminSettings(ctx context.Context) (admincontrol.AdminSettings, error) {
	settings := defaultAdminSettings(s.clock.Now())
	if err := s.loadTyped(ctx, settingsKeyAdminSettings, &settings); err != nil {
		return admincontrol.AdminSettings{}, err
	}
	return settings, nil
}

func (s *Service) NotificationEmailTemplates(ctx context.Context) map[string]string {
	settings, err := s.GetAdminSettings(ctx)
	if err != nil {
		return map[string]string{}
	}
	return cloneStringMap(settings.Email.Templates)
}

func (s *Service) UpdateAdminSettings(ctx context.Context, settings admincontrol.AdminSettings, actorUserID int) (admincontrol.AdminSettings, error) {
	normalized, err := normalizeAdminSettings(settings)
	if err != nil {
		return admincontrol.AdminSettings{}, err
	}
	if err := s.saveTyped(ctx, settingsKeyAdminSettings, normalized, actorUserID); err != nil {
		return admincontrol.AdminSettings{}, err
	}
	s.settingsCache.Invalidate()
	return cloneAdminSettings(normalized), nil
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

func (s *Service) ListUserAnnouncements(ctx context.Context, user userscontract.User, opts admincontrol.ListOptions) (admincontrol.UserAnnouncementList, error) {
	if user.ID <= 0 {
		return admincontrol.UserAnnouncementList{}, admincontrol.ErrInvalidInput
	}
	var collection announcementCollection
	if err := s.loadTyped(ctx, settingsKeyAnnouncements, &collection); err != nil {
		return admincontrol.UserAnnouncementList{}, err
	}
	now := s.clock.Now()
	visible := make([]admincontrol.Announcement, 0, len(collection.Items))
	for _, item := range collection.Items {
		if !announcementVisibleToUser(item, user, now) {
			continue
		}
		visible = append(visible, item)
	}
	sort.SliceStable(visible, func(i, j int) bool { return visible[i].CreatedAt.After(visible[j].CreatedAt) })

	ids := announcementIDs(visible)
	reads, err := s.store.ListAnnouncementReads(ctx, user.ID, ids)
	if err != nil {
		return admincontrol.UserAnnouncementList{}, err
	}
	readByAnnouncement := announcementReadByID(reads)
	items := make([]admincontrol.UserAnnouncement, 0, len(visible))
	var unread int
	for _, item := range visible {
		userItem := admincontrol.UserAnnouncement{Announcement: item}
		if read, ok := readByAnnouncement[item.ID]; ok && !read.ReadAt.Before(item.UpdatedAt) {
			userItem.Read = true
			readAt := read.ReadAt
			userItem.ReadAt = &readAt
		} else {
			unread++
		}
		items = append(items, userItem)
	}
	return admincontrol.UserAnnouncementList{
		Items:  pageItems(items, opts),
		Total:  len(items),
		Unread: unread,
	}, nil
}

func (s *Service) MarkUserAnnouncementRead(ctx context.Context, user userscontract.User, announcementID int) (admincontrol.UserAnnouncement, error) {
	if user.ID <= 0 || announcementID <= 0 {
		return admincontrol.UserAnnouncement{}, admincontrol.ErrInvalidInput
	}
	var collection announcementCollection
	if err := s.loadTyped(ctx, settingsKeyAnnouncements, &collection); err != nil {
		return admincontrol.UserAnnouncement{}, err
	}
	now := s.clock.Now()
	for _, item := range collection.Items {
		if item.ID != announcementID {
			continue
		}
		if !announcementVisibleToUser(item, user, now) {
			return admincontrol.UserAnnouncement{}, admincontrol.ErrNotFound
		}
		reads, err := s.store.ListAnnouncementReads(ctx, user.ID, []int{announcementID})
		if err != nil {
			return admincontrol.UserAnnouncement{}, err
		}
		if len(reads) > 0 && !reads[0].ReadAt.Before(item.UpdatedAt) {
			readAt := reads[0].ReadAt
			return admincontrol.UserAnnouncement{
				Announcement: item,
				Read:         true,
				ReadAt:       &readAt,
			}, nil
		}
		read, err := s.store.MarkAnnouncementRead(ctx, user.ID, announcementID, now)
		if err != nil {
			return admincontrol.UserAnnouncement{}, err
		}
		readAt := read.ReadAt
		return admincontrol.UserAnnouncement{
			Announcement: item,
			Read:         true,
			ReadAt:       &readAt,
		}, nil
	}
	return admincontrol.UserAnnouncement{}, admincontrol.ErrNotFound
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

func (s *Service) DeleteRedeemCode(ctx context.Context, id int, actorUserID int) (admincontrol.RedeemCode, error) {
	var collection redeemCodeCollection
	if err := s.loadTyped(ctx, settingsKeyRedeemCodes, &collection); err != nil {
		return admincontrol.RedeemCode{}, err
	}
	for idx, item := range collection.Items {
		if item.ID != id {
			continue
		}
		collection.Items = append(collection.Items[:idx], collection.Items[idx+1:]...)
		if err := s.saveTyped(ctx, settingsKeyRedeemCodes, collection, actorUserID); err != nil {
			return admincontrol.RedeemCode{}, err
		}
		return item, nil
	}
	return admincontrol.RedeemCode{}, admincontrol.ErrNotFound
}

func (s *Service) RedeemCode(ctx context.Context, user userscontract.User, req admincontrol.RedeemCodeRedemptionRequest) (admincontrol.RedeemCodeRedemptionResult, error) {
	if user.ID <= 0 {
		return admincontrol.RedeemCodeRedemptionResult{}, admincontrol.ErrInvalidInput
	}
	code := normalizeCode(req.Code)
	if code == "" {
		return admincontrol.RedeemCodeRedemptionResult{}, admincontrol.ErrInvalidInput
	}
	return s.store.RedeemCode(ctx, admincontrol.RedeemCodeRedemptionInput{
		UserID:     user.ID,
		Code:       code,
		RedeemedAt: s.clock.Now(),
	})
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

// ListPromoCodeUsages returns the redemption history for one promo code.
func (s *Service) ListPromoCodeUsages(ctx context.Context, promoCodeID, limit int) ([]admincontrol.PromoCodeApplication, error) {
	if promoCodeID <= 0 {
		return nil, admincontrol.ErrInvalidInput
	}
	return s.store.ListPromoCodeUsages(ctx, promoCodeID, limit)
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

func (s *Service) RecordSystemLog(ctx context.Context, req admincontrol.RecordSystemLogRequest) (admincontrol.OpsSystemLog, error) {
	log, err := systemLogFromRecordRequest(req, s.clock.Now())
	if err != nil {
		return admincontrol.OpsSystemLog{}, err
	}
	if store, ok := s.systemLogStore(); ok {
		return store.CreateSystemLog(ctx, log)
	}
	var collection systemLogCollection
	if err := s.loadTyped(ctx, settingsKeySystemLogs, &collection); err != nil {
		return admincontrol.OpsSystemLog{}, err
	}
	log.ID = nextID(collection.NextID, len(collection.Items))
	collection.Items = append(collection.Items, log)
	collection.NextID = log.ID + 1
	if err := s.saveTyped(ctx, settingsKeySystemLogs, collection, 0); err != nil {
		return admincontrol.OpsSystemLog{}, err
	}
	return log, nil
}

func (s *Service) ListSystemLogs(ctx context.Context, opts admincontrol.SystemLogListOptions) (admincontrol.SystemLogList, error) {
	if err := validateSystemLogListOptions(opts); err != nil {
		return admincontrol.SystemLogList{}, err
	}
	if store, ok := s.systemLogStore(); ok {
		return store.ListSystemLogs(ctx, opts)
	}
	var collection systemLogCollection
	if err := s.loadTyped(ctx, settingsKeySystemLogs, &collection); err != nil {
		return admincontrol.SystemLogList{}, err
	}
	items := make([]admincontrol.OpsSystemLog, 0, len(collection.Items))
	for _, item := range collection.Items {
		if !systemLogMatches(item, systemLogFilterFromListOptions(opts)) {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return admincontrol.SystemLogList{Items: pageItems(items, listOptionsFromSystemLogOptions(opts)), Total: len(items)}, nil
}

func (s *Service) CleanupSystemLogs(ctx context.Context, filter admincontrol.SystemLogCleanupFilter) (admincontrol.SystemLogCleanupResult, error) {
	normalized, err := normalizeSystemLogCleanupFilter(filter)
	if err != nil {
		return admincontrol.SystemLogCleanupResult{}, err
	}
	if store, ok := s.systemLogStore(); ok {
		return store.CleanupSystemLogs(ctx, normalized)
	}
	var collection systemLogCollection
	if err := s.loadTyped(ctx, settingsKeySystemLogs, &collection); err != nil {
		return admincontrol.SystemLogCleanupResult{}, err
	}
	kept := collection.Items[:0]
	var matched, deleted int
	for _, item := range collection.Items {
		if !systemLogMatches(item, normalized) {
			kept = append(kept, item)
			continue
		}
		matched++
		if normalized.DryRun || deleted >= normalized.MaxDelete {
			kept = append(kept, item)
			continue
		}
		deleted++
	}
	result := admincontrol.SystemLogCleanupResult{
		Matched:   matched,
		Deleted:   deleted,
		DryRun:    normalized.DryRun,
		MaxDelete: normalized.MaxDelete,
		Limited:   matched > deleted && !normalized.DryRun,
	}
	if normalized.DryRun {
		return result, nil
	}
	collection.Items = kept
	if err := s.saveTyped(ctx, settingsKeySystemLogs, collection, 0); err != nil {
		return admincontrol.SystemLogCleanupResult{}, err
	}
	return result, nil
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

func (s *Service) systemLogStore() (admincontrol.SystemLogStore, bool) {
	store, ok := s.store.(admincontrol.SystemLogStore)
	return store, ok
}

// AnnouncementReadStatus returns who has read one announcement (recent-first),
// for the admin read-status view.
func (s *Service) AnnouncementReadStatus(ctx context.Context, announcementID int) (admincontrol.AnnouncementReadStatus, error) {
	if announcementID <= 0 {
		return admincontrol.AnnouncementReadStatus{}, admincontrol.ErrInvalidInput
	}
	readers, err := s.store.ListAnnouncementReadsByAnnouncement(ctx, announcementID, 500)
	if err != nil {
		return admincontrol.AnnouncementReadStatus{}, err
	}
	return admincontrol.AnnouncementReadStatus{
		AnnouncementID: announcementID,
		Total:          len(readers),
		Readers:        readers,
	}, nil
}

func announcementVisibleToUser(item admincontrol.Announcement, user userscontract.User, now time.Time) bool {
	if item.Status != admincontrol.AnnouncementStatusPublished {
		return false
	}
	if item.StartsAt != nil && now.Before(item.StartsAt.UTC()) {
		return false
	}
	if item.EndsAt != nil && !now.Before(item.EndsAt.UTC()) {
		return false
	}
	if !announcementMatchesAudience(item.Audience, user.Roles) {
		return false
	}
	// Segments refine the audience: when present, at least one segment must
	// match the user. No segments = audience-only delivery (back-compat).
	if len(item.Segments) > 0 && !announcementMatchesSegments(item.Segments, user) {
		return false
	}
	return true
}

func announcementMatchesAudience(audience admincontrol.AnnouncementAudience, roles []userscontract.Role) bool {
	switch audience {
	case admincontrol.AnnouncementAudienceAll:
		return true
	case admincontrol.AnnouncementAudienceUsers:
		return !hasAdminRole(roles)
	case admincontrol.AnnouncementAudienceAdmins:
		return hasAdminRole(roles)
	default:
		return false
	}
}

func announcementMatchesSegments(segments []admincontrol.AnnouncementSegment, user userscontract.User) bool {
	for _, seg := range segments {
		if announcementSegmentMatches(seg, user) {
			return true
		}
	}
	return false
}

// announcementSegmentMatches is AND across the segment's non-empty conditions.
func announcementSegmentMatches(seg admincontrol.AnnouncementSegment, user userscontract.User) bool {
	if len(seg.Roles) > 0 && !userRolesIntersect(user.Roles, seg.Roles) {
		return false
	}
	if len(seg.UserIDs) > 0 && !containsInt(seg.UserIDs, user.ID) {
		return false
	}
	if len(seg.EmailDomains) > 0 && !emailDomainIn(user.Email, seg.EmailDomains) {
		return false
	}
	return true
}

func userRolesIntersect(roles []userscontract.Role, want []string) bool {
	for _, role := range roles {
		for _, w := range want {
			if strings.EqualFold(string(role), strings.TrimSpace(w)) {
				return true
			}
		}
	}
	return false
}

func containsInt(values []int, target int) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func emailDomainIn(email string, domains []string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return false
	}
	domain := strings.ToLower(strings.TrimSpace(email[at+1:]))
	for _, d := range domains {
		if strings.ToLower(strings.TrimSpace(d)) == domain {
			return true
		}
	}
	return false
}

func hasAdminRole(roles []userscontract.Role) bool {
	for _, role := range roles {
		if role == userscontract.RoleOwner || role == userscontract.RoleAdmin {
			return true
		}
	}
	return false
}

func announcementIDs(items []admincontrol.Announcement) []int {
	ids := make([]int, 0, len(items))
	for _, item := range items {
		if item.ID > 0 {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func announcementReadByID(reads []admincontrol.AnnouncementRead) map[int]admincontrol.AnnouncementRead {
	out := make(map[int]admincontrol.AnnouncementRead, len(reads))
	for _, read := range reads {
		if read.AnnouncementID <= 0 {
			continue
		}
		out[read.AnnouncementID] = read
	}
	return out
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
		Segments:  normalizeAnnouncementSegments(req.Segments),
		StartsAt:  req.StartsAt,
		EndsAt:    req.EndsAt,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// normalizeAnnouncementSegments trims/dedupes each segment's conditions and
// drops segments that carry no condition (which would otherwise match nobody).
func normalizeAnnouncementSegments(segments []admincontrol.AnnouncementSegment) []admincontrol.AnnouncementSegment {
	if len(segments) == 0 {
		return nil
	}
	out := make([]admincontrol.AnnouncementSegment, 0, len(segments))
	for _, seg := range segments {
		roles := uniqueTrimmedStrings(seg.Roles)
		domains := lowerUniqueTrimmedStrings(seg.EmailDomains)
		ids := uniquePositiveInts(seg.UserIDs)
		if len(roles) == 0 && len(domains) == 0 && len(ids) == 0 {
			continue
		}
		out = append(out, admincontrol.AnnouncementSegment{Roles: roles, UserIDs: ids, EmailDomains: domains})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func redeemCodeFromCreateRequest(req admincontrol.CreateRedeemCodeRequest, id int, now time.Time) (admincontrol.RedeemCode, error) {
	if !req.Type.Valid() || strings.TrimSpace(req.Code) == "" || !validRedeemCodeValue(req.Type, req.Value) {
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
	if !req.Type.Valid() || !validRedeemCodeValue(req.Type, req.Value) {
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
	if req.DiscountType == admincontrol.PromoDiscountTypeAmount && !validPositiveDecimal(req.DiscountValue) {
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
	perUserLimit := req.PerUserLimit
	if perUserLimit < 0 || perUserLimit > maxUses {
		return admincontrol.PromoCode{}, admincontrol.ErrInvalidInput
	}
	minOrderAmount := strings.TrimSpace(req.MinOrderAmount)
	if minOrderAmount != "" && !validPositiveDecimal(minOrderAmount) {
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
		ID:             id,
		Code:           normalizeCode(req.Code),
		Status:         status,
		DiscountType:   req.DiscountType,
		DiscountValue:  strings.TrimSpace(req.DiscountValue),
		Currency:       normalizeCurrency(req.Currency),
		MaxUses:        maxUses,
		PerUserLimit:   perUserLimit,
		MinOrderAmount: minOrderAmount,
		UsedCount:      usedCount,
		StartsAt:       req.StartsAt,
		ExpiresAt:      req.ExpiresAt,
		CreatedAt:      createdAt,
		UpdatedAt:      now,
	}, nil
}

func defaultAdminSettings(now time.Time) admincontrol.AdminSettings {
	balanceLowNotifyEnabled := true
	subscriptionExpiryNotifyEnabled := true
	accountQuotaNotifyEnabled := true
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
			AdminAPIKey:                      admincontrol.SecretConfigured{Configured: false},
			RegistrationEnabled:              true,
			RegistrationEmailSuffixAllowlist: []string{},
			OAuthEnabled:                     false,
			OAuthProviders:                   []string{},
			OAuthProviderConfigs:             []admincontrol.OAuthProviderConfig{},
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
			RetryCount:                           3,
			MaxRetryCredentials:                  0,
			MaxRetryIntervalMS:                   2000,
			SchedulerStrategyRolloutEnabled:      false,
			SchedulerStrategyShadowStrategy:      "",
			SchedulerStrategyRolloutPercent:      0,
			SchedulerStrategyRolloutModels:       []string{},
			SchedulerStrategyRolloutAPIKeyHashes: []string{},
			PassthroughUpstreamHeaders:           false,
			PassthroughHeaderAllowlist:           cloneStringSlice(defaultGatewayPassthroughHeaderAllowlist),
		},
		Payment: admincontrol.AdminSettingsPayment{
			Enabled:                  false,
			Providers:                []string{},
			SubscriptionPlansEnabled: false,
		},
		Email: admincontrol.AdminSettingsEmail{
			SMTPConfigured:                   false,
			SMTPHost:                         "",
			SMTPPort:                         587,
			SMTPUsername:                     "",
			SMTPFrom:                         "",
			SMTPFromName:                     "",
			SMTPUseTLS:                       false,
			PublicBaseURL:                    "",
			Templates:                        map[string]string{},
			BalanceLowNotifyEnabled:          &balanceLowNotifyEnabled,
			BalanceLowNotifyThreshold:        "5.00000000",
			BalanceLowNotifyRechargeURL:      "",
			SubscriptionExpiryNotifyEnabled:  &subscriptionExpiryNotifyEnabled,
			AccountQuotaNotifyEnabled:        &accountQuotaNotifyEnabled,
			AccountQuotaNotifyRemainingRatio: "0.20000000",
		},
		Backup: admincontrol.AdminSettingsBackup{
			Enabled:       false,
			LastBackupAt:  &now,
			RetentionDays: 30,
		},
		Copilot: admincontrol.AdminSettingsCopilot{
			Enabled:           false,
			Source:            "account",
			Models:            []string{},
			DedicatedProtocol: "openai-compatible",
			MaxSteps:          0, // vestigial: the copilot loop no longer enforces a per-turn step limit
			OwnerOnly:         false,
			AutoRunReads:      true,
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
	settings.Email.SMTPHost = strings.TrimSpace(settings.Email.SMTPHost)
	settings.Email.SMTPUsername = strings.TrimSpace(settings.Email.SMTPUsername)
	settings.Email.SMTPFrom = strings.TrimSpace(settings.Email.SMTPFrom)
	settings.Email.SMTPFromName = strings.TrimSpace(settings.Email.SMTPFromName)
	settings.Email.PublicBaseURL = strings.TrimRight(strings.TrimSpace(settings.Email.PublicBaseURL), "/")
	settings.Email.BalanceLowNotifyThreshold = strings.TrimSpace(settings.Email.BalanceLowNotifyThreshold)
	settings.Email.BalanceLowNotifyRechargeURL = strings.TrimSpace(settings.Email.BalanceLowNotifyRechargeURL)
	settings.Email.AccountQuotaNotifyRemainingRatio = strings.TrimSpace(settings.Email.AccountQuotaNotifyRemainingRatio)
	registrationEmailSuffixAllowlist, err := normalizeRegistrationEmailSuffixAllowlist(settings.Security.RegistrationEmailSuffixAllowlist)
	if err != nil {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	settings.Security.RegistrationEmailSuffixAllowlist = registrationEmailSuffixAllowlist
	settings.Security.OAuthProviders = uniqueTrimmedStrings(settings.Security.OAuthProviders)
	oauthProviderConfigs, err := normalizeOAuthProviderConfigs(settings.Security.OAuthProviderConfigs)
	if err != nil {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	settings.Security.OAuthProviderConfigs = oauthProviderConfigs
	settings.Gateway.SchedulerStrategyRolloutModels = uniqueTrimmedStrings(settings.Gateway.SchedulerStrategyRolloutModels)
	settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes = uniqueTrimmedStrings(settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes)
	settings.Gateway.RetryCount = normalizeGatewayRetryCount(settings.Gateway.RetryCount)
	settings.Gateway.MaxRetryCredentials = normalizeGatewayMaxRetryCredentials(settings.Gateway.MaxRetryCredentials)
	settings.Gateway.MaxRetryIntervalMS = normalizeGatewayMaxRetryIntervalMS(settings.Gateway.MaxRetryIntervalMS)
	settings.Gateway.PassthroughHeaderAllowlist = normalizePassthroughHeaderAllowlist(settings.Gateway.PassthroughHeaderAllowlist)
	if settings.General.SiteName == "" || !validDecimal(settings.Users.DefaultBalance) || settings.Users.RPMLimitDefault < 0 || settings.Gateway.StreamTimeoutSeconds <= 0 || settings.Backup.RetentionDays <= 0 {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Gateway.SchedulerStrategyRolloutPercent < 0 || settings.Gateway.SchedulerStrategyRolloutPercent > 100 {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Gateway.SchedulerStrategyRolloutEnabled && (settings.Gateway.SchedulerStrategyShadowStrategy == "" || settings.Gateway.SchedulerStrategyRolloutPercent <= 0) {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Email.SMTPPort == 0 {
		settings.Email.SMTPPort = 587
	}
	if settings.Email.SMTPPort < 0 || settings.Email.SMTPPort > 65535 {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Email.PublicBaseURL != "" && !validPublicHTTPBaseURL(settings.Email.PublicBaseURL) {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Email.BalanceLowNotifyThreshold == "" {
		settings.Email.BalanceLowNotifyThreshold = "5.00000000"
	}
	if !validPositiveDecimal(settings.Email.BalanceLowNotifyThreshold) {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Email.BalanceLowNotifyRechargeURL != "" && !validPublicHTTPBaseURL(settings.Email.BalanceLowNotifyRechargeURL) {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	if settings.Email.AccountQuotaNotifyRemainingRatio == "" {
		settings.Email.AccountQuotaNotifyRemainingRatio = "0.20000000"
	}
	if !validPercentDecimal(settings.Email.AccountQuotaNotifyRemainingRatio) {
		return admincontrol.AdminSettings{}, admincontrol.ErrInvalidInput
	}
	settings.Email.SMTPConfigured = settings.Email.SMTPHost != "" && settings.Email.SMTPFrom != ""
	if settings.General.CustomMenus == nil {
		settings.General.CustomMenus = []map[string]any{}
	}
	if settings.Features.EnabledChannels == nil {
		settings.Features.EnabledChannels = []string{}
	}
	if settings.Security.OAuthProviders == nil {
		settings.Security.OAuthProviders = []string{}
	}
	if settings.Security.OAuthProviderConfigs == nil {
		settings.Security.OAuthProviderConfigs = []admincontrol.OAuthProviderConfig{}
	}
	if settings.Security.RegistrationEmailSuffixAllowlist == nil {
		settings.Security.RegistrationEmailSuffixAllowlist = []string{}
	}
	if settings.Payment.Providers == nil {
		settings.Payment.Providers = []string{}
	}
	if settings.Email.Templates == nil {
		settings.Email.Templates = map[string]string{}
	}
	if settings.Email.BalanceLowNotifyEnabled == nil {
		enabled := true
		settings.Email.BalanceLowNotifyEnabled = &enabled
	}
	if settings.Email.SubscriptionExpiryNotifyEnabled == nil {
		enabled := true
		settings.Email.SubscriptionExpiryNotifyEnabled = &enabled
	}
	if settings.Email.AccountQuotaNotifyEnabled == nil {
		enabled := true
		settings.Email.AccountQuotaNotifyEnabled = &enabled
	}
	if settings.Gateway.SchedulerStrategyRolloutModels == nil {
		settings.Gateway.SchedulerStrategyRolloutModels = []string{}
	}
	if settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes == nil {
		settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes = []string{}
	}
	settings.Copilot.Source = strings.TrimSpace(strings.ToLower(settings.Copilot.Source))
	if settings.Copilot.Source != "dedicated" {
		settings.Copilot.Source = "account"
	}
	settings.Copilot.Model = strings.TrimSpace(settings.Copilot.Model)
	settings.Copilot.Models = uniqueTrimmedStrings(settings.Copilot.Models)
	if settings.Copilot.Models == nil {
		settings.Copilot.Models = []string{}
	}
	settings.Copilot.DedicatedProtocol = strings.TrimSpace(strings.ToLower(settings.Copilot.DedicatedProtocol))
	if settings.Copilot.DedicatedProtocol == "" {
		settings.Copilot.DedicatedProtocol = "openai-compatible"
	}
	settings.Copilot.DedicatedBaseURL = strings.TrimRight(strings.TrimSpace(settings.Copilot.DedicatedBaseURL), "/")
	if settings.Copilot.ProviderAccountID < 0 {
		settings.Copilot.ProviderAccountID = 0
	}
	// MaxSteps is vestigial: the copilot loop no longer enforces a per-turn step
	// limit (only an internal runaway guard), so the value is left as-is.
	return settings, nil
}

// gatewayRetryCountDefault / gatewayRetryCountMax bound the operator-tunable
// cross-candidate failover cap. They mirror the OpenAPI schema bounds for
// AdminSettingsGateway.retry_count and keep the failover hot path within a sane
// envelope even if persisted settings predate the field.
const (
	gatewayRetryCountDefault       = 3
	gatewayRetryCountMax           = 20
	gatewayMaxRetryIntervalDefault = 2000
)

func normalizeGatewayRetryCount(count int) int {
	if count <= 0 {
		return gatewayRetryCountDefault
	}
	if count > gatewayRetryCountMax {
		return gatewayRetryCountMax
	}
	return count
}

func normalizeGatewayMaxRetryCredentials(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func normalizeGatewayMaxRetryIntervalMS(value int) int {
	if value < 0 {
		return gatewayMaxRetryIntervalDefault
	}
	return value
}

func normalizeOAuthProviderConfigs(values []admincontrol.OAuthProviderConfig) ([]admincontrol.OAuthProviderConfig, error) {
	if len(values) == 0 {
		return []admincontrol.OAuthProviderConfig{}, nil
	}
	out := make([]admincontrol.OAuthProviderConfig, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		provider := normalizeOAuthProvider(value.Provider)
		providerKey := strings.TrimSpace(value.ProviderKey)
		if providerKey == "" {
			providerKey = provider
		}
		displayName := strings.TrimSpace(value.DisplayName)
		if displayName == "" {
			displayName = providerKey
		}
		clientID := strings.TrimSpace(value.ClientID)
		authorizeURL := strings.TrimSpace(value.AuthorizeURL)
		tokenURL := strings.TrimSpace(value.TokenURL)
		userInfoURL := strings.TrimSpace(value.UserInfoURL)
		tokenAuthMethod := normalizeOAuthTokenAuthMethod(value.TokenAuthMethod)
		redirectURI := strings.TrimSpace(value.RedirectURI)
		if provider == "" || providerKey == "" || clientID == "" || !validOAuthAuthorizeURL(authorizeURL) || !validOAuthRedirectURI(redirectURI) {
			return nil, admincontrol.ErrInvalidInput
		}
		if tokenAuthMethod == "" || (tokenURL == "") != (userInfoURL == "") {
			return nil, admincontrol.ErrInvalidInput
		}
		if tokenURL != "" && (!validOAuthBackchannelURL(tokenURL) || !validOAuthBackchannelURL(userInfoURL)) {
			return nil, admincontrol.ErrInvalidInput
		}
		key := strings.ToLower(provider + "\x00" + providerKey)
		if _, ok := seen[key]; ok {
			return nil, admincontrol.ErrConflict
		}
		seen[key] = struct{}{}
		out = append(out, admincontrol.OAuthProviderConfig{
			Provider:        provider,
			ProviderKey:     providerKey,
			DisplayName:     displayName,
			ClientID:        clientID,
			AuthorizeURL:    authorizeURL,
			TokenURL:        tokenURL,
			UserInfoURL:     userInfoURL,
			TokenAuthMethod: tokenAuthMethod,
			RedirectURI:     redirectURI,
			Scopes:          normalizeOAuthScopes(value.Scopes),
		})
	}
	return out, nil
}

func normalizeOAuthScopes(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, scope := range strings.Fields(strings.ReplaceAll(value, ",", " ")) {
			scope = strings.TrimSpace(scope)
			if scope == "" {
				continue
			}
			key := strings.ToLower(scope)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, scope)
		}
	}
	return out
}

func normalizeOAuthProvider(provider string) string {
	switch userscontract.AuthIdentityProvider(strings.ToLower(strings.TrimSpace(provider))) {
	case userscontract.AuthIdentityProviderOIDC:
		return string(userscontract.AuthIdentityProviderOIDC)
	case userscontract.AuthIdentityProviderGitHub:
		return string(userscontract.AuthIdentityProviderGitHub)
	case userscontract.AuthIdentityProviderGoogle:
		return string(userscontract.AuthIdentityProviderGoogle)
	case userscontract.AuthIdentityProviderLinuxDo:
		return string(userscontract.AuthIdentityProviderLinuxDo)
	case userscontract.AuthIdentityProviderWeChat:
		return string(userscontract.AuthIdentityProviderWeChat)
	case userscontract.AuthIdentityProviderDingTalk:
		return string(userscontract.AuthIdentityProviderDingTalk)
	default:
		return ""
	}
}

func normalizeOAuthTokenAuthMethod(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return oauthTokenAuthMethodNone
	}
	if value == oauthTokenAuthMethodNone {
		return oauthTokenAuthMethodNone
	}
	return ""
}

func validOAuthAuthorizeURL(value string) bool {
	parsed, ok := parseOAuthURL(value)
	return ok && parsed.Scheme == "https"
}

func validOAuthBackchannelURL(value string) bool {
	parsed, ok := parseOAuthURL(value)
	if !ok {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && localOAuthHost(parsed.Hostname())
}

func validOAuthRedirectURI(value string) bool {
	parsed, ok := parseOAuthURL(value)
	if !ok {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && localOAuthHost(parsed.Hostname())
}

func parseOAuthURL(value string) (*url.URL, bool) {
	if value == "" || strings.ContainsAny(value, "\r\n\t ") {
		return nil, false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, false
	}
	return parsed, true
}

func localOAuthHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func normalizeRegistrationEmailSuffixAllowlist(values []string) ([]string, error) {
	if len(values) == 0 {
		return []string{}, nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		suffix, err := normalizeRegistrationEmailSuffix(value)
		if err != nil {
			return nil, err
		}
		if suffix == "" {
			continue
		}
		if _, ok := seen[suffix]; ok {
			continue
		}
		seen[suffix] = struct{}{}
		out = append(out, suffix)
	}
	return out, nil
}

func normalizeRegistrationEmailSuffix(value string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "", nil
	}
	domain := trimmed
	if strings.Contains(trimmed, "@") {
		if !strings.HasPrefix(trimmed, "@") || strings.Count(trimmed, "@") != 1 {
			return "", admincontrol.ErrInvalidInput
		}
		domain = strings.TrimPrefix(trimmed, "@")
	}
	if domain == "" || strings.Contains(domain, "@") || !emailSuffixDomainPattern.MatchString(domain) {
		return "", admincontrol.ErrInvalidInput
	}
	return "@" + domain, nil
}

func validPublicHTTPBaseURL(value string) bool {
	if value == "" || strings.ContainsAny(value, "\r\n\t ") || strings.Contains(value, "?") || strings.Contains(value, "#") {
		return false
	}
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://")
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

// lowerUniqueTrimmedStrings is uniqueTrimmedStrings but lowercases the kept
// values (used for case-insensitive email domains).
func lowerUniqueTrimmedStrings(values []string) []string {
	out := uniqueTrimmedStrings(values)
	for i := range out {
		out[i] = strings.ToLower(out[i])
	}
	return out
}

func uniquePositiveInts(values []int) []int {
	out := make([]int, 0, len(values))
	seen := map[int]struct{}{}
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// normalizePassthroughHeaderAllowlist canonicalizes upstream response header
// allowlist entries to lowercase (HTTP header names are case-insensitive),
// trims whitespace, drops blanks, and dedupes. It always returns a non-nil
// slice so the persisted settings round-trip cleanly. A trailing "*" wildcard
// is preserved for prefix matching (e.g. "x-ratelimit-*").
func normalizePassthroughHeaderAllowlist(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" || trimmed == "*" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
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

func systemLogFromRecordRequest(req admincontrol.RecordSystemLogRequest, now time.Time) (admincontrol.OpsSystemLog, error) {
	level := req.Level
	if level == "" {
		level = admincontrol.OpsSystemLogLevelInfo
	}
	message := strings.TrimSpace(req.Message)
	source := strings.TrimSpace(req.Source)
	if !level.Valid() || message == "" || source == "" {
		return admincontrol.OpsSystemLog{}, admincontrol.ErrInvalidInput
	}
	createdAt := req.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	return admincontrol.OpsSystemLog{
		Level:     level,
		Message:   message,
		Source:    source,
		RequestID: strings.TrimSpace(req.RequestID),
		TraceID:   strings.TrimSpace(req.TraceID),
		Metadata:  cloneAnyMap(req.Metadata),
		CreatedAt: createdAt.UTC(),
	}, nil
}

func validateSystemLogListOptions(opts admincontrol.SystemLogListOptions) error {
	if opts.Level != "" && !opts.Level.Valid() {
		return admincontrol.ErrInvalidInput
	}
	if opts.Start != nil && opts.End != nil && opts.Start.After(*opts.End) {
		return admincontrol.ErrInvalidInput
	}
	return nil
}

func normalizeSystemLogCleanupFilter(filter admincontrol.SystemLogCleanupFilter) (admincontrol.SystemLogCleanupFilter, error) {
	filter.Source = strings.TrimSpace(filter.Source)
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.Level != "" && !filter.Level.Valid() {
		return admincontrol.SystemLogCleanupFilter{}, admincontrol.ErrInvalidInput
	}
	if filter.Start != nil && filter.End != nil && filter.Start.After(*filter.End) {
		return admincontrol.SystemLogCleanupFilter{}, admincontrol.ErrInvalidInput
	}
	if filter.Level == "" && filter.Source == "" && filter.Query == "" && filter.Start == nil && filter.End == nil {
		return admincontrol.SystemLogCleanupFilter{}, admincontrol.ErrInvalidInput
	}
	if filter.MaxDelete == 0 {
		filter.MaxDelete = defaultSystemLogCleanupMax
	}
	if filter.MaxDelete < 0 || filter.MaxDelete > maxSystemLogCleanupMax {
		return admincontrol.SystemLogCleanupFilter{}, admincontrol.ErrInvalidInput
	}
	return filter, nil
}

func systemLogFilterFromListOptions(opts admincontrol.SystemLogListOptions) admincontrol.SystemLogCleanupFilter {
	return admincontrol.SystemLogCleanupFilter{
		Level:  opts.Level,
		Source: strings.TrimSpace(opts.Source),
		Query:  strings.TrimSpace(opts.Query),
		Start:  opts.Start,
		End:    opts.End,
	}
}

func listOptionsFromSystemLogOptions(opts admincontrol.SystemLogListOptions) admincontrol.ListOptions {
	return admincontrol.ListOptions{Page: opts.Page, PageSize: opts.PageSize, Level: string(opts.Level)}
}

func systemLogMatches(log admincontrol.OpsSystemLog, filter admincontrol.SystemLogCleanupFilter) bool {
	if filter.Level != "" && log.Level != filter.Level {
		return false
	}
	if filter.Source != "" && !strings.EqualFold(log.Source, filter.Source) {
		return false
	}
	if filter.Start != nil && log.CreatedAt.Before(filter.Start.UTC()) {
		return false
	}
	if filter.End != nil && !log.CreatedAt.Before(filter.End.UTC()) {
		return false
	}
	if filter.Query != "" {
		query := strings.ToLower(filter.Query)
		if !strings.Contains(strings.ToLower(log.Message), query) && !strings.Contains(strings.ToLower(log.Source), query) && !strings.Contains(strings.ToLower(log.RequestID), query) && !strings.Contains(strings.ToLower(log.TraceID), query) {
			return false
		}
	}
	return true
}

func cloneAnyMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
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

func cloneStringMap(value map[string]string) map[string]string {
	out := map[string]string{}
	for key, item := range value {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = item
	}
	return out
}

func cloneAdminSettings(settings admincontrol.AdminSettings) admincontrol.AdminSettings {
	settings.General.CustomMenus = cloneAnyMapSlice(settings.General.CustomMenus)
	settings.Features.EnabledChannels = cloneStringSlice(settings.Features.EnabledChannels)
	settings.Security.RegistrationEmailSuffixAllowlist = cloneStringSlice(settings.Security.RegistrationEmailSuffixAllowlist)
	settings.Security.OAuthProviders = cloneStringSlice(settings.Security.OAuthProviders)
	settings.Security.OAuthProviderConfigs = cloneOAuthProviderConfigs(settings.Security.OAuthProviderConfigs)
	settings.Gateway.SchedulerStrategyRolloutModels = cloneStringSlice(settings.Gateway.SchedulerStrategyRolloutModels)
	settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes = cloneStringSlice(settings.Gateway.SchedulerStrategyRolloutAPIKeyHashes)
	settings.Gateway.PassthroughHeaderAllowlist = cloneStringSlice(settings.Gateway.PassthroughHeaderAllowlist)
	settings.Payment.Providers = cloneStringSlice(settings.Payment.Providers)
	settings.Email.Templates = cloneStringMap(settings.Email.Templates)
	settings.Email.BalanceLowNotifyEnabled = cloneBoolPtr(settings.Email.BalanceLowNotifyEnabled)
	settings.Email.SubscriptionExpiryNotifyEnabled = cloneBoolPtr(settings.Email.SubscriptionExpiryNotifyEnabled)
	settings.Email.AccountQuotaNotifyEnabled = cloneBoolPtr(settings.Email.AccountQuotaNotifyEnabled)
	settings.Backup.LastBackupAt = cloneTimePtr(settings.Backup.LastBackupAt)
	settings.Copilot.Models = cloneStringSlice(settings.Copilot.Models)
	return settings
}

func cloneAnyMapSlice(values []map[string]any) []map[string]any {
	if values == nil {
		return nil
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, cloneAnyMap(value))
	}
	return out
}

func cloneOAuthProviderConfigs(values []admincontrol.OAuthProviderConfig) []admincontrol.OAuthProviderConfig {
	if values == nil {
		return nil
	}
	out := make([]admincontrol.OAuthProviderConfig, 0, len(values))
	for _, value := range values {
		value.Scopes = cloneStringSlice(value.Scopes)
		out = append(out, value)
	}
	return out
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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

func validPositiveDecimal(value string) bool {
	rat, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	return ok && rat.Sign() > 0
}

func validRedeemCodeValue(codeType admincontrol.RedeemCodeType, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	switch codeType {
	case admincontrol.RedeemCodeTypeBalance:
		amount, ok := new(big.Rat).SetString(value)
		return ok && amount.Sign() > 0
	case admincontrol.RedeemCodeTypeSubscription:
		planID, err := strconv.Atoi(value)
		return err == nil && planID > 0
	default:
		return false
	}
}

func validPercentDecimal(value string) bool {
	ratio, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok {
		return false
	}
	return ratio.Sign() > 0 && ratio.Cmp(big.NewRat(1, 1)) <= 0
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
