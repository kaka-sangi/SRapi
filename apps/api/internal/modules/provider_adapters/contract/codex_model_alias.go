package contract

import "strings"

var codexUpstreamModelAliases = map[string]string{
	"codex-auto-review":          "codex-auto-review",
	"codex-mini-latest":          "gpt-5.3-codex",
	"gpt-5":                      "gpt-5.4",
	"gpt-5-codex":                "gpt-5.3-codex",
	"gpt-5-mini":                 "gpt-5.4",
	"gpt-5-nano":                 "gpt-5.4",
	"gpt-5.1":                    "gpt-5.4",
	"gpt-5.1-codex":              "gpt-5.3-codex",
	"gpt-5.1-codex-max":          "gpt-5.3-codex",
	"gpt-5.1-codex-mini":         "gpt-5.3-codex",
	"gpt-5.2":                    "gpt-5.2",
	"gpt-5.2-codex":              "gpt-5.2",
	"gpt-5.2-high":               "gpt-5.2",
	"gpt-5.2-low":                "gpt-5.2",
	"gpt-5.2-medium":             "gpt-5.2",
	"gpt-5.2-none":               "gpt-5.2",
	"gpt-5.2-xhigh":              "gpt-5.2",
	"gpt-5.3":                    "gpt-5.3-codex",
	"gpt-5.3-codex":              "gpt-5.3-codex",
	"gpt-5.3-codex-high":         "gpt-5.3-codex",
	"gpt-5.3-codex-low":          "gpt-5.3-codex",
	"gpt-5.3-codex-medium":       "gpt-5.3-codex",
	"gpt-5.3-codex-spark":        "gpt-5.3-codex-spark",
	"gpt-5.3-codex-spark-high":   "gpt-5.3-codex-spark",
	"gpt-5.3-codex-spark-low":    "gpt-5.3-codex-spark",
	"gpt-5.3-codex-spark-medium": "gpt-5.3-codex-spark",
	"gpt-5.3-codex-spark-xhigh":  "gpt-5.3-codex-spark",
	"gpt-5.3-codex-xhigh":        "gpt-5.3-codex",
	"gpt-5.3-high":               "gpt-5.3-codex",
	"gpt-5.3-low":                "gpt-5.3-codex",
	"gpt-5.3-medium":             "gpt-5.3-codex",
	"gpt-5.3-none":               "gpt-5.3-codex",
	"gpt-5.3-xhigh":              "gpt-5.3-codex",
	"gpt-5.4":                    "gpt-5.4",
	"gpt-5.4-chat-latest":        "gpt-5.4",
	"gpt-5.4-high":               "gpt-5.4",
	"gpt-5.4-low":                "gpt-5.4",
	"gpt-5.4-medium":             "gpt-5.4",
	"gpt-5.4-mini":               "gpt-5.4-mini",
	"gpt-5.4-none":               "gpt-5.4",
	"gpt-5.4-xhigh":              "gpt-5.4",
	"gpt-5.5":                    "gpt-5.5",
}

// NormalizeCodexUpstreamModelName canonicalizes known OpenAI/Codex client
// model aliases to the upstream names Codex accepts. Unknown model names are
// left intact so operator-defined custom mappings continue to pass through.
func NormalizeCodexUpstreamModelName(model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return ""
	}
	canonical := canonicalizeCodexModelAliasSpelling(trimmed)
	if canonical == "" {
		return trimmed
	}
	if mapped := codexUpstreamModelAliases[canonical]; mapped != "" {
		return mapped
	}
	if strings.HasSuffix(canonical, "-openai-compact") {
		if mapped := codexUpstreamModelAliases[strings.TrimSuffix(canonical, "-openai-compact")]; mapped != "" {
			return mapped
		}
	}
	for _, prefix := range codexUpstreamVersionModelPrefixes {
		if strings.HasPrefix(canonical, prefix.prefix) {
			return prefix.target
		}
	}
	return trimmed
}

func canonicalizeCodexModelAliasSpelling(model string) string {
	model = strings.ToLower(lastCodexModelSegment(model))
	if model == "" {
		return ""
	}
	normalized := strings.ReplaceAll(model, "_", "-")
	normalized = strings.Join(strings.Fields(normalized), "-")
	for strings.Contains(normalized, "--") {
		normalized = strings.ReplaceAll(normalized, "--", "-")
	}
	if strings.HasPrefix(normalized, "gpt5") {
		normalized = "gpt-5" + strings.TrimPrefix(normalized, "gpt5")
	}
	if !strings.HasPrefix(normalized, "gpt-") && !strings.Contains(normalized, "codex") {
		return ""
	}
	replacements := []struct {
		from string
		to   string
	}{
		{from: "gpt-5.4mini", to: "gpt-5.4-mini"},
		{from: "gpt-5.3-codexspark", to: "gpt-5.3-codex-spark"},
		{from: "gpt-5.3codexspark", to: "gpt-5.3-codex-spark"},
		{from: "gpt-5.3codex", to: "gpt-5.3-codex"},
	}
	for _, replacement := range replacements {
		normalized = strings.ReplaceAll(normalized, replacement.from, replacement.to)
	}
	return normalized
}

func lastCodexModelSegment(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	parts := strings.Split(model, "/")
	return strings.TrimSpace(parts[len(parts)-1])
}

var codexUpstreamVersionModelPrefixes = []struct {
	prefix string
	target string
}{
	{prefix: "gpt-5.3-codex-spark", target: "gpt-5.3-codex-spark"},
	{prefix: "gpt-5.3-codex", target: "gpt-5.3-codex"},
	{prefix: "gpt-5.4-mini", target: "gpt-5.4-mini"},
	{prefix: "gpt-5.5", target: "gpt-5.5"},
	{prefix: "gpt-5.4", target: "gpt-5.4"},
	{prefix: "gpt-5.2", target: "gpt-5.2"},
}
