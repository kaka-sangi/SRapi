package contract

import (
	"context"
	"errors"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

var (
	ErrNotFound = errors.New("scheduler resource not found")
	ErrConflict = errors.New("scheduler resource conflict")
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

type StrategyStatus string

const (
	StrategyStatusDraft      StrategyStatus = "draft"
	StrategyStatusActive     StrategyStatus = "active"
	StrategyStatusDeprecated StrategyStatus = "deprecated"
)

type StrategyScopeType string

const (
	StrategyScopeGlobal       StrategyScopeType = "global"
	StrategyScopeAPIKey       StrategyScopeType = "api_key"
	StrategyScopeAccountGroup StrategyScopeType = "account_group"
	StrategyScopeUser         StrategyScopeType = "user"
)

type StrategyDescriptor struct {
	ID           int
	Name         StrategyName
	Version      string
	Status       StrategyStatus
	ScopeType    StrategyScopeType
	ScopeID      *int
	ConfigHash   string
	Config       map[string]any
	Weights      map[string]float64
	Description  string
	CreatedBy    *int
	CreatedAt    time.Time
	ActivatedAt  *time.Time
	DeprecatedAt *time.Time
}

type Candidate struct {
	Account               accountcontract.ProviderAccount
	Provider              providercontract.Provider
	Mapping               modelcontract.ModelProviderMapping
	ModelFamily           string
	QualityTier           string
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
	// CostWindowUsed is the account's spend over its rolling cost window
	// (cost_window_seconds), compared against RuntimeLimits.CostWindowLimit.
	CostWindowUsed float64
	// LastUsedUnixMs is when this account was last selected (epoch ms; 0 = never
	// within the tracking window). It is a least-recently-used tie-breaker so
	// equally-scored accounts share load instead of always picking the same one.
	// It is snapshotted per candidate, so scheduler replay stays deterministic.
	LastUsedUnixMs int64
}

type RuntimeLimits struct {
	MaxConcurrency *int
	RPMLimit       *int
	TPMLimit       *int
	// CostWindowLimit caps an account's spend over its rolling cost window; once
	// CostWindowUsed reaches it the account is skipped until the window rolls off.
	CostWindowLimit *float64
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

// FeedbackSignalQuery selects successful feedback used to derive cost/cache scheduler signals.
type FeedbackSignalQuery struct {
	AccountIDs []int
	Model      string
	Since      time.Time
}

// FeedbackSignal is an account-level aggregate used to enrich scheduler cost/cache scores.
type FeedbackSignal struct {
	AccountID       int
	SampleCount     int
	InputTokens     int
	OutputTokens    int
	CachedTokens    int
	CostPer1KTokens float64
	HasCost         bool
	CacheHitRate    float64
	HasCache        bool
}

type StrategyQuery struct {
	Name      StrategyName
	Status    StrategyStatus
	ScopeType StrategyScopeType
	ScopeID   *int
}

type StrategyMutation struct {
	Name        StrategyName
	Version     string
	Status      StrategyStatus
	ScopeType   StrategyScopeType
	ScopeID     *int
	Config      map[string]any
	Weights     map[string]float64
	Description string
	CreatedBy   *int
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

// AccountLastUsedReporter is an optional capability of a Store (and the lease
// store it wraps) that reports when an account was last selected (epoch ms),
// used as a least-recently-used scheduling tie-breaker. Stores that cannot
// report it (e.g. the in-memory store) simply do not implement it.
type AccountLastUsedReporter interface {
	AccountLastUsed(ctx context.Context, accountID int) (int64, error)
}

// AccountConcurrencyCounter is an optional capability of a Store (and of the
// lease store it wraps) that reports the live number of in-flight leases for an
// account. The gateway uses it to feed Candidate.RuntimeState.CurrentConcurrency
// so load-aware scheduler scoring (saturation penalty, concurrency-full reject)
// reflects real traffic instead of always seeing zero. Stores that cannot
// report live concurrency (e.g. the in-memory store) simply do not implement it.
type AccountConcurrencyCounter interface {
	CountAccountConcurrency(ctx context.Context, accountID int) (int, error)
}

// AccountRuntimeBatchReader is an optional capability of a Store that resolves
// live concurrency and last-used markers for many accounts in a constant
// number of round trips (e.g. one Redis MGET per signal). The gateway
// scheduling hot path prefers it over per-account reads when assembling
// candidate runtime state. Accounts without a marker are absent from the maps.
type AccountRuntimeBatchReader interface {
	CountAccountConcurrencyBatch(ctx context.Context, accountIDs []int) (map[int]int, error)
	AccountLastUsedBatch(ctx context.Context, accountIDs []int) (map[int]int64, error)
}

// ActiveLeaseCounter reports live pending scheduler leases without materializing
// every historical lease row. Stores that cannot report it may omit this.
type ActiveLeaseCounter interface {
	CountActiveLeases(ctx context.Context) (int, error)
}

type Store interface {
	CreateDecision(ctx context.Context, input Decision) (Decision, error)
	CreateDecisionWithSnapshot(ctx context.Context, decision Decision, snapshot RequestSnapshot) (Decision, RequestSnapshot, error)
	ListDecisions(ctx context.Context) ([]Decision, error)
	ListRequestSnapshots(ctx context.Context) ([]RequestSnapshot, error)
	CreateFeedback(ctx context.Context, input Feedback) (Feedback, error)
	ListFeedbacks(ctx context.Context) ([]Feedback, error)
	ListFeedbackSignals(ctx context.Context, query FeedbackSignalQuery) ([]FeedbackSignal, error)
	ListStrategies(ctx context.Context, query StrategyQuery) ([]StrategyDescriptor, error)
	ListActiveStrategies(ctx context.Context) ([]StrategyDescriptor, error)
	CreateStrategy(ctx context.Context, input StrategyDescriptor) (StrategyDescriptor, error)
	UpdateStrategy(ctx context.Context, id int, input StrategyDescriptor) (StrategyDescriptor, error)
	GetStrategy(ctx context.Context, id int) (StrategyDescriptor, error)
	AcquireLease(ctx context.Context, input Lease, maxConcurrency *int) (Lease, error)
	UpdateLeaseStatus(ctx context.Context, requestID string, attemptNo int, status LeaseStatus) (Lease, error)
	ListLeases(ctx context.Context) ([]Lease, error)
}
