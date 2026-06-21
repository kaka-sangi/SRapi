package contract

import (
	"context"
	"time"
)

// FindingKind identifies the safety finding class detected in a gateway request.
type FindingKind string

const (
	FindingKindPIIEmail           FindingKind = "pii_email"
	FindingKindPIIPhone           FindingKind = "pii_phone"
	FindingKindPIISSN             FindingKind = "pii_ssn"
	FindingKindPIINationalID      FindingKind = "pii_national_id"
	FindingKindPIICreditCard      FindingKind = "pii_credit_card"
	FindingKindPromptInjection    FindingKind = "prompt_injection"
	FindingKindCustomKeyword      FindingKind = "custom_keyword"
	FindingKindModerationCategory FindingKind = "moderation_category"
)

// Severity describes the operational severity of a finding.
type Severity string

const (
	SeverityMedium Severity = "medium"
	SeverityHigh   Severity = "high"
)

type Mode string

const (
	ModeMonitor Mode = "monitor"
	ModeEnforce Mode = "enforce"
)

// Config controls gateway request scanning and enforcement.
type Config struct {
	Enabled              bool
	Mode                 Mode
	RedactPII            bool
	BlockPII             bool
	BlockPromptInjection bool
	BlockCustomKeywords  bool
	CustomKeywords       []string
	ModelScopes          []string
	Moderation           ModerationOptions
}

// ModerationOptions controls the upstream-classifier pass that runs after
// the local regex/keyword scan. Provider is the actual transport (HTTP
// client) the service calls; nil disables the pass even when Enabled=true,
// so the runtime can fail-open if credentials are missing or undecryptable.
type ModerationOptions struct {
	Enabled     bool
	BlockOnFlag bool
	Thresholds  map[string]float64
	Provider    ModerationProvider
}

// ModerationProvider classifies a single textual input against a vendor's
// safety taxonomy. Implementations must be safe for concurrent use and
// MUST treat their `input` as untrusted. Use ctx for timeout/cancel; any
// returned error skips the moderation pass for this request rather than
// failing the user's gateway call.
type ModerationProvider interface {
	Classify(ctx context.Context, input string) (ModerationResult, error)
}

// ModerationResult mirrors the structure of OpenAI's /v1/moderations
// response so other providers can be adapted without bending the
// downstream consumer.
type ModerationResult struct {
	Provider   string
	Model      string
	Flagged    bool
	Categories map[string]bool
	Scores     map[string]float64
	LatencyMS  int64
	CachedHit  bool
	FetchedAt  time.Time
}

// Finding is safe audit evidence for a detected request issue.
type Finding struct {
	Kind     FindingKind
	Severity Severity
	Count    int
	Redacted bool
	// Category names the moderation taxonomy class (only set on Kind ==
	// FindingKindModerationCategory). Empty for legacy/local findings.
	Category string
	// Score is the upstream-reported confidence in [0, 1]. Zero when the
	// upstream returns only a boolean classification.
	Score float64
}

// Result reports the mutations and warnings produced by content safety scanning.
type Result struct {
	Changed  bool
	Blocked  bool
	Reason   string
	Findings []Finding
	Warnings []string
}
