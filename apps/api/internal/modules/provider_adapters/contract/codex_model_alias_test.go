package contract

import "testing"

func TestNormalizeCodexUpstreamModelName(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "exact latest", model: "gpt-5.5", want: "gpt-5.5"},
		{name: "provider prefix", model: "openai/gpt5.4mini", want: "gpt-5.4-mini"},
		{name: "compact suffix", model: "gpt-5.4-openai-compact", want: "gpt-5.4"},
		{name: "removed model fallback", model: "gpt-5.1", want: "gpt-5.4"},
		{name: "spark effort suffix", model: "gpt-5.3-codex-spark-xhigh", want: "gpt-5.3-codex-spark"},
		{name: "codex auto review", model: "codex-auto-review", want: "codex-auto-review"},
		{name: "unknown custom", model: " custom-codex-model ", want: "custom-codex-model"},
		{name: "non codex custom", model: "custom-model", want: "custom-model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeCodexUpstreamModelName(tt.model); got != tt.want {
				t.Fatalf("NormalizeCodexUpstreamModelName(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}
