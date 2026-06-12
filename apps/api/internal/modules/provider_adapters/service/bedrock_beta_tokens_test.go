package service

import (
	"reflect"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// TestBedrockAnthropicBetaTokenFiltering proves only Bedrock-supported beta
// tokens survive: unsupported tokens are dropped (they would 400 the whole
// request upstream), generic tokens are transformed to their Bedrock
// equivalent, and tool-search auto-pairs its examples companion.
func TestBedrockAnthropicBetaTokenFiltering(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "unsupported tokens dropped",
			raw:  "output-128k-2025-02-19, files-api-2025-04-14, context-1m-2025-08-07",
			want: []string{"context-1m-2025-08-07"},
		},
		{
			name: "all unsupported yields none",
			raw:  "output-128k-2025-02-19, structured-outputs-2024-12-12",
			want: []string{},
		},
		{
			name: "transform advanced-tool-use and auto-pair examples",
			raw:  "advanced-tool-use-2025-11-20",
			want: []string{"tool-search-tool-2025-10-19", "tool-examples-2025-10-29"},
		},
		{
			name: "dedup survives filtering",
			raw:  "context-management-2025-06-27,context-management-2025-06-27",
			want: []string{"context-management-2025-06-27"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := contract.ConversationRequest{Credential: map[string]any{"anthropic_beta": tc.raw}}
			got := bedrockAnthropicBetaTokens(req)
			if got == nil {
				got = []string{}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("tokens for %q: got %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
