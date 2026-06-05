package glob

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"", "anything", true},
		{"*", "anything", true},
		{"gpt-4o", "gpt-4o", true},
		{"gpt-4o", "GPT-4O", true},
		{"gpt-4o", "gpt-4o-mini", false},
		{"gpt-4*", "gpt-4o-mini", true},
		{"gpt-4*", "x-gpt-4o", false},
		{"*-preview", "o1-preview", true},
		{"*-preview", "o1-preview-2", false},
		{"*vision*", "gpt-4-vision-preview", true},
		{"*vision*", "gpt-4o", false},
		{"a*b*c", "axbyc", true},
		{"a*b*c", "axbyz", false},
		{"  gpt-4o  ", " gpt-4o ", true},
	}
	for _, tc := range cases {
		if got := Match(tc.pattern, tc.value); got != tc.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tc.pattern, tc.value, got, tc.want)
		}
	}
}

func TestMatchAny(t *testing.T) {
	patterns := []string{"  ", "claude-*", "*-preview"}
	if !MatchAny(patterns, "claude-3-opus") {
		t.Errorf("expected claude-3-opus to match")
	}
	if !MatchAny(patterns, "o1-preview") {
		t.Errorf("expected o1-preview to match")
	}
	if MatchAny(patterns, "gpt-4o") {
		t.Errorf("expected gpt-4o not to match")
	}
	if MatchAny(nil, "gpt-4o") {
		t.Errorf("expected empty patterns not to match")
	}
	if MatchAny([]string{"   ", ""}, "gpt-4o") {
		t.Errorf("expected blank-only patterns not to match")
	}
}
