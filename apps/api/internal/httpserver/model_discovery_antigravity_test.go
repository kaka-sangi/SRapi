package httpserver

import (
	"net/http"
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

// TestShouldFallbackToNextAntigravityURL locks in the ported sub2api fallback
// policy: connection/transport failures (status 0) plus 429/408/404/5xx advance
// to the next base URL, while 2xx/3xx and other 4xx are terminal.
func TestShouldFallbackToNextAntigravityURL(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"transport/connection error", 0, true},
		{"429 too many requests", http.StatusTooManyRequests, true},
		{"408 request timeout", http.StatusRequestTimeout, true},
		{"404 not found", http.StatusNotFound, true},
		{"500 internal", http.StatusInternalServerError, true},
		{"503 unavailable", http.StatusServiceUnavailable, true},
		{"599 upper 5xx", 599, true},
		{"200 ok", http.StatusOK, false},
		{"301 redirect", http.StatusMovedPermanently, false},
		{"400 bad request", http.StatusBadRequest, false},
		{"401 unauthorized", http.StatusUnauthorized, false},
		{"403 forbidden", http.StatusForbidden, false},
		{"499 lower edge non-fallback 4xx", 499, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFallbackToNextAntigravityURL(tc.status); got != tc.want {
				t.Fatalf("shouldFallbackToNextAntigravityURL(%d) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestAntigravityBootstrapCandidateURLs verifies the ordered, deduped fallback
// list: with no operator override it is exactly sub2api's prod -> daily list; a
// configured override is honored first and never duplicates a fixed entry.
func TestAntigravityBootstrapCandidateURLs(t *testing.T) {
	t.Run("default prod then daily", func(t *testing.T) {
		got := antigravityBootstrapCandidateURLs(providercontract.Provider{}, accountcontract.ProviderAccount{})
		want := []string{antigravityProdBaseURL, antigravityDailyBaseURL}
		assertURLs(t, got, want)
	})

	t.Run("override prepended and trailing slash trimmed", func(t *testing.T) {
		account := accountcontract.ProviderAccount{
			Metadata: map[string]any{"antigravity_base_url": "https://custom.example.com/"},
		}
		got := antigravityBootstrapCandidateURLs(providercontract.Provider{}, account)
		want := []string{
			"https://custom.example.com",
			antigravityProdBaseURL,
			antigravityDailyBaseURL,
		}
		assertURLs(t, got, want)
	})

	t.Run("override equal to prod is deduped (still tries daily fallback)", func(t *testing.T) {
		account := accountcontract.ProviderAccount{
			Metadata: map[string]any{"antigravity_base_url": antigravityProdBaseURL},
		}
		got := antigravityBootstrapCandidateURLs(providercontract.Provider{}, account)
		want := []string{antigravityProdBaseURL, antigravityDailyBaseURL}
		assertURLs(t, got, want)
	})
}

func assertURLs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("url count = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("url[%d] = %q, want %q (full got=%v)", i, got[i], want[i], got)
		}
	}
}
