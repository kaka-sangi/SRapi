package service

import (
	"context"
	"regexp"
	"sort"
	"strings"

	contentsafetycontract "github.com/srapi/srapi/apps/api/internal/modules/content_safety/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
)

const (
	warningPIIRedacted             = "content_safety_pii_redacted"
	warningPromptInjectionDetected = "content_safety_prompt_injection_detected"
	warningModerationFlagged       = "content_safety_moderation_flagged"
	warningModerationCallFailed    = "content_safety_moderation_failed"
	moderationDefaultFlaggedScore  = 1.0
	moderationMaxInputCharacters   = 50_000
)

var piiPatterns = []redactionPattern{
	{
		kind:        contentsafetycontract.FindingKindPIIEmail,
		severity:    contentsafetycontract.SeverityMedium,
		replacement: "[REDACTED_EMAIL]",
		pattern:     regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`),
	},
	{
		kind:        contentsafetycontract.FindingKindPIIPhone,
		severity:    contentsafetycontract.SeverityMedium,
		replacement: "[REDACTED_PHONE]",
		pattern:     regexp.MustCompile(`(?:\+?1[\s.\-]?)?(?:\([2-9][0-9]{2}\)|[2-9][0-9]{2})[\s.\-]?[0-9]{3}[\s.\-]?[0-9]{4}\b`),
	},
	{
		kind:        contentsafetycontract.FindingKindPIIPhone,
		severity:    contentsafetycontract.SeverityMedium,
		replacement: "[REDACTED_PHONE]",
		pattern:     regexp.MustCompile(`(?:\+?86[\s.\-]?)?1[3-9][0-9][\s.\-]?[0-9]{4}[\s.\-]?[0-9]{4}\b`),
	},
	{
		kind:        contentsafetycontract.FindingKindPIISSN,
		severity:    contentsafetycontract.SeverityHigh,
		replacement: "[REDACTED_SSN]",
		pattern:     regexp.MustCompile(`\b[0-9]{3}-[0-9]{2}-[0-9]{4}\b`),
	},
	{
		kind:        contentsafetycontract.FindingKindPIINationalID,
		severity:    contentsafetycontract.SeverityHigh,
		replacement: "[REDACTED_NATIONAL_ID]",
		pattern:     regexp.MustCompile(`\b[1-9][0-9]{5}(?:18|19|20)[0-9]{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12][0-9]|3[01])[0-9]{3}[0-9Xx]\b`),
	},
	{
		kind:        contentsafetycontract.FindingKindPIICreditCard,
		severity:    contentsafetycontract.SeverityHigh,
		replacement: "[REDACTED_CREDIT_CARD]",
		pattern:     regexp.MustCompile(`\b(?:[0-9][ -]?){13,19}\b`),
	},
}

var promptInjectionPatterns = []string{
	"ignore previous instructions",
	"ignore all previous instructions",
	"disregard previous instructions",
	"developer mode",
	"reveal your system prompt",
	"show me your system prompt",
	"print your system prompt",
}

// Service applies lightweight content-safety checks to canonical gateway requests.
type Service struct {
	config contentsafetycontract.Config
}

type redactionPattern struct {
	kind        contentsafetycontract.FindingKind
	severity    contentsafetycontract.Severity
	replacement string
	pattern     *regexp.Regexp
}

// New creates a content-safety service with the supplied runtime defaults.
func New(config contentsafetycontract.Config) *Service {
	return &Service{config: NormalizeConfig(config)}
}

func DefaultConfig() contentsafetycontract.Config {
	return contentsafetycontract.Config{
		Enabled:              true,
		Mode:                 contentsafetycontract.ModeMonitor,
		RedactPII:            true,
		BlockPII:             false,
		BlockPromptInjection: false,
		BlockCustomKeywords:  false,
		CustomKeywords:       []string{},
		ModelScopes:          []string{},
	}
}

func NormalizeConfig(config contentsafetycontract.Config) contentsafetycontract.Config {
	if config.Mode != contentsafetycontract.ModeMonitor && config.Mode != contentsafetycontract.ModeEnforce {
		config.Mode = contentsafetycontract.ModeMonitor
	}
	config.CustomKeywords = uniqueLowerTrimmedStrings(config.CustomKeywords)
	config.ModelScopes = uniqueLowerTrimmedStrings(config.ModelScopes)
	return config
}

// Apply redacts detected PII from gateway request text fields and records safe finding evidence.
func (s *Service) Apply(req gatewaycontract.CanonicalRequest) (gatewaycontract.CanonicalRequest, contentsafetycontract.Result) {
	return s.ApplyWithContext(context.Background(), req, s.config)
}

// ApplyWithConfig retains the synchronous, provider-less call shape used by
// tests and the runtime initialization path. Callers that need the upstream
// moderation pass should invoke ApplyWithContext with a ctx the caller
// controls so the upstream classifier can be timed out.
func (s *Service) ApplyWithConfig(req gatewaycontract.CanonicalRequest, config contentsafetycontract.Config) (gatewaycontract.CanonicalRequest, contentsafetycontract.Result) {
	return s.ApplyWithContext(context.Background(), req, config)
}

// ApplyWithContext runs the local regex/keyword pass followed by the
// optional upstream moderation pass. The moderation pass is skipped when
// config.Moderation.Provider is nil so the runtime can fail-open (missing
// credentials, transient API key decryption errors) without aborting the
// request.
func (s *Service) ApplyWithContext(ctx context.Context, req gatewaycontract.CanonicalRequest, config contentsafetycontract.Config) (gatewaycontract.CanonicalRequest, contentsafetycontract.Result) {
	config = NormalizeConfig(config)
	result := contentsafetycontract.Result{}
	if !config.Enabled {
		return req, result
	}
	if !configMatchesModel(config, req.CanonicalModel, req.Model) {
		return req, result
	}
	req.Prompt = s.scanText(req.Prompt, config, &result)
	req.Instructions = s.scanText(req.Instructions, config, &result)
	req.InputItems = s.scanContentBlocks(req.InputItems, config, &result)
	req.Messages = s.scanMessages(req.Messages, config, &result)
	req.EmbeddingInput = s.scanStrings(req.EmbeddingInput, config, &result)
	req.ImagePrompt = s.scanText(req.ImagePrompt, config, &result)
	req.AudioPrompt = s.scanText(req.AudioPrompt, config, &result)
	req.SpeechInput = s.scanText(req.SpeechInput, config, &result)
	req.SpeechInstructions = s.scanText(req.SpeechInstructions, config, &result)
	req.ModerationInput = s.scanStrings(req.ModerationInput, config, &result)
	req.RerankQuery = s.scanText(req.RerankQuery, config, &result)
	req.RerankDocuments = s.scanRerankDocuments(req.RerankDocuments, config, &result)
	applyModeration(ctx, config, req, &result)
	applyContentSafetyConfig(config, &result)
	result.Findings = sortedFindings(result.Findings)
	// Merge the warnings derived from findings with any warnings the
	// moderation pass appended directly (e.g. upstream-failure surface).
	// Re-assigning to resultWarnings(result.Findings) would silently drop
	// those direct appends.
	result.Warnings = mergeWarnings(result.Warnings, resultWarnings(result.Findings))
	req.CompatibilityWarnings = mergeWarnings(req.CompatibilityWarnings, result.Warnings)
	return req, result
}

// applyModeration concatenates all scanned text fields into a single
// classifier input and adds findings for every category that exceeds its
// configured threshold (or fires the upstream `flagged` boolean when no
// thresholds are set). On upstream error the call is recorded as a
// warning rather than failing the request.
func applyModeration(ctx context.Context, config contentsafetycontract.Config, req gatewaycontract.CanonicalRequest, result *contentsafetycontract.Result) {
	if result == nil || !config.Moderation.Enabled || config.Moderation.Provider == nil {
		return
	}
	text := collectModerationInput(req)
	if text == "" {
		return
	}
	if len(text) > moderationMaxInputCharacters {
		text = text[:moderationMaxInputCharacters]
	}
	classification, err := config.Moderation.Provider.Classify(ctx, text)
	if err != nil {
		result.Warnings = append(result.Warnings, warningModerationCallFailed)
		return
	}
	categories := classifierTriggeredCategories(classification, config.Moderation.Thresholds)
	// Without operator thresholds, defer to the upstream `flagged` boolean.
	// With thresholds, the operator is explicitly opting in to per-category
	// gating: a flagged-but-below-threshold response must NOT be added back
	// in as a generic finding — that would defeat the threshold.
	if len(categories) == 0 {
		if !classification.Flagged || len(config.Moderation.Thresholds) > 0 {
			return
		}
		// Upstream said flagged but did not break it down — record a single
		// catch-all so the audit log shows the verdict instead of dropping it.
		categories = append(categories, classifierCategory{Name: "flagged", Score: moderationDefaultFlaggedScore})
	}
	for _, category := range categories {
		result.Findings = append(result.Findings, contentsafetycontract.Finding{
			Kind:     contentsafetycontract.FindingKindModerationCategory,
			Severity: contentsafetycontract.SeverityHigh,
			Count:    1,
			Category: category.Name,
			Score:    category.Score,
		})
	}
	if config.Mode == contentsafetycontract.ModeEnforce && config.Moderation.BlockOnFlag {
		result.Blocked = true
		result.Reason = "moderation_flagged"
	}
}

type classifierCategory struct {
	Name  string
	Score float64
}

func classifierTriggeredCategories(result contentsafetycontract.ModerationResult, thresholds map[string]float64) []classifierCategory {
	if len(result.Categories) == 0 && len(result.Scores) == 0 {
		return nil
	}
	triggered := map[string]classifierCategory{}
	for category, hit := range result.Categories {
		category = strings.ToLower(strings.TrimSpace(category))
		if category == "" || !hit {
			continue
		}
		if threshold, ok := thresholds[category]; ok && threshold > 0 {
			if score := result.Scores[category]; score < threshold {
				continue
			}
		}
		triggered[category] = classifierCategory{Name: category, Score: result.Scores[category]}
	}
	for category, threshold := range thresholds {
		if threshold <= 0 {
			continue
		}
		score, ok := result.Scores[category]
		if !ok {
			continue
		}
		if score < threshold {
			continue
		}
		if _, already := triggered[category]; already {
			continue
		}
		triggered[category] = classifierCategory{Name: category, Score: score}
	}
	out := make([]classifierCategory, 0, len(triggered))
	for _, value := range triggered {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// collectModerationInput stitches every text field the regex pass walks
// into one input — the moderation API takes a single string per call and
// running it once per request is far cheaper than per-field.
func collectModerationInput(req gatewaycontract.CanonicalRequest) string {
	var b strings.Builder
	appendField := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(value)
	}
	appendField(req.Prompt)
	appendField(req.Instructions)
	appendField(req.ImagePrompt)
	appendField(req.AudioPrompt)
	appendField(req.SpeechInput)
	appendField(req.SpeechInstructions)
	appendField(req.RerankQuery)
	for _, item := range req.InputItems {
		appendField(item.Text)
	}
	for _, msg := range req.Messages {
		for _, block := range msg.Content {
			appendField(block.Text)
		}
	}
	for _, value := range req.EmbeddingInput {
		appendField(value)
	}
	for _, value := range req.ModerationInput {
		appendField(value)
	}
	for _, doc := range req.RerankDocuments {
		appendField(doc.Text)
	}
	return b.String()
}

func (s *Service) scanMessages(values []gatewaycontract.Message, config contentsafetycontract.Config, result *contentsafetycontract.Result) []gatewaycontract.Message {
	if values == nil {
		return nil
	}
	out := append([]gatewaycontract.Message(nil), values...)
	for i := range out {
		out[i].Content = s.scanContentBlocks(out[i].Content, config, result)
	}
	return out
}

func (s *Service) scanContentBlocks(values []gatewaycontract.ContentBlock, config contentsafetycontract.Config, result *contentsafetycontract.Result) []gatewaycontract.ContentBlock {
	if values == nil {
		return nil
	}
	out := append([]gatewaycontract.ContentBlock(nil), values...)
	for i := range out {
		out[i].Text = s.scanText(out[i].Text, config, result)
	}
	return out
}

func (s *Service) scanRerankDocuments(values []gatewaycontract.RerankDocument, config contentsafetycontract.Config, result *contentsafetycontract.Result) []gatewaycontract.RerankDocument {
	if values == nil {
		return nil
	}
	out := append([]gatewaycontract.RerankDocument(nil), values...)
	for i := range out {
		out[i].Text = s.scanText(out[i].Text, config, result)
	}
	return out
}

func (s *Service) scanStrings(values []string, config contentsafetycontract.Config, result *contentsafetycontract.Result) []string {
	if values == nil {
		return nil
	}
	out := append([]string(nil), values...)
	for i := range out {
		out[i] = s.scanText(out[i], config, result)
	}
	return out
}

func (s *Service) scanText(value string, config contentsafetycontract.Config, result *contentsafetycontract.Result) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	redacted := value
	for _, item := range piiPatterns {
		matches := contentSafetyMatches(item, redacted)
		if len(matches) == 0 {
			continue
		}
		if config.RedactPII {
			redacted = replaceContentSafetyMatches(item, redacted)
			result.Changed = true
		}
		addFinding(result, item.kind, item.severity, len(matches), config.RedactPII)
	}
	lower := strings.ToLower(value)
	for _, phrase := range promptInjectionPatterns {
		if strings.Contains(lower, phrase) {
			addFinding(result, contentsafetycontract.FindingKindPromptInjection, contentsafetycontract.SeverityMedium, 1, false)
		}
	}
	for _, keyword := range config.CustomKeywords {
		if strings.Contains(lower, keyword) {
			addFinding(result, contentsafetycontract.FindingKindCustomKeyword, contentsafetycontract.SeverityHigh, strings.Count(lower, keyword), false)
		}
	}
	return redacted
}

func contentSafetyMatches(item redactionPattern, value string) []string {
	indexes := item.pattern.FindAllStringIndex(value, -1)
	out := make([]string, 0, len(indexes))
	for _, index := range indexes {
		match := value[index[0]:index[1]]
		if contentSafetyMatchAllowed(item.kind, value, index[0], index[1], match) {
			out = append(out, match)
		}
	}
	return out
}

func replaceContentSafetyMatches(item redactionPattern, value string) string {
	indexes := item.pattern.FindAllStringIndex(value, -1)
	if len(indexes) == 0 {
		return value
	}
	var out strings.Builder
	start := 0
	for _, index := range indexes {
		match := value[index[0]:index[1]]
		if !contentSafetyMatchAllowed(item.kind, value, index[0], index[1], match) {
			continue
		}
		out.WriteString(value[start:index[0]])
		out.WriteString(item.replacement)
		start = index[1]
	}
	if start == 0 {
		return value
	}
	out.WriteString(value[start:])
	return out.String()
}

func contentSafetyMatchAllowed(kind contentsafetycontract.FindingKind, value string, start int, end int, match string) bool {
	if kind == contentsafetycontract.FindingKindPIICreditCard {
		return luhnValidDigits(match)
	}
	if kind == contentsafetycontract.FindingKindPIIPhone {
		return !adjacentDigit(value, start-1) && !adjacentDigit(value, end)
	}
	return true
}

func adjacentDigit(value string, idx int) bool {
	if idx < 0 || idx >= len(value) {
		return false
	}
	return value[idx] >= '0' && value[idx] <= '9'
}

func luhnValidDigits(value string) bool {
	digits := make([]int, 0, len(value))
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digits = append(digits, int(char-'0'))
		}
	}
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	var sum int
	double := false
	for idx := len(digits) - 1; idx >= 0; idx-- {
		digit := digits[idx]
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}
	return sum%10 == 0
}

func applyContentSafetyConfig(config contentsafetycontract.Config, result *contentsafetycontract.Result) {
	if result == nil || config.Mode != contentsafetycontract.ModeEnforce {
		return
	}
	for _, finding := range result.Findings {
		if finding.Kind == contentsafetycontract.FindingKindPromptInjection && config.BlockPromptInjection {
			result.Blocked = true
			result.Reason = "prompt_injection"
			return
		}
		if finding.Kind == contentsafetycontract.FindingKindCustomKeyword && config.BlockCustomKeywords {
			result.Blocked = true
			result.Reason = "custom_keyword"
			return
		}
		if isPIIFinding(finding.Kind) && config.BlockPII {
			result.Blocked = true
			result.Reason = string(finding.Kind)
			return
		}
	}
}

func configMatchesModel(config contentsafetycontract.Config, canonicalModel string, requestedModel string) bool {
	if len(config.ModelScopes) == 0 {
		return true
	}
	model := strings.ToLower(strings.TrimSpace(canonicalModel))
	if model == "" {
		model = strings.ToLower(strings.TrimSpace(requestedModel))
	}
	if model == "" {
		return false
	}
	for _, scope := range config.ModelScopes {
		if model == scope {
			return true
		}
		if strings.HasSuffix(scope, "*") && strings.HasPrefix(model, strings.TrimSuffix(scope, "*")) {
			return true
		}
	}
	return false
}

func isPIIFinding(kind contentsafetycontract.FindingKind) bool {
	switch kind {
	case contentsafetycontract.FindingKindPIIEmail,
		contentsafetycontract.FindingKindPIIPhone,
		contentsafetycontract.FindingKindPIISSN,
		contentsafetycontract.FindingKindPIINationalID,
		contentsafetycontract.FindingKindPIICreditCard:
		return true
	default:
		return false
	}
}

func uniqueLowerTrimmedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func addFinding(result *contentsafetycontract.Result, kind contentsafetycontract.FindingKind, severity contentsafetycontract.Severity, count int, redacted bool) {
	if result == nil || count <= 0 {
		return
	}
	for i := range result.Findings {
		if result.Findings[i].Kind == kind {
			result.Findings[i].Count += count
			result.Findings[i].Redacted = result.Findings[i].Redacted || redacted
			return
		}
	}
	result.Findings = append(result.Findings, contentsafetycontract.Finding{
		Kind:     kind,
		Severity: severity,
		Count:    count,
		Redacted: redacted,
	})
}

func sortedFindings(values []contentsafetycontract.Finding) []contentsafetycontract.Finding {
	out := append([]contentsafetycontract.Finding(nil), values...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Kind < out[j].Kind
	})
	return out
}

func resultWarnings(findings []contentsafetycontract.Finding) []string {
	hasPII := false
	hasPromptInjection := false
	hasModeration := false
	for _, finding := range findings {
		switch finding.Kind {
		case contentsafetycontract.FindingKindPromptInjection:
			hasPromptInjection = true
			continue
		case contentsafetycontract.FindingKindModerationCategory:
			hasModeration = true
			continue
		}
		if finding.Redacted {
			hasPII = true
		}
	}
	warnings := []string{}
	if hasPII {
		warnings = append(warnings, warningPIIRedacted)
	}
	if hasPromptInjection {
		warnings = append(warnings, warningPromptInjectionDetected)
	}
	if hasModeration {
		warnings = append(warnings, warningModerationFlagged)
	}
	return warnings
}

func mergeWarnings(existing []string, added []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(existing)+len(added))
	for _, value := range append(append([]string(nil), existing...), added...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
