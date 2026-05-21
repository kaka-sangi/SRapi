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
	StrategyBalanced  StrategyName = "balanced"
	StrategyCostSaver StrategyName = "cost_saver"
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
	UserID                  int
	APIKeyID                int
	SourceProtocol          string
	SourceEndpoint          string
	TargetProtocol          string
	Model                   string
	UserTier                UserTier
	UserBalanceInsufficient bool
	EstimatedInputTokens    int
	EstimatedOutputTokens   int
	IsStream                bool
	StickyAccountID         *int
	StickyStrength          StickyStrength
	Strategy                StrategyName
	Warnings                []string
	RequestCapabilities     []capabilitiescontract.Descriptor
	Candidates              []Candidate
	LeaseTTL                time.Duration
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
	ID                    int
	RequestID             string
	AttemptNo             int
	UserID                int
	APIKeyID              int
	SourceProtocol        string
	SourceEndpoint        string
	TargetProtocol        string
	Model                 string
	Strategy              StrategyName
	StrategyVersion       string
	StrategyConfigHash    string
	SelectedProviderID    *int
	SelectedAccountID     *int
	CandidateCount        int
	RejectedCount         int
	Scores                map[string]any
	RejectReasons         map[string]any
	StrategyWeights       map[string]any
	CompatibilityWarnings []string
	StickyHit             bool
	CacheAffinityHit      bool
	EstimatedCost         string
	Currency              string
	CreatedAt             time.Time
}

type ScheduleResult struct {
	Decision  Decision
	Candidate Candidate
	Lease     Lease
}

type Lease struct {
	ID        string
	RequestID string
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
	ListDecisions(ctx context.Context) ([]Decision, error)
	CreateFeedback(ctx context.Context, input Feedback) (Feedback, error)
	ListFeedbacks(ctx context.Context) ([]Feedback, error)
	AcquireLease(ctx context.Context, input Lease, maxConcurrency *int) (Lease, error)
	UpdateLeaseStatus(ctx context.Context, requestID string, status LeaseStatus) (Lease, error)
	ListLeases(ctx context.Context) ([]Lease, error)
}
