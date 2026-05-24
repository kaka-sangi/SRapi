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
)

// Severity describes the operational severity of a finding.
type Severity string

const (
	SeverityMedium Severity = "medium"
	SeverityHigh   Severity = "high"
)

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
	Findings []Finding
	Warnings []string
}
