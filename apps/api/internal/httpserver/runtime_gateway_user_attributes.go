package httpserver

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/platform/localcache"
)

// userAttributeOverrides holds gateway-relevant overrides extracted from a
// user's custom attributes. Populated once per request via
// resolveUserAttributeOverrides and applied at admission + scheduling time.
type userAttributeOverrides struct {
	GroupOverride  string
	RPMOverride    int
	CostMultiplier float64
	Organization   string
}

const userAttributeCacheTTL = 30 * time.Second

// resolveUserAttributeOverrides loads the user's custom attributes (cached)
// and extracts recognized override keys. Unknown keys are ignored.
func (rt *runtimeState) resolveUserAttributeOverrides(ctx context.Context, userID int) userAttributeOverrides {
	if userID <= 0 || rt.userAttributes == nil {
		return userAttributeOverrides{}
	}
	cacheKey := "uao:" + strconv.Itoa(userID)
	if rt.userAttributeCache != nil {
		if cached, ok := rt.userAttributeCache.Get(cacheKey); ok {
			return cached
		}
	}
	values, err := rt.userAttributesStore.ListValuesByUser(ctx, userID)
	if err != nil || len(values) == 0 {
		result := userAttributeOverrides{}
		if rt.userAttributeCache != nil {
			rt.userAttributeCache.Set(cacheKey, result)
		}
		return result
	}
	defs, err := rt.userAttributesStore.ListDefinitions(ctx)
	if err != nil {
		return userAttributeOverrides{}
	}
	defMap := make(map[int]string, len(defs))
	for _, d := range defs {
		if d.Enabled {
			defMap[d.ID] = d.Key
		}
	}
	var result userAttributeOverrides
	for _, v := range values {
		key := defMap[v.DefinitionID]
		val := strings.TrimSpace(v.Value)
		if val == "" {
			continue
		}
		switch key {
		case "group_override":
			result.GroupOverride = val
		case "rpm_override":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				result.RPMOverride = n
			}
		case "cost_multiplier":
			if f, err := strconv.ParseFloat(val, 64); err == nil && f > 0 {
				result.CostMultiplier = f
			}
		case "organization":
			result.Organization = val
		}
	}
	if rt.userAttributeCache != nil {
		rt.userAttributeCache.Set(cacheKey, result)
	}
	return result
}

// resolveAccountGroupByName looks up an account group by name and returns
// its ID(s) for use as AccountGroupScope. Returns nil if not found.
func (rt *runtimeState) resolveAccountGroupByName(ctx context.Context, name string) []int {
	if rt.accounts == nil || name == "" {
		return nil
	}
	groups, err := rt.accounts.ListGroups(ctx)
	if err != nil {
		return nil
	}
	for _, g := range groups {
		if strings.EqualFold(strings.TrimSpace(g.Name), name) {
			return []int{g.ID}
		}
	}
	return nil
}

func newUserAttributeCache() *localcache.Cache[userAttributeOverrides] {
	return localcache.New[userAttributeOverrides](localcache.Config{
		MaxEntries: 4096,
		DefaultTTL: userAttributeCacheTTL,
	})
}
