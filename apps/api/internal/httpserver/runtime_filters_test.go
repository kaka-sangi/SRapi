package httpserver

import (
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

// TestFilterUsageLogsAccountIDAndSuccess covers two query params the iter-67
// (account filter) and iter-69 (success/error filter) frontend changes
// depend on. These are server-side query params the panels pass through;
// regressing here would silently drop the filters.
func TestFilterUsageLogsAccountIDAndSuccess(t *testing.T) {
	acc7 := 7
	acc8 := 8
	prov := 11
	items := []usagecontract.UsageLog{
		{ID: 1, UserID: 1, AccountID: &acc7, ProviderID: &prov, Success: true, CreatedAt: time.Now().UTC()},
		{ID: 2, UserID: 1, AccountID: &acc8, ProviderID: &prov, Success: true, CreatedAt: time.Now().UTC()},
		{ID: 3, UserID: 2, AccountID: &acc7, ProviderID: &prov, Success: false, CreatedAt: time.Now().UTC()},
		{ID: 4, UserID: 3, AccountID: nil, Success: true, CreatedAt: time.Now().UTC()},
	}

	// account_id=7 → rows 1 and 3.
	r := httptest.NewRequest("GET", "/?account_id=7", nil)
	got := filterUsageLogs(items, r)
	if len(got) != 2 {
		t.Fatalf("account_id=7: want 2 rows, got %d (%+v)", len(got), idsOf(got))
	}
	if got[0].ID != 1 || got[1].ID != 3 {
		t.Fatalf("account_id=7: want [1 3], got %v", idsOf(got))
	}

	// account_id=7 AND success=false → only row 3.
	r = httptest.NewRequest("GET", "/?account_id=7&success=false", nil)
	got = filterUsageLogs(items, r)
	if len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("account_id=7&success=false: want [3], got %v", idsOf(got))
	}

	// success=true alone keeps a, b, d (excludes c).
	r = httptest.NewRequest("GET", "/?success=true", nil)
	got = filterUsageLogs(items, r)
	if len(got) != 3 {
		t.Fatalf("success=true: want 3 rows, got %d (%v)", len(got), idsOf(got))
	}

	// Unknown account_id returns empty — important guard, otherwise an unknown
	// id would silently match everything (e.g. via empty-string equality).
	r = httptest.NewRequest("GET", "/?account_id=999", nil)
	got = filterUsageLogs(items, r)
	if len(got) != 0 {
		t.Fatalf("account_id=999: want 0 rows, got %d", len(got))
	}
}

func idsOf(items []usagecontract.UsageLog) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, strconv.Itoa(it.ID))
	}
	return out
}
