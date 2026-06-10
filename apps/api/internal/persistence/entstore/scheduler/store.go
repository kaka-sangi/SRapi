package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/srapi/srapi/apps/api/ent"
	"github.com/srapi/srapi/apps/api/ent/predicate"
	entschedulerdecision "github.com/srapi/srapi/apps/api/ent/schedulerdecision"
	entschedulerfeedback "github.com/srapi/srapi/apps/api/ent/schedulerfeedback"
	entschedulerrequestsnapshot "github.com/srapi/srapi/apps/api/ent/schedulerrequestsnapshot"
	entschedulerstrategy "github.com/srapi/srapi/apps/api/ent/schedulerstrategy"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

var ErrInvalidStore = errors.New("invalid scheduler ent store")

type Store struct {
	client         *ent.Client
	mu             sync.Mutex
	leases         map[string]contract.Lease
	leaseByRequest map[string]string
	leaseStore     LeaseStore
}

func New(client *ent.Client) (*Store, error) {
	return NewWithLeaseStore(client, nil)
}

type LeaseStore interface {
	AcquireLease(ctx context.Context, input contract.Lease, maxConcurrency *int) (contract.Lease, error)
	UpdateLeaseStatus(ctx context.Context, requestID string, attemptNo int, status contract.LeaseStatus) (contract.Lease, error)
	ListLeases(ctx context.Context) ([]contract.Lease, error)
}

func NewWithLeaseStore(client *ent.Client, leaseStore LeaseStore) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{
		client:         client,
		leases:         map[string]contract.Lease{},
		leaseByRequest: map[string]string{},
		leaseStore:     leaseStore,
	}, nil
}

func (s *Store) CreateDecision(ctx context.Context, input contract.Decision) (contract.Decision, error) {
	return createDecision(ctx, s.client, input)
}

func (s *Store) CreateDecisionWithSnapshot(ctx context.Context, input contract.Decision, snapshot contract.RequestSnapshot) (contract.Decision, contract.RequestSnapshot, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return contract.Decision{}, contract.RequestSnapshot{}, err
	}
	decision, err := createDecision(ctx, tx.Client(), input)
	if err != nil {
		return contract.Decision{}, contract.RequestSnapshot{}, rollback(tx, err)
	}
	createdSnapshot, err := createRequestSnapshot(ctx, tx.Client(), decision, snapshot)
	if err != nil {
		return contract.Decision{}, contract.RequestSnapshot{}, rollback(tx, err)
	}
	if err := tx.Commit(); err != nil {
		return contract.Decision{}, contract.RequestSnapshot{}, err
	}
	return decision, createdSnapshot, nil
}

func createDecision(ctx context.Context, client *ent.Client, input contract.Decision) (contract.Decision, error) {
	create := client.SchedulerDecision.Create().
		SetRequestID(input.RequestID).
		SetAttemptNo(input.AttemptNo).
		SetUserID(input.UserID).
		SetAPIKeyID(input.APIKeyID).
		SetSourceProtocol(input.SourceProtocol).
		SetSourceEndpoint(input.SourceEndpoint).
		SetTargetProtocol(input.TargetProtocol).
		SetModel(input.Model).
		SetStrategy(string(input.Strategy)).
		SetStrategyVersion(input.StrategyVersion).
		SetStrategyConfigHash(input.StrategyConfigHash).
		SetNillableFallbackFromDecisionID(input.FallbackFromDecisionID).
		SetNillableSelectedProviderID(input.SelectedProviderID).
		SetNillableSelectedAccountID(input.SelectedAccountID).
		SetCandidateCount(input.CandidateCount).
		SetRejectedCount(input.RejectedCount).
		SetScoresJSON(cloneMap(input.Scores)).
		SetRejectReasonsJSON(cloneMap(input.RejectReasons)).
		SetStrategyWeightsJSON(cloneMap(input.StrategyWeights)).
		SetCompatibilityWarningsJSON(cloneStrings(input.CompatibilityWarnings)).
		SetSelectionRationale(input.SelectionRationale).
		SetStickyHit(input.StickyHit).
		SetCacheAffinityHit(input.CacheAffinityHit).
		SetEstimatedCost(input.EstimatedCost).
		SetCurrency(input.Currency)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.Decision{}, err
	}
	return toDecision(created), nil
}

func createRequestSnapshot(ctx context.Context, client *ent.Client, decision contract.Decision, input contract.RequestSnapshot) (contract.RequestSnapshot, error) {
	candidates, err := candidateSnapshotPayload(input.CandidateSnapshot)
	if err != nil {
		return contract.RequestSnapshot{}, err
	}
	snapshot := input
	snapshot.DecisionID = decision.ID
	snapshot.RequestID = decision.RequestID
	snapshot.AttemptNo = decision.AttemptNo
	snapshot.SelectedAccountID = cloneInt(decision.SelectedAccountID)
	snapshot.SelectedProviderID = cloneInt(decision.SelectedProviderID)
	snapshot.Strategy = decision.Strategy
	snapshot.StrategyVersion = decision.StrategyVersion
	snapshot.StrategyConfigHash = decision.StrategyConfigHash
	snapshot.StrategyWeights = cloneMap(decision.StrategyWeights)
	snapshot.CompatibilityWarnings = cloneStrings(decision.CompatibilityWarnings)
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = decision.CreatedAt
	}
	create := client.SchedulerRequestSnapshot.Create().
		SetRequestID(snapshot.RequestID).
		SetAttemptNo(snapshot.AttemptNo).
		SetDecisionID(snapshot.DecisionID).
		SetRequestProfileJSON(cloneMap(snapshot.RequestProfile)).
		SetCandidateSnapshotJSON(candidates).
		SetRejectedSnapshotJSON(cloneMap(snapshot.RejectedSnapshot)).
		SetRankedAccountIdsJSON(cloneInts(snapshot.RankedAccountIDs)).
		SetNillableSelectedAccountID(snapshot.SelectedAccountID).
		SetNillableSelectedProviderID(snapshot.SelectedProviderID).
		SetStrategy(string(snapshot.Strategy)).
		SetStrategyVersion(snapshot.StrategyVersion).
		SetStrategyConfigHash(snapshot.StrategyConfigHash).
		SetStrategyWeightsJSON(cloneMap(snapshot.StrategyWeights)).
		SetCompatibilityWarningsJSON(cloneStrings(snapshot.CompatibilityWarnings))
	if !snapshot.CreatedAt.IsZero() {
		create.SetCreatedAt(snapshot.CreatedAt).SetUpdatedAt(snapshot.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.RequestSnapshot{}, err
	}
	return toRequestSnapshot(created)
}

func (s *Store) ListDecisions(ctx context.Context) ([]contract.Decision, error) {
	rows, err := s.client.SchedulerDecision.Query().
		Order(entschedulerdecision.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Decision, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDecision(row))
	}
	return out, nil
}

func (s *Store) ListRequestSnapshots(ctx context.Context) ([]contract.RequestSnapshot, error) {
	rows, err := s.client.SchedulerRequestSnapshot.Query().
		Order(entschedulerrequestsnapshot.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.RequestSnapshot, 0, len(rows))
	for _, row := range rows {
		snapshot, err := toRequestSnapshot(row)
		if err != nil {
			return nil, err
		}
		out = append(out, snapshot)
	}
	return out, nil
}

func (s *Store) CreateFeedback(ctx context.Context, input contract.Feedback) (contract.Feedback, error) {
	create := s.client.SchedulerFeedback.Create().
		SetRequestID(input.RequestID).
		SetDecisionID(input.DecisionID).
		SetAttemptNo(input.AttemptNo).
		SetAccountID(input.AccountID).
		SetProviderID(input.ProviderID).
		SetModel(input.Model).
		SetSuccess(input.Success).
		SetNillableErrorClass(input.ErrorClass).
		SetNillableStatusCode(input.StatusCode).
		SetLatencyMs(input.LatencyMS).
		SetInputTokens(input.InputTokens).
		SetOutputTokens(input.OutputTokens).
		SetCachedTokens(input.CachedTokens).
		SetActualCost(input.ActualCost).
		SetCurrency(input.Currency)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.Feedback{}, err
	}
	return toFeedback(created), nil
}

func (s *Store) ListFeedbacks(ctx context.Context) ([]contract.Feedback, error) {
	rows, err := s.client.SchedulerFeedback.Query().
		Order(entschedulerfeedback.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Feedback, 0, len(rows))
	for _, row := range rows {
		out = append(out, toFeedback(row))
	}
	return out, nil
}

func (s *Store) ListFeedbackSignals(ctx context.Context, query contract.FeedbackSignalQuery) ([]contract.FeedbackSignal, error) {
	if len(query.AccountIDs) == 0 {
		return []contract.FeedbackSignal{}, nil
	}
	predicates := []predicate.SchedulerFeedback{
		entschedulerfeedback.SuccessEQ(true),
		entschedulerfeedback.AccountIDIn(query.AccountIDs...),
		entschedulerfeedback.Or(
			entschedulerfeedback.InputTokensGT(0),
			entschedulerfeedback.OutputTokensGT(0),
			entschedulerfeedback.CachedTokensGT(0),
		),
	}
	if query.Model != "" {
		predicates = append(predicates, entschedulerfeedback.ModelEQ(query.Model))
	}
	if !query.Since.IsZero() {
		predicates = append(predicates, entschedulerfeedback.CreatedAtGTE(query.Since))
	}
	var rows []feedbackSignalRow
	err := s.client.SchedulerFeedback.Query().
		Where(predicates...).
		GroupBy(entschedulerfeedback.FieldAccountID).
		Aggregate(
			ent.As(ent.Count(), "sample_count"),
			ent.As(ent.Sum(entschedulerfeedback.FieldInputTokens), "input_tokens"),
			ent.As(ent.Sum(entschedulerfeedback.FieldOutputTokens), "output_tokens"),
			ent.As(ent.Sum(entschedulerfeedback.FieldCachedTokens), "cached_tokens"),
			ent.As(sumActualCost(), "total_cost"),
		).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]contract.FeedbackSignal, 0, len(rows))
	for _, row := range rows {
		totalTokens := row.InputTokens + row.OutputTokens + row.CachedTokens
		if row.AccountID <= 0 || row.SampleCount <= 0 || totalTokens <= 0 {
			continue
		}
		signal := contract.FeedbackSignal{
			AccountID:    row.AccountID,
			SampleCount:  int(row.SampleCount),
			InputTokens:  int(row.InputTokens),
			OutputTokens: int(row.OutputTokens),
			CachedTokens: int(row.CachedTokens),
		}
		if row.TotalCost > 0 {
			signal.CostPer1KTokens = row.TotalCost / float64(totalTokens) * 1000
			signal.HasCost = true
		}
		cacheBasis := row.InputTokens + row.CachedTokens
		if cacheBasis <= 0 {
			cacheBasis = totalTokens
		}
		if cacheBasis > 0 {
			signal.CacheHitRate = float64(row.CachedTokens) / float64(cacheBasis)
			signal.HasCache = true
		}
		out = append(out, signal)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AccountID < out[j].AccountID })
	return out, nil
}

type feedbackSignalRow struct {
	AccountID    int     `sql:"account_id"`
	SampleCount  int64   `sql:"sample_count"`
	InputTokens  int64   `sql:"input_tokens"`
	OutputTokens int64   `sql:"output_tokens"`
	CachedTokens int64   `sql:"cached_tokens"`
	TotalCost    float64 `sql:"total_cost"`
}

func sumActualCost() ent.AggregateFunc {
	return func(selector *sql.Selector) string {
		return "SUM(CAST(" + selector.C(entschedulerfeedback.FieldActualCost) + " AS DOUBLE PRECISION))"
	}
}

func (s *Store) ListStrategies(ctx context.Context, query contract.StrategyQuery) ([]contract.StrategyDescriptor, error) {
	q := s.client.SchedulerStrategy.Query()
	predicates := []predicate.SchedulerStrategy{}
	if query.Name != "" {
		predicates = append(predicates, entschedulerstrategy.NameEQ(string(query.Name)))
	}
	if query.Status != "" {
		predicates = append(predicates, entschedulerstrategy.StatusEQ(string(query.Status)))
	}
	if query.ScopeType != "" {
		predicates = append(predicates, entschedulerstrategy.ScopeTypeEQ(string(query.ScopeType)))
	}
	if query.ScopeID != nil {
		predicates = append(predicates, entschedulerstrategy.ScopeIDEQ(*query.ScopeID))
	}
	if len(predicates) > 0 {
		q = q.Where(predicates...)
	}
	rows, err := q.Order(entschedulerstrategy.ByScopeType(), entschedulerstrategy.ByName(), entschedulerstrategy.ByID()).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.StrategyDescriptor, 0, len(rows))
	for _, row := range rows {
		out = append(out, toStrategyDescriptor(row))
	}
	return out, nil
}

func (s *Store) ListActiveStrategies(ctx context.Context) ([]contract.StrategyDescriptor, error) {
	rows, err := s.client.SchedulerStrategy.Query().
		Where(entschedulerstrategy.StatusEQ(string(contract.StrategyStatusActive))).
		Order(entschedulerstrategy.ByScopeType(), entschedulerstrategy.ByName(), entschedulerstrategy.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	latestByName := map[string]*ent.SchedulerStrategy{}
	for _, row := range rows {
		name := strategyScopeNameKey(row)
		current := latestByName[name]
		if current == nil || strategyRowNewer(row, current) {
			latestByName[name] = row
		}
	}
	names := make([]string, 0, len(latestByName))
	for name := range latestByName {
		names = append(names, string(name))
	}
	sort.Strings(names)
	out := make([]contract.StrategyDescriptor, 0, len(names))
	for _, name := range names {
		row := latestByName[name]
		out = append(out, toStrategyDescriptor(row))
	}
	return out, nil
}

func (s *Store) GetStrategy(ctx context.Context, id int) (contract.StrategyDescriptor, error) {
	row, err := s.client.SchedulerStrategy.Get(ctx, id)
	if err != nil {
		return contract.StrategyDescriptor{}, strategyStoreError(err)
	}
	return toStrategyDescriptor(row), nil
}

func (s *Store) CreateStrategy(ctx context.Context, input contract.StrategyDescriptor) (contract.StrategyDescriptor, error) {
	if exists, err := s.strategyVersionExists(ctx, 0, input); err != nil {
		return contract.StrategyDescriptor{}, err
	} else if exists {
		return contract.StrategyDescriptor{}, contract.ErrConflict
	}
	create := s.client.SchedulerStrategy.Create().
		SetName(string(input.Name)).
		SetVersion(input.Version).
		SetStatus(string(input.Status)).
		SetScopeType(string(input.ScopeType)).
		SetNillableScopeID(input.ScopeID).
		SetConfigJSON(cloneMap(input.Config)).
		SetConfigHash(input.ConfigHash).
		SetDescription(input.Description).
		SetNillableCreatedBy(input.CreatedBy).
		SetNillableActivatedAt(input.ActivatedAt).
		SetNillableDeprecatedAt(input.DeprecatedAt)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return contract.StrategyDescriptor{}, strategyStoreError(err)
	}
	return toStrategyDescriptor(row), nil
}

func (s *Store) UpdateStrategy(ctx context.Context, id int, input contract.StrategyDescriptor) (contract.StrategyDescriptor, error) {
	current, err := s.client.SchedulerStrategy.Get(ctx, id)
	if err != nil {
		return contract.StrategyDescriptor{}, strategyStoreError(err)
	}
	next := toStrategyDescriptor(current)
	next = mergeStrategyDescriptor(next, input)
	if exists, err := s.strategyVersionExists(ctx, id, next); err != nil {
		return contract.StrategyDescriptor{}, err
	} else if exists {
		return contract.StrategyDescriptor{}, contract.ErrConflict
	}
	update := s.client.SchedulerStrategy.UpdateOneID(id).
		SetName(string(next.Name)).
		SetVersion(next.Version).
		SetStatus(string(next.Status)).
		SetScopeType(string(next.ScopeType)).
		SetConfigJSON(cloneMap(next.Config)).
		SetConfigHash(next.ConfigHash).
		SetDescription(next.Description).
		SetNillableCreatedBy(next.CreatedBy)
	if next.ScopeID == nil {
		update.ClearScopeID()
	} else {
		update.SetScopeID(*next.ScopeID)
	}
	if next.ActivatedAt == nil {
		update.ClearActivatedAt()
	} else {
		update.SetActivatedAt(*next.ActivatedAt)
	}
	if next.DeprecatedAt == nil {
		update.ClearDeprecatedAt()
	} else {
		update.SetDeprecatedAt(*next.DeprecatedAt)
	}
	row, err := update.Save(ctx)
	if err != nil {
		return contract.StrategyDescriptor{}, strategyStoreError(err)
	}
	return toStrategyDescriptor(row), nil
}

func (s *Store) strategyVersionExists(ctx context.Context, exceptID int, input contract.StrategyDescriptor) (bool, error) {
	predicates := []predicate.SchedulerStrategy{
		entschedulerstrategy.NameEQ(string(input.Name)),
		entschedulerstrategy.VersionEQ(input.Version),
		entschedulerstrategy.ScopeTypeEQ(string(input.ScopeType)),
	}
	if input.ScopeID == nil {
		predicates = append(predicates, entschedulerstrategy.ScopeIDIsNil())
	} else {
		predicates = append(predicates, entschedulerstrategy.ScopeIDEQ(*input.ScopeID))
	}
	if exceptID > 0 {
		predicates = append(predicates, entschedulerstrategy.IDNEQ(exceptID))
	}
	return s.client.SchedulerStrategy.Query().Where(predicates...).Exist(ctx)
}

func mergeStrategyDescriptor(current, input contract.StrategyDescriptor) contract.StrategyDescriptor {
	next := current
	if input.Name != "" {
		next.Name = input.Name
	}
	if input.Version != "" {
		next.Version = input.Version
	}
	if input.Status != "" {
		next.Status = input.Status
	}
	if input.ScopeType != "" {
		next.ScopeType = input.ScopeType
		next.ScopeID = cloneInt(input.ScopeID)
	}
	if input.Config != nil {
		next.Config = cloneMap(input.Config)
	}
	if input.Weights != nil {
		next.Weights = cloneWeights(input.Weights)
	}
	if input.ConfigHash != "" {
		next.ConfigHash = input.ConfigHash
	}
	next.Description = input.Description
	if input.CreatedBy != nil {
		next.CreatedBy = cloneInt(input.CreatedBy)
	}
	next.ActivatedAt = cloneTime(input.ActivatedAt)
	next.DeprecatedAt = cloneTime(input.DeprecatedAt)
	return next
}

func toStrategyDescriptor(row *ent.SchedulerStrategy) contract.StrategyDescriptor {
	if row == nil {
		return contract.StrategyDescriptor{}
	}
	return contract.StrategyDescriptor{
		ID:           row.ID,
		Name:         contract.StrategyName(row.Name),
		Version:      row.Version,
		Status:       contract.StrategyStatus(row.Status),
		ScopeType:    contract.StrategyScopeType(row.ScopeType),
		ScopeID:      cloneInt(row.ScopeID),
		ConfigHash:   row.ConfigHash,
		Config:       cloneMap(row.ConfigJSON),
		Description:  row.Description,
		CreatedBy:    cloneInt(row.CreatedBy),
		CreatedAt:    row.CreatedAt,
		ActivatedAt:  cloneTime(row.ActivatedAt),
		DeprecatedAt: cloneTime(row.DeprecatedAt),
	}
}

func strategyStoreError(err error) error {
	switch {
	case ent.IsNotFound(err):
		return contract.ErrNotFound
	case ent.IsConstraintError(err):
		return contract.ErrConflict
	default:
		return err
	}
}

func strategyScopeNameKey(row *ent.SchedulerStrategy) string {
	scopeID := 0
	if row.ScopeID != nil {
		scopeID = *row.ScopeID
	}
	return row.ScopeType + ":" + strconv.Itoa(scopeID) + ":" + row.Name
}

func strategyRowNewer(left, right *ent.SchedulerStrategy) bool {
	leftTime := strategyRowEffectiveAt(left)
	rightTime := strategyRowEffectiveAt(right)
	if !leftTime.Equal(rightTime) {
		return leftTime.After(rightTime)
	}
	return left.ID > right.ID
}

func strategyRowEffectiveAt(row *ent.SchedulerStrategy) time.Time {
	if row == nil {
		return time.Time{}
	}
	if row.ActivatedAt != nil {
		return *row.ActivatedAt
	}
	if !row.UpdatedAt.IsZero() {
		return row.UpdatedAt
	}
	return row.CreatedAt
}

func rollback(tx *ent.Tx, cause error) error {
	if err := tx.Rollback(); err != nil {
		return fmt.Errorf("%w: rollback: %v", cause, err)
	}
	return cause
}

func (s *Store) AcquireLease(ctx context.Context, input contract.Lease, maxConcurrency *int) (contract.Lease, error) {
	if s.leaseStore != nil {
		return s.leaseStore.AcquireLease(ctx, input, maxConcurrency)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.expireLeases(now)
	if input.ID == "" || input.RequestID == "" || input.AccountID <= 0 {
		return contract.Lease{}, errors.New("invalid lease")
	}
	if input.AttemptNo <= 0 {
		input.AttemptNo = 1
	}
	if maxConcurrency != nil && *maxConcurrency >= 0 && s.pendingConcurrency(input.AccountID) >= *maxConcurrency {
		return contract.Lease{}, errors.New("concurrency full")
	}
	lease := input
	lease.Status = contract.LeaseStatusPending
	if lease.CreatedAt.IsZero() {
		lease.CreatedAt = now
	}
	if lease.UpdatedAt.IsZero() {
		lease.UpdatedAt = lease.CreatedAt
	}
	if lease.ExpiresAt.IsZero() {
		lease.ExpiresAt = now.Add(30 * time.Second)
	}
	s.leases[lease.ID] = lease
	s.leaseByRequest[leaseRequestKey(lease.RequestID, lease.AttemptNo)] = lease.ID
	return lease, nil
}

func (s *Store) UpdateLeaseStatus(ctx context.Context, requestID string, attemptNo int, status contract.LeaseStatus) (contract.Lease, error) {
	if s.leaseStore != nil {
		return s.leaseStore.UpdateLeaseStatus(ctx, requestID, attemptNo, status)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLeases(time.Now().UTC())
	leaseID, ok := s.leaseByRequest[leaseRequestKey(requestID, attemptNo)]
	if !ok {
		return contract.Lease{}, errors.New("lease not found")
	}
	lease, ok := s.leases[leaseID]
	if !ok {
		return contract.Lease{}, errors.New("lease not found")
	}
	if lease.Status == contract.LeaseStatusPending {
		lease.Status = status
		lease.UpdatedAt = time.Now().UTC()
		s.leases[lease.ID] = lease
	}
	return lease, nil
}

// CountAccountConcurrency forwards to the lease store when it can report live
// concurrency (the Redis lease store), otherwise returns 0. Implements
// contract.AccountConcurrencyCounter.
func (s *Store) CountAccountConcurrency(ctx context.Context, accountID int) (int, error) {
	if s.leaseStore == nil {
		return 0, nil
	}
	counter, ok := s.leaseStore.(contract.AccountConcurrencyCounter)
	if !ok {
		return 0, nil
	}
	return counter.CountAccountConcurrency(ctx, accountID)
}

func (s *Store) CountActiveLeases(ctx context.Context) (int, error) {
	if s.leaseStore != nil {
		if counter, ok := s.leaseStore.(contract.ActiveLeaseCounter); ok {
			return counter.CountActiveLeases(ctx)
		}
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLeases(time.Now().UTC())
	count := 0
	for _, lease := range s.leases {
		if lease.Status == contract.LeaseStatusPending {
			count++
		}
	}
	return count, nil
}

// AccountLastUsed forwards to the lease store when it can report it (the Redis
// lease store), otherwise returns 0. Implements contract.AccountLastUsedReporter.
func (s *Store) AccountLastUsed(ctx context.Context, accountID int) (int64, error) {
	if s.leaseStore == nil {
		return 0, nil
	}
	reporter, ok := s.leaseStore.(contract.AccountLastUsedReporter)
	if !ok {
		return 0, nil
	}
	return reporter.AccountLastUsed(ctx, accountID)
}

func (s *Store) ListLeases(ctx context.Context) ([]contract.Lease, error) {
	if s.leaseStore != nil {
		return s.leaseStore.ListLeases(ctx)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLeases(time.Now().UTC())
	out := make([]contract.Lease, 0, len(s.leases))
	for _, lease := range s.leases {
		out = append(out, lease)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) pendingConcurrency(accountID int) int {
	count := 0
	for _, lease := range s.leases {
		if lease.AccountID == accountID && lease.Status == contract.LeaseStatusPending {
			count++
		}
	}
	return count
}

func (s *Store) expireLeases(now time.Time) {
	for id, lease := range s.leases {
		if lease.Status == contract.LeaseStatusPending && !lease.ExpiresAt.IsZero() && !lease.ExpiresAt.After(now) {
			lease.Status = contract.LeaseStatusExpired
			lease.UpdatedAt = now
			s.leases[id] = lease
		}
	}
}

func leaseRequestKey(requestID string, attemptNo int) string {
	if attemptNo <= 0 {
		attemptNo = 1
	}
	return requestID + ":" + strconv.Itoa(attemptNo)
}

func toDecision(row *ent.SchedulerDecision) contract.Decision {
	return contract.Decision{
		ID:                     row.ID,
		RequestID:              row.RequestID,
		AttemptNo:              row.AttemptNo,
		UserID:                 row.UserID,
		APIKeyID:               row.APIKeyID,
		SourceProtocol:         row.SourceProtocol,
		SourceEndpoint:         row.SourceEndpoint,
		TargetProtocol:         row.TargetProtocol,
		Model:                  row.Model,
		Strategy:               contract.StrategyName(row.Strategy),
		StrategyVersion:        row.StrategyVersion,
		StrategyConfigHash:     row.StrategyConfigHash,
		FallbackFromDecisionID: cloneInt(row.FallbackFromDecisionID),
		SelectedProviderID:     cloneInt(row.SelectedProviderID),
		SelectedAccountID:      cloneInt(row.SelectedAccountID),
		CandidateCount:         row.CandidateCount,
		RejectedCount:          row.RejectedCount,
		Scores:                 cloneMap(row.ScoresJSON),
		RejectReasons:          cloneMap(row.RejectReasonsJSON),
		StrategyWeights:        cloneMap(row.StrategyWeightsJSON),
		CompatibilityWarnings:  cloneStrings(row.CompatibilityWarningsJSON),
		SelectionRationale:     row.SelectionRationale,
		StickyHit:              row.StickyHit,
		CacheAffinityHit:       row.CacheAffinityHit,
		EstimatedCost:          row.EstimatedCost,
		Currency:               row.Currency,
		CreatedAt:              row.CreatedAt,
	}
}

func toRequestSnapshot(row *ent.SchedulerRequestSnapshot) (contract.RequestSnapshot, error) {
	candidates, err := toCandidateSnapshots(row.CandidateSnapshotJSON)
	if err != nil {
		return contract.RequestSnapshot{}, err
	}
	return contract.RequestSnapshot{
		ID:                    row.ID,
		RequestID:             row.RequestID,
		AttemptNo:             row.AttemptNo,
		DecisionID:            row.DecisionID,
		RequestProfile:        cloneMap(row.RequestProfileJSON),
		CandidateSnapshot:     candidates,
		RejectedSnapshot:      cloneMap(row.RejectedSnapshotJSON),
		RankedAccountIDs:      cloneInts(row.RankedAccountIdsJSON),
		SelectedAccountID:     cloneInt(row.SelectedAccountID),
		SelectedProviderID:    cloneInt(row.SelectedProviderID),
		Strategy:              contract.StrategyName(row.Strategy),
		StrategyVersion:       row.StrategyVersion,
		StrategyConfigHash:    row.StrategyConfigHash,
		StrategyWeights:       cloneMap(row.StrategyWeightsJSON),
		CompatibilityWarnings: cloneStrings(row.CompatibilityWarningsJSON),
		CreatedAt:             row.CreatedAt,
	}, nil
}

func toFeedback(row *ent.SchedulerFeedback) contract.Feedback {
	return contract.Feedback{
		ID:           row.ID,
		RequestID:    row.RequestID,
		DecisionID:   row.DecisionID,
		AttemptNo:    row.AttemptNo,
		AccountID:    row.AccountID,
		ProviderID:   row.ProviderID,
		Model:        row.Model,
		Success:      row.Success,
		ErrorClass:   cloneString(row.ErrorClass),
		StatusCode:   cloneInt(row.StatusCode),
		LatencyMS:    row.LatencyMs,
		InputTokens:  row.InputTokens,
		OutputTokens: row.OutputTokens,
		CachedTokens: row.CachedTokens,
		ActualCost:   row.ActualCost,
		Currency:     row.Currency,
		CreatedAt:    row.CreatedAt,
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInts(values []int) []int {
	if values == nil {
		return nil
	}
	cloned := make([]int, len(values))
	copy(cloned, values)
	return cloned
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneWeights(values map[string]float64) map[string]float64 {
	if values == nil {
		return nil
	}
	cloned := make(map[string]float64, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func candidateSnapshotPayload(values []contract.CandidateSnapshot) ([]map[string]any, error) {
	if values == nil {
		return []map[string]any{}, nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func toCandidateSnapshots(values []map[string]any) ([]contract.CandidateSnapshot, error) {
	if values == nil {
		return nil, nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	var out []contract.CandidateSnapshot
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return map[string]any{}
	}
	return cloned
}
