package usage

import (
	"context"
	"errors"

	"github.com/srapi/srapi/apps/api/ent"
	entusagelog "github.com/srapi/srapi/apps/api/ent/usagelog"
	"github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

var ErrInvalidStore = errors.New("invalid usage ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.UsageLog) (contract.UsageLog, error) {
	create := s.client.UsageLog.Create().
		SetRequestID(input.RequestID).
		SetUserID(input.UserID).
		SetAPIKeyID(input.APIKeyID).
		SetNillableProviderID(input.ProviderID).
		SetNillableAccountID(input.AccountID).
		SetSourceProtocol(input.SourceProtocol).
		SetSourceEndpoint(input.SourceEndpoint).
		SetTargetProtocol(input.TargetProtocol).
		SetModel(input.Model).
		SetInputTokens(input.InputTokens).
		SetOutputTokens(input.OutputTokens).
		SetCachedTokens(input.CachedTokens).
		SetTotalTokens(input.TotalTokens).
		SetUsageEstimated(input.UsageEstimated).
		SetLatencyMs(input.LatencyMS).
		SetSuccess(input.Success).
		SetNillableErrorClass(input.ErrorClass).
		SetCost(input.Cost).
		SetCurrency(input.Currency).
		SetCompatibilityWarningsJSON(cloneStrings(input.CompatibilityWarnings))
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.UsageLog{}, err
	}
	return toUsageLog(created), nil
}

func (s *Store) List(ctx context.Context) ([]contract.UsageLog, error) {
	rows, err := s.client.UsageLog.Query().
		Order(entusagelog.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return toUsageLogs(rows), nil
}

func (s *Store) ListByUser(ctx context.Context, userID int) ([]contract.UsageLog, error) {
	rows, err := s.client.UsageLog.Query().
		Where(entusagelog.UserIDEQ(userID)).
		Order(entusagelog.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return toUsageLogs(rows), nil
}

func toUsageLogs(rows []*ent.UsageLog) []contract.UsageLog {
	out := make([]contract.UsageLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, toUsageLog(row))
	}
	return out
}

func toUsageLog(row *ent.UsageLog) contract.UsageLog {
	return contract.UsageLog{
		ID:                    row.ID,
		RequestID:             row.RequestID,
		UserID:                row.UserID,
		APIKeyID:              row.APIKeyID,
		ProviderID:            cloneInt(row.ProviderID),
		AccountID:             cloneInt(row.AccountID),
		SourceProtocol:        row.SourceProtocol,
		SourceEndpoint:        row.SourceEndpoint,
		TargetProtocol:        row.TargetProtocol,
		Model:                 row.Model,
		InputTokens:           row.InputTokens,
		OutputTokens:          row.OutputTokens,
		CachedTokens:          row.CachedTokens,
		TotalTokens:           row.TotalTokens,
		UsageEstimated:        row.UsageEstimated,
		LatencyMS:             row.LatencyMs,
		Success:               row.Success,
		ErrorClass:            cloneString(row.ErrorClass),
		Cost:                  row.Cost,
		Currency:              row.Currency,
		CompatibilityWarnings: cloneStrings(row.CompatibilityWarningsJSON),
		CreatedAt:             row.CreatedAt,
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

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
