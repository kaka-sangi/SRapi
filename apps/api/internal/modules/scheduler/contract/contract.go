package contract

import (
	"context"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

type StrategyName string

const (
	StrategyBalanced           StrategyName = "balanced"
	StrategyCostSaver          StrategyName = "cost_saver"
	StrategyLatencyFirst       StrategyName = "latency_first"
	StrategyQuotaProtect       StrategyName = "quota_protect"
	StrategyStickyFirst        StrategyName = "sticky_first"
	StrategyCacheAffinityFirst StrategyName = "cache_affinity_first"
	StrategyPremiumQuality     StrategyName = "premium_quality"
)

type UserTier string

const (
	UserTierFree     UserTier = "free"
	UserTierStandard UserTier = "standard"
	UserTierPro      UserTier = "pro"
	UserTierAdmin    UserTier = "admin"
)

type StickyStrength string

const (
	StickyStrengthNone StickyStrength = ""
	StickyStrengthSoft StickyStrength = "soft"
	StickyStrengthHard StickyStrength = "hard"
)

type LeaseStatus string

const (
	LeaseStatusPending   LeaseStatus = "pending"
	LeaseStatusCommitted LeaseStatus = "committed"
	LeaseStatusReleased  LeaseStatus = "released"
	LeaseStatusExpired   LeaseStatus = "expired"
	LeaseStatusFailed    LeaseStatus = "failed"
)

type StrategyDescriptor struct {
	ID          int
	Name        StrategyName
	Version     string
	Status      string
	ConfigHash  string
	Config      map[string]any
	Weights     map[string]float64
	Description string
}

type Candidate struct {
	Account               accountcontract.ProviderAccount
	Provider              providercontract.Provider
	Mapping               modelcontract.ModelProviderMapping
	EffectiveCapabilities []capabilitiescontract.Descriptor
	RuntimeState          RuntimeState
	Limits                RuntimeLimits
}

type ScheduleRequest struct {
	RequestID               string
	AttemptNo               int
	FallbackFromDecisionID  *int
	UserID                  int
	APIKeyID                int
	SourceProtocol          string
	SourceEndpoint          string
	TargetProtocol          string
	Model                   string
	ModelAlias              string
	FallbackModels          []string
	SessionAffinityKey      string
	SessionAffinitySource   string
	AccountGroupScope       []int
	UserTier                UserTier
	UserBalanceInsufficient bool
	EstimatedInputTokens    int
	EstimatedOutputTokens   int
	EstimatedCost           string
	Currency                string
	PricingRuleID           *int
	PricingSource           string
	PricingEstimated        bool
	IsStream                bool
	StickyAccountID         *int
	StickyStrength          StickyStrength
	Strategy                StrategyName
	StrategyRollout         StrategyRollout
	Warnings                []string
	RequestCapabilities     []capabilitiescontract.Descriptor
	Candidates              []Candidate
	ExcludedAccountIDs      []int
	LeaseTTL                time.Duration
}

// StrategyRollout applies a deterministic shadow strategy percentage to real scheduler attempts.
type StrategyRollout struct {
	Enabled        bool
	ShadowStrategy StrategyName
	Percent        float64
	Key            string
	Bucket         float64
	ShadowSelected bool
	KeyHash        string
}

type RuntimeState struct {
	QuotaExhausted      bool
	HealthScore         *float64
	QuotaRemainingRatio *float64
	LatencyP95MS        *int
	CircuitOpen         bool
	CooldownActive      bool
	CurrentConcurrency  int
	RPMUsed             int
	TPMUsed             int
}

type RuntimeLimits struct {
	MaxConcurrency *int
	RPMLimit       *int
	TPMLimit       *int
}

type Decision struct {
	ID                     int
	RequestID              string
	AttemptNo              int
	UserID                 int
	APIKeyID               int
	SourceProtocol         string
	SourceEndpoint         string
	TargetProtocol         string
	Model                  string
	Strategy               StrategyName
	StrategyVersion        string
	StrategyConfigHash     string
	FallbackFromDecisionID *int
	SelectedProviderID     *int
	SelectedAccountID      *int
	CandidateCount         int
	RejectedCount          int
	Scores                 map[string]any
	RejectReasons          map[string]any
	StrategyWeights        map[string]any
	CompatibilityWarnings  []string
	SelectionRationale     string
	StickyHit              bool
	CacheAffinityHit       bool
	EstimatedCost          string
	Currency               string
	CreatedAt              time.Time
}

type ScheduleResult struct {
	Decision   Decision
	Candidate  Candidate
	Candidates []Candidate
	Lease      Lease
}

// RequestSnapshot persists a sanitized scheduler request profile and candidate set for replay.
type RequestSnapshot struct {
	ID                    int
	RequestID             string
	AttemptNo             int
	DecisionID            int
	RequestProfile        map[string]any
	CandidateSnapshot     []CandidateSnapshot
	RejectedSnapshot      map[string]any
	RankedAccountIDs      []int
	SelectedAccountID     *int
	SelectedProviderID    *int
	Strategy              StrategyName
	StrategyVersion       string
	StrategyConfigHash    string
	StrategyWeights       map[string]any
	CompatibilityWarnings []string
	CreatedAt             time.Time
}

// CandidateSnapshot contains only scheduler-replay fields and never stores credentials.
type CandidateSnapshot struct {
	AccountID             int
	ProviderID            int
	MappingID             int
	ModelID               int
	RuntimeClass          string
	AccountHasCredential  *bool
	AccountStatus         string
	AccountWeight         float32
	AccountRiskLevel      *string
	AccountMetadata       map[string]any
	ProviderProtocol      string
	ProviderStatus        string
	ProviderCapabilities  map[string]any
	ProviderConfig        map[string]any
	MappingStatus         string
	UpstreamModelName     string
	PricingOverride       map[string]any
	EffectiveCapabilities []capabilitiescontract.Descriptor
	RuntimeState          RuntimeState
	Limits                RuntimeLimits
}

// StrategySimulationRequest compares two scheduler strategies against one request profile.
type StrategySimulationRequest struct {
	Request              ScheduleRequest
	CurrentStrategy      StrategyName
	ShadowStrategy       StrategyName
	ShadowRolloutPercent *float64
	RolloutKey           string
}

// StrategySimulationResult contains side-effect-free current and shadow scheduler outcomes.
type StrategySimulationResult struct {
	Current SimulatedStrategyDecision
	Shadow  SimulatedStrategyDecision
	Diff    StrategySimulationDiff
	Rollout StrategySimulationRollout
	DryRun  bool
}

// SimulatedStrategyDecision is a scheduler outcome that was not persisted and did not acquire a lease.
type SimulatedStrategyDecision struct {
	Decision Decision
	Error    string
}

// StrategySimulationDiff captures the operator-facing differences between current and shadow outcomes.
type StrategySimulationDiff struct {
	WinnerChanged             bool
	CurrentSelectedAccountID  *int
	ShadowSelectedAccountID   *int
	CurrentSelectedProviderID *int
	ShadowSelectedProviderID  *int
	FinalScoreDelta           float64
	CostScoreDelta            float64
	LatencyScoreDelta         float64
	QualityScoreDelta         float64
	RiskPenaltyDelta          float64
}

// StrategySimulationRollout previews deterministic shadow gray-release routing for this request.
type StrategySimulationRollout struct {
	Enabled        bool
	Percent        float64
	Bucket         float64
	ShadowSelected bool
	KeyHash        string
}

// StrategyReplayRequest compares scheduler strategies against persisted request snapshots.
type StrategyReplayRequest struct {
	CurrentStrategy      StrategyName
	ShadowStrategy       StrategyName
	ShadowRolloutPercent *float64
	Limit                int
	Since                *time.Time
	Until                *time.Time
	Model                string
	RequestID            string
}

// StrategyReplayResult summarizes side-effect-free historical strategy replay.
type StrategyReplayResult struct {
	DryRun                   bool
	Requested                int
	Replayed                 int
	Skipped                  int
	WinnerChanged            int
	CurrentWinCounts         map[string]int
	ShadowWinCounts          map[string]int
	AverageFinalScoreDelta   float64
	AverageCostScoreDelta    float64
	AverageLatencyScoreDelta float64
	AverageQualityScoreDelta float64
	AverageRiskPenaltyDelta  float64
	Items                    []StrategyReplayItem
}

// StrategyReplayItem is one persisted snapshot replayed through current and shadow strategies.
type StrategyReplayItem struct {
	SnapshotID                int
	DecisionID                int
	RequestID                 string
	AttemptNo                 int
	CreatedAt                 time.Time
	OriginalStrategy          StrategyName
	OriginalSelectedAccountID *int
	Current                   SimulatedStrategyDecision
	Shadow                    SimulatedStrategyDecision
	Diff                      StrategySimulationDiff
	Rollout                   StrategySimulationRollout
}

type Lease struct {
	ID        string
	RequestID string
	AttemptNo int
	AccountID int
	Status    LeaseStatus
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Feedback struct {
	ID           int
	RequestID    string
	DecisionID   int
	AttemptNo    int
	AccountID    int
	ProviderID   int
	Model        string
	Success      bool
	ErrorClass   *string
	StatusCode   *int
	LatencyMS    int
	InputTokens  int
	OutputTokens int
	CachedTokens int
	ActualCost   string
	Currency     string
	CreatedAt    time.Time
}

type RecordFeedbackRequest struct {
	RequestID    string
	DecisionID   int
	AttemptNo    int
	AccountID    int
	ProviderID   int
	Model        string
	Success      bool
	ErrorClass   *string
	StatusCode   *int
	LatencyMS    int
	InputTokens  int
	OutputTokens int
	CachedTokens int
	ActualCost   string
	Currency     string
}

type Store interface {
	CreateDecision(ctx context.Context, input Decision) (Decision, error)
	CreateDecisionWithSnapshot(ctx context.Context, decision Decision, snapshot RequestSnapshot) (Decision, RequestSnapshot, error)
	ListDecisions(ctx context.Context) ([]Decision, error)
	ListRequestSnapshots(ctx context.Context) ([]RequestSnapshot, error)
	CreateFeedback(ctx context.Context, input Feedback) (Feedback, error)
	ListFeedbacks(ctx context.Context) ([]Feedback, error)
	ListActiveStrategies(ctx context.Context) ([]StrategyDescriptor, error)
	AcquireLease(ctx context.Context, input Lease, maxConcurrency *int) (Lease, error)
	UpdateLeaseStatus(ctx context.Context, requestID string, attemptNo int, status LeaseStatus) (Lease, error)
	ListLeases(ctx context.Context) ([]Lease, error)
}
