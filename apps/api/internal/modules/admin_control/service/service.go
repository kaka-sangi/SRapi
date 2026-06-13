package service

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"time"

	admincontrol "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/ttlcache"
)

const (
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
