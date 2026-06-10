package contract

// FindingKind identifies the safety finding class detected in a gateway request.
type FindingKind string

const (
	FindingKindPIIEmail        FindingKind = "pii_email"
	FindingKindPIIPhone        FindingKind = "pii_phone"
	FindingKindPIISSN          FindingKind = "pii_ssn"
	FindingKindPIINationalID   FindingKind = "pii_national_id"
	FindingKindPIICreditCard   FindingKind = "pii_credit_card"
	FindingKindPromptInjection FindingKind = "prompt_injection"
	FindingKindCustomKeyword   FindingKind = "custom_keyword"
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
}

// Finding is safe audit evidence for a detected request issue.
type Finding struct {
	Kind     FindingKind
	Severity Severity
	Count    int
	Redacted bool
}

// Result reports the mutations and warnings produced by content safety scanning.
type Result struct {
	Changed  bool
	Blocked  bool
	Reason   string
	Findings []Finding
	Warnings []string
}
