package memory

import (
	"context"
	"encoding/json"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

type Store struct {
	mu                     sync.Mutex
	values                 map[string]map[string]any
	systemLogs             []admincontrol.OpsSystemLog
	nextLogID              int
	reads                  map[int]map[int]admincontrol.AnnouncementRead
	nextReadID             int
	users                  userscontract.Store
	billing                billingcontract.Store
	subs                   subscriptioncontract.Store
	redemptions            map[int]map[int]admincontrol.RedeemCodeRedemption
	nextRedemptionID       int
	promoApplications      map[int]admincontrol.PromoCodeApplication
	nextPromoApplicationID int
}

func New() *Store {
	return NewWithFulfillment(nil, nil, nil)
}

func NewWithFulfillment(users userscontract.Store, billing billingcontract.Store, subs subscriptioncontract.Store) *Store {
	return &Store{
		values:                 map[string]map[string]any{},
		nextLogID:              1,
		reads:                  map[int]map[int]admincontrol.AnnouncementRead{},
		nextReadID:             1,
		users:                  users,
		billing:                billing,
		subs:                   subs,
		redemptions:            map[int]map[int]admincontrol.RedeemCodeRedemption{},
		nextRedemptionID:       1,
		promoApplications:      map[int]admincontrol.PromoCodeApplication{},
		nextPromoApplicationID: 1,
	}
}

func (s *Store) Get(_ context.Context, key string) (map[string]any, bool, error) {
	if key == "" {
		return nil, false, admincontrol.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return nil, false, nil
	}
	return cloneMap(value), true, nil
}

func (s *Store) Set(_ context.Context, key string, value map[string]any, _ *int) error {
	if key == "" {
		return admincontrol.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = cloneMap(value)
	return nil
}

func (s *Store) ListAnnouncementReads(_ context.Context, userID int, announcementIDs []int) ([]admincontrol.AnnouncementRead, error) {
	if userID <= 0 {
		return nil, admincontrol.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	byAnnouncement := s.reads[userID]
	if len(byAnnouncement) == 0 {
		return []admincontrol.AnnouncementRead{}, nil
	}
	if len(announcementIDs) == 0 {
		items := make([]admincontrol.AnnouncementRead, 0, len(byAnnouncement))
		for _, item := range byAnnouncement {
			items = append(items, item)
		}
		return items, nil
	}
	items := make([]admincontrol.AnnouncementRead, 0, len(announcementIDs))
	seen := map[int]bool{}
	for _, id := range announcementIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		if item, ok := byAnnouncement[id]; ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *Store) MarkAnnouncementRead(_ context.Context, userID int, announcementID int, at time.Time) (admincontrol.AnnouncementRead, error) {
	if userID <= 0 || announcementID <= 0 {
		return admincontrol.AnnouncementRead{}, admincontrol.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := at.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if _, ok := s.reads[userID]; !ok {
		s.reads[userID] = map[int]admincontrol.AnnouncementRead{}
	}
	if existing, ok := s.reads[userID][announcementID]; ok {
		existing.ReadAt = now
		existing.UpdatedAt = now
		s.reads[userID][announcementID] = existing
		return existing, nil
	}
	item := admincontrol.AnnouncementRead{
		ID:             s.nextReadID,
		UserID:         userID,
		AnnouncementID: announcementID,
		ReadAt:         now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.nextReadID++
	s.reads[userID][announcementID] = item
	return item, nil
}

func (s *Store) RedeemCode(ctx context.Context, input admincontrol.RedeemCodeRedemptionInput) (admincontrol.RedeemCodeRedemptionResult, error) {
	if input.UserID <= 0 || strings.TrimSpace(input.Code) == "" {
		return admincontrol.RedeemCodeRedemptionResult{}, admincontrol.ErrInvalidInput
	}
	now := input.RedeemedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	codeValue := normalizeCode(input.Code)

	s.mu.Lock()
	defer s.mu.Unlock()

	collection, err := s.redeemCodeCollection()
	if err != nil {
		return admincontrol.RedeemCodeRedemptionResult{}, err
	}
	for idx, item := range collection.Items {
		item = redeemCodeWithDerivedStatus(item, now)
		if item.Code != codeValue {
			collection.Items[idx] = item
			continue
		}
		if existing, ok := s.redemption(input.UserID, item.ID); ok {
			return admincontrol.RedeemCodeRedemptionResult{
				Redemption:      existing,
				RedeemCode:      item,
				AlreadyRedeemed: true,
			}, nil
		}
		if item.Status != admincontrol.RedeemCodeStatusActive || item.RedeemedCount >= item.MaxRedemptions {
			collection.Items[idx] = item
			_ = s.saveRedeemCodeCollection(collection)
			return admincontrol.RedeemCodeRedemptionResult{}, admincontrol.ErrConflict
		}

		redemption, err := s.fulfillRedeemCode(ctx, input.UserID, item, now)
		if err != nil {
			return admincontrol.RedeemCodeRedemptionResult{}, err
		}
		item.RedeemedCount++
		item.UpdatedAt = now
		if item.RedeemedCount >= item.MaxRedemptions {
			item.Status = admincontrol.RedeemCodeStatusRedeemed
		}
		collection.Items[idx] = item
		if err := s.saveRedeemCodeCollection(collection); err != nil {
			return admincontrol.RedeemCodeRedemptionResult{}, err
		}
		if _, ok := s.redemptions[input.UserID]; !ok {
			s.redemptions[input.UserID] = map[int]admincontrol.RedeemCodeRedemption{}
		}
		s.redemptions[input.UserID][item.ID] = redemption
		return admincontrol.RedeemCodeRedemptionResult{
			Redemption: redemption,
			RedeemCode: item,
		}, nil
	}
	_ = s.saveRedeemCodeCollection(collection)
	return admincontrol.RedeemCodeRedemptionResult{}, admincontrol.ErrNotFound
}

func (s *Store) PreviewPromoCode(_ context.Context, input admincontrol.PromoCodePreviewInput) (admincontrol.PromoCodeApplication, error) {
	if input.UserID <= 0 || strings.TrimSpace(input.Code) == "" {
		return admincontrol.PromoCodeApplication{}, admincontrol.ErrInvalidInput
	}
	now := input.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	collection, err := s.promoCodeCollection()
	if err != nil {
		return admincontrol.PromoCodeApplication{}, err
	}
	item, _, ok := findPromoCode(collection.Items, input.Code, now)
	if !ok {
		return admincontrol.PromoCodeApplication{}, admincontrol.ErrNotFound
	}
	return previewPromoCode(item, input.UserID, input.Amount, input.Currency, now)
}

func (s *Store) FinalizePromoCode(_ context.Context, input admincontrol.PromoCodeFinalizeInput) (admincontrol.PromoCodeApplication, error) {
	if input.UserID <= 0 || strings.TrimSpace(input.Code) == "" || input.PaymentOrderID <= 0 || strings.TrimSpace(input.OrderNo) == "" {
		return admincontrol.PromoCodeApplication{}, admincontrol.ErrInvalidInput
	}
	now := input.AppliedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.promoApplications[input.PaymentOrderID]; ok {
		return existing, nil
	}
	collection, err := s.promoCodeCollection()
	if err != nil {
		return admincontrol.PromoCodeApplication{}, err
	}
	item, idx, ok := findPromoCode(collection.Items, input.Code, now)
	if !ok {
		return admincontrol.PromoCodeApplication{}, admincontrol.ErrNotFound
	}
	application, err := previewPromoCode(item, input.UserID, input.OriginalAmount, input.Currency, now)
	if err != nil {
		return admincontrol.PromoCodeApplication{}, err
	}
	if normalizeCurrency(application.Currency) != normalizeCurrency(input.Currency) || application.FinalAmount != formatInputMoney(input.FinalAmount) {
		return admincontrol.PromoCodeApplication{}, admincontrol.ErrInvalidInput
	}
	application.ID = s.nextPromoApplicationID
	application.PaymentOrderID = input.PaymentOrderID
	application.OrderNo = strings.TrimSpace(input.OrderNo)
	application.CreatedAt = now
	application.UpdatedAt = now
	s.nextPromoApplicationID++
	item.UsedCount++
	item.UpdatedAt = now
	if item.UsedCount >= item.MaxUses {
		item.Status = admincontrol.PromoCodeStatusExpired
	}
	collection.Items[idx] = item
	if err := s.savePromoCodeCollection(collection); err != nil {
		return admincontrol.PromoCodeApplication{}, err
	}
	s.promoApplications[input.PaymentOrderID] = application
	return application, nil
}

func (s *Store) CreateSystemLog(_ context.Context, input admincontrol.OpsSystemLog) (admincontrol.OpsSystemLog, error) {
	if strings.TrimSpace(input.Source) == "" || strings.TrimSpace(input.Message) == "" || !input.Level.Valid() {
		return admincontrol.OpsSystemLog{}, admincontrol.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.ID <= 0 {
		input.ID = s.nextLogID
		s.nextLogID++
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	}
	input.Metadata = cloneMap(input.Metadata)
	s.systemLogs = append(s.systemLogs, input)
	return cloneSystemLog(input), nil
}

func (s *Store) ListSystemLogs(_ context.Context, opts admincontrol.SystemLogListOptions) (admincontrol.SystemLogList, error) {
	if opts.Level != "" && !opts.Level.Valid() {
		return admincontrol.SystemLogList{}, admincontrol.ErrInvalidInput
	}
	if opts.Start != nil && opts.End != nil && opts.Start.After(*opts.End) {
		return admincontrol.SystemLogList{}, admincontrol.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]admincontrol.OpsSystemLog, 0, len(s.systemLogs))
	filter := systemLogCleanupFilterFromListOptions(opts)
	for _, item := range s.systemLogs {
		if systemLogMatches(item, filter) {
			items = append(items, cloneSystemLog(item))
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return admincontrol.SystemLogList{Items: pageSystemLogs(items, opts.Page, opts.PageSize), Total: len(items)}, nil
}

func (s *Store) CleanupSystemLogs(_ context.Context, filter admincontrol.SystemLogCleanupFilter) (admincontrol.SystemLogCleanupResult, error) {
	normalized, err := normalizeSystemLogCleanupFilter(filter)
	if err != nil {
		return admincontrol.SystemLogCleanupResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.systemLogs[:0]
	var matched, deleted int
	for _, item := range s.systemLogs {
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
	if !normalized.DryRun {
		s.systemLogs = kept
	}
	return admincontrol.SystemLogCleanupResult{
		Matched:   matched,
		Deleted:   deleted,
		DryRun:    normalized.DryRun,
		MaxDelete: normalized.MaxDelete,
		Limited:   matched > deleted && !normalized.DryRun,
	}, nil
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func cloneSystemLog(value admincontrol.OpsSystemLog) admincontrol.OpsSystemLog {
	value.Metadata = cloneMap(value.Metadata)
	return value
}

func systemLogCleanupFilterFromListOptions(opts admincontrol.SystemLogListOptions) admincontrol.SystemLogCleanupFilter {
	return admincontrol.SystemLogCleanupFilter{
		Level:  opts.Level,
		Source: strings.TrimSpace(opts.Source),
		Query:  strings.TrimSpace(opts.Query),
		Start:  opts.Start,
		End:    opts.End,
	}
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
		filter.MaxDelete = 1000
	}
	if filter.MaxDelete < 0 || filter.MaxDelete > 10000 {
		return admincontrol.SystemLogCleanupFilter{}, admincontrol.ErrInvalidInput
	}
	return filter, nil
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

func pageSystemLogs(items []admincontrol.OpsSystemLog, page, pageSize int) []admincontrol.OpsSystemLog {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []admincontrol.OpsSystemLog{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

const settingsKeyRedeemCodes = "admin_control.redeem_codes"

type redeemCodeCollection struct {
	NextID int                       `json:"next_id"`
	Items  []admincontrol.RedeemCode `json:"items"`
}

const settingsKeyPromoCodes = "admin_control.promo_codes"

type promoCodeCollection struct {
	NextID int                      `json:"next_id"`
	Items  []admincontrol.PromoCode `json:"items"`
}

func (s *Store) redeemCodeCollection() (redeemCodeCollection, error) {
	raw, ok := s.values[settingsKeyRedeemCodes]
	if !ok {
		return redeemCodeCollection{}, nil
	}
	var collection redeemCodeCollection
	encoded, err := json.Marshal(raw)
	if err != nil {
		return redeemCodeCollection{}, err
	}
	if err := json.Unmarshal(encoded, &collection); err != nil {
		return redeemCodeCollection{}, err
	}
	return collection, nil
}

func (s *Store) saveRedeemCodeCollection(collection redeemCodeCollection) error {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		return err
	}
	s.values[settingsKeyRedeemCodes] = raw
	return nil
}

func (s *Store) promoCodeCollection() (promoCodeCollection, error) {
	raw, ok := s.values[settingsKeyPromoCodes]
	if !ok {
		return promoCodeCollection{}, nil
	}
	var collection promoCodeCollection
	encoded, err := json.Marshal(raw)
	if err != nil {
		return promoCodeCollection{}, err
	}
	if err := json.Unmarshal(encoded, &collection); err != nil {
		return promoCodeCollection{}, err
	}
	return collection, nil
}

func (s *Store) savePromoCodeCollection(collection promoCodeCollection) error {
	encoded, err := json.Marshal(collection)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		return err
	}
	s.values[settingsKeyPromoCodes] = raw
	return nil
}

func (s *Store) redemption(userID int, redeemCodeID int) (admincontrol.RedeemCodeRedemption, bool) {
	byCode := s.redemptions[userID]
	if len(byCode) == 0 {
		return admincontrol.RedeemCodeRedemption{}, false
	}
	item, ok := byCode[redeemCodeID]
	return item, ok
}

func (s *Store) fulfillRedeemCode(ctx context.Context, userID int, code admincontrol.RedeemCode, now time.Time) (admincontrol.RedeemCodeRedemption, error) {
	switch code.Type {
	case admincontrol.RedeemCodeTypeBalance:
		return s.fulfillBalanceRedeemCode(ctx, userID, code, now)
	case admincontrol.RedeemCodeTypeSubscription:
		return s.fulfillSubscriptionRedeemCode(ctx, userID, code, now)
	default:
		return admincontrol.RedeemCodeRedemption{}, admincontrol.ErrInvalidInput
	}
}

func (s *Store) fulfillBalanceRedeemCode(ctx context.Context, userID int, code admincontrol.RedeemCode, now time.Time) (admincontrol.RedeemCodeRedemption, error) {
	if s.users == nil {
		return admincontrol.RedeemCodeRedemption{}, admincontrol.ErrInvalidInput
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return admincontrol.RedeemCodeRedemption{}, err
	}
	if user.Status != userscontract.StatusActive {
		return admincontrol.RedeemCodeRedemption{}, admincontrol.ErrConflict
	}
	amount, ok := decimalRat(code.Value)
	if !ok || amount.Sign() <= 0 {
		return admincontrol.RedeemCodeRedemption{}, admincontrol.ErrInvalidInput
	}
	before, ok := decimalRat(user.Balance)
	if !ok {
		return admincontrol.RedeemCodeRedemption{}, admincontrol.ErrInvalidInput
	}
	after := new(big.Rat).Add(before, amount)
	balanceBefore := formatRatFixed(before, 8)
	balanceAfter := formatRatFixed(after, 8)
	currency := normalizeCurrency(code.Currency)
	_, err = s.users.Update(ctx, userID, userscontract.UpdateStoredUser{
		Balance:  &balanceAfter,
		Currency: &currency,
	})
	if err != nil {
		return admincontrol.RedeemCodeRedemption{}, err
	}
	var ledgerID *int
	if s.billing != nil {
		ledger, err := s.billing.Create(ctx, billingcontract.LedgerEntry{
			UserID:        userID,
			Type:          billingcontract.LedgerTypeRedeemCodeCredit,
			Amount:        formatRatFixed(amount, 8),
			Currency:      currency,
			BalanceBefore: balanceBefore,
			BalanceAfter:  balanceAfter,
			ReferenceType: "redeem_code",
			ReferenceID:   strconv.Itoa(code.ID),
			Metadata: map[string]any{
				"redeem_code_id": code.ID,
			},
			CreatedAt: now,
		})
		if err != nil {
			return admincontrol.RedeemCodeRedemption{}, err
		}
		ledgerID = &ledger.ID
	}
	return s.newRedemption(userID, code, formatRatFixed(amount, 8), currency, balanceBefore, balanceAfter, ledgerID, nil, now), nil
}

func (s *Store) fulfillSubscriptionRedeemCode(ctx context.Context, userID int, code admincontrol.RedeemCode, now time.Time) (admincontrol.RedeemCodeRedemption, error) {
	if s.users == nil || s.subs == nil {
		return admincontrol.RedeemCodeRedemption{}, admincontrol.ErrInvalidInput
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return admincontrol.RedeemCodeRedemption{}, err
	}
	if user.Status != userscontract.StatusActive {
		return admincontrol.RedeemCodeRedemption{}, admincontrol.ErrConflict
	}
	planID, err := strconv.Atoi(strings.TrimSpace(code.Value))
	if err != nil || planID <= 0 {
		return admincontrol.RedeemCodeRedemption{}, admincontrol.ErrInvalidInput
	}
	plan, err := s.subs.FindPlanByID(ctx, planID)
	if err != nil {
		return admincontrol.RedeemCodeRedemption{}, err
	}
	if plan.Status != subscriptioncontract.PlanStatusActive || plan.ValidityDays <= 0 {
		return admincontrol.RedeemCodeRedemption{}, admincontrol.ErrConflict
	}
	expiresAt := now.AddDate(0, 0, plan.ValidityDays)
	subscription, err := s.subs.CreateUserSubscription(ctx, subscriptioncontract.CreateStoredSubscription{
		UserID:               userID,
		PlanID:               plan.ID,
		Status:               subscriptioncontract.SubscriptionStatusActive,
		StartsAt:             now,
		ExpiresAt:            expiresAt,
		EntitlementsSnapshot: cloneMap(plan.Entitlements),
		SourceType:           "redeem_code",
		SourceID:             strconv.Itoa(code.ID),
	})
	if err != nil {
		return admincontrol.RedeemCodeRedemption{}, err
	}
	return s.newRedemption(userID, code, "0.00000000", normalizeCurrency(plan.Currency), user.Balance, user.Balance, nil, &subscription.ID, now), nil
}

func (s *Store) newRedemption(userID int, code admincontrol.RedeemCode, amount string, currency string, balanceBefore string, balanceAfter string, billingLedgerID *int, userSubscriptionID *int, now time.Time) admincontrol.RedeemCodeRedemption {
	item := admincontrol.RedeemCodeRedemption{
		ID:                 s.nextRedemptionID,
		UserID:             userID,
		RedeemCodeID:       code.ID,
		Type:               code.Type,
		Amount:             amount,
		Currency:           normalizeCurrency(currency),
		BalanceBefore:      balanceBefore,
		BalanceAfter:       balanceAfter,
		BillingLedgerID:    cloneInt(billingLedgerID),
		UserSubscriptionID: cloneInt(userSubscriptionID),
		RedeemedAt:         now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.nextRedemptionID++
	return item
}

func redeemCodeWithDerivedStatus(item admincontrol.RedeemCode, now time.Time) admincontrol.RedeemCode {
	if item.Status == admincontrol.RedeemCodeStatusActive && item.ExpiresAt != nil && item.ExpiresAt.Before(now) {
		item.Status = admincontrol.RedeemCodeStatusExpired
	}
	if item.Status == admincontrol.RedeemCodeStatusActive && item.MaxRedemptions > 0 && item.RedeemedCount >= item.MaxRedemptions {
		item.Status = admincontrol.RedeemCodeStatusRedeemed
	}
	return item
}

func findPromoCode(items []admincontrol.PromoCode, code string, now time.Time) (admincontrol.PromoCode, int, bool) {
	code = normalizeCode(code)
	for idx, item := range items {
		item = promoCodeWithDerivedStatus(item, now)
		if normalizeCode(item.Code) == code {
			return item, idx, true
		}
	}
	return admincontrol.PromoCode{}, -1, false
}

func promoCodeWithDerivedStatus(item admincontrol.PromoCode, now time.Time) admincontrol.PromoCode {
	if item.Status == admincontrol.PromoCodeStatusActive && item.ExpiresAt != nil && item.ExpiresAt.Before(now) {
		item.Status = admincontrol.PromoCodeStatusExpired
	}
	if item.Status == admincontrol.PromoCodeStatusActive && item.MaxUses > 0 && item.UsedCount >= item.MaxUses {
		item.Status = admincontrol.PromoCodeStatusExpired
	}
	return item
}

func previewPromoCode(item admincontrol.PromoCode, userID int, amount string, currency string, now time.Time) (admincontrol.PromoCodeApplication, error) {
	item = promoCodeWithDerivedStatus(item, now)
	if item.Status != admincontrol.PromoCodeStatusActive || item.MaxUses <= 0 || item.UsedCount >= item.MaxUses {
		return admincontrol.PromoCodeApplication{}, admincontrol.ErrConflict
	}
	if item.StartsAt != nil && item.StartsAt.After(now) {
		return admincontrol.PromoCodeApplication{}, admincontrol.ErrConflict
	}
	inputAmount, ok := decimalRat(amount)
	if !ok || inputAmount.Sign() <= 0 {
		return admincontrol.PromoCodeApplication{}, admincontrol.ErrInvalidInput
	}
	normalizedCurrency := normalizeCurrency(currency)
	if item.DiscountType == admincontrol.PromoDiscountTypeAmount && normalizeCurrency(item.Currency) != normalizedCurrency {
		return admincontrol.PromoCodeApplication{}, admincontrol.ErrConflict
	}
	discount, err := promoDiscountAmount(item, inputAmount)
	if err != nil {
		return admincontrol.PromoCodeApplication{}, err
	}
	if discount.Sign() <= 0 || discount.Cmp(inputAmount) >= 0 {
		return admincontrol.PromoCodeApplication{}, admincontrol.ErrInvalidInput
	}
	finalAmount := new(big.Rat).Sub(inputAmount, discount)
	return admincontrol.PromoCodeApplication{
		UserID:         userID,
		PromoCodeID:    item.ID,
		OriginalAmount: formatRatFixed(inputAmount, 8),
		DiscountAmount: formatRatFixed(discount, 8),
		FinalAmount:    formatRatFixed(finalAmount, 8),
		Currency:       normalizedCurrency,
		DiscountType:   item.DiscountType,
		AppliedAt:      now,
	}, nil
}

func promoDiscountAmount(item admincontrol.PromoCode, amount *big.Rat) (*big.Rat, error) {
	value, ok := decimalRat(item.DiscountValue)
	if !ok || value.Sign() <= 0 {
		return nil, admincontrol.ErrInvalidInput
	}
	switch item.DiscountType {
	case admincontrol.PromoDiscountTypeAmount:
		return value, nil
	case admincontrol.PromoDiscountTypePercent:
		if value.Cmp(big.NewRat(1, 1)) > 0 {
			return nil, admincontrol.ErrInvalidInput
		}
		return new(big.Rat).Mul(amount, value), nil
	default:
		return nil, admincontrol.ErrInvalidInput
	}
}

func formatInputMoney(value string) string {
	rat, ok := decimalRat(value)
	if !ok {
		return ""
	}
	return formatRatFixed(rat, 8)
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

func decimalRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "eE") {
		return nil, false
	}
	rat, ok := new(big.Rat).SetString(value)
	return rat, ok
}

func formatRatFixed(value *big.Rat, places int) string {
	if value == nil {
		value = new(big.Rat)
	}
	return value.FloatString(places)
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
