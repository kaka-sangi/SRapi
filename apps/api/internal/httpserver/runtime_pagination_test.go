package httpserver

import (
	"net/http/httptest"
	"testing"
)

// TestPaginateRealWindowing proves the paginate helper does real paging: it
// slices to the requested page/page_size and reports honest total/hasNext, while
// preserving the full list (page 1) when no page_size is sent.
func TestPaginateRealWindowing(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7}

	// No page_size: full list, honest metadata, no next.
	got, pg := paginate(httptest.NewRequest("GET", "/x", nil), items)
	if len(got) != 7 || pg.Total != 7 || pg.HasNext {
		t.Fatalf("no page_size should return all 7 with hasNext=false, got len=%d %+v", len(got), pg)
	}

	// page_size=3, page 1: first window, hasNext true.
	got, pg = paginate(httptest.NewRequest("GET", "/x?page_size=3", nil), items)
	if len(got) != 3 || got[0] != 1 || pg.Total != 7 || pg.PageSize != 3 || pg.Page != 1 || !pg.HasNext {
		t.Fatalf("page 1 of 3 wrong: len=%d first=%v %+v", len(got), got, pg)
	}

	// page 2: middle window.
	got, pg = paginate(httptest.NewRequest("GET", "/x?page_size=3&page=2", nil), items)
	if len(got) != 3 || got[0] != 4 || !pg.HasNext {
		t.Fatalf("page 2 of 3 wrong: %v %+v", got, pg)
	}

	// page 3: last (partial) window, no next.
	got, pg = paginate(httptest.NewRequest("GET", "/x?page_size=3&page=3", nil), items)
	if len(got) != 1 || got[0] != 7 || pg.HasNext {
		t.Fatalf("last page wrong: %v %+v", got, pg)
	}

	// page beyond the end: empty, no next.
	got, pg = paginate(httptest.NewRequest("GET", "/x?page_size=3&page=99", nil), items)
	if len(got) != 0 || pg.HasNext {
		t.Fatalf("past-end page should be empty, got %v %+v", got, pg)
	}
}
