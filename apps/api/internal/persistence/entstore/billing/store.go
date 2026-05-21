package billing

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/srapi/srapi/apps/api/ent"
	entbillingledger "github.com/srapi/srapi/apps/api/ent/billingledger"
	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
)

var ErrInvalidStore = errors.New("invalid billing ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.LedgerEntry) (contract.LedgerEntry, error) {
	create := s.client.BillingLedger.Create().
		SetUserID(input.UserID).
		SetType(string(input.Type)).
		SetAmount(input.Amount).
		SetCurrency(input.Currency).
		SetBalanceBefore(input.BalanceBefore).
		SetBalanceAfter(input.BalanceAfter).
		SetReferenceType(input.ReferenceType).
		SetReferenceID(input.ReferenceID).
		SetMetadataJSON(cloneMap(input.Metadata))
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.LedgerEntry{}, err
	}
	return toLedgerEntry(created), nil
}

func (s *Store) List(ctx context.Context) ([]contract.LedgerEntry, error) {
	rows, err := s.client.BillingLedger.Query().
		Order(entbillingledger.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.LedgerEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, toLedgerEntry(row))
	}
	return out, nil
}

func toLedgerEntry(row *ent.BillingLedger) contract.LedgerEntry {
	return contract.LedgerEntry{
		ID:            row.ID,
		UserID:        row.UserID,
		Type:          contract.LedgerType(row.Type),
		Amount:        row.Amount,
		Currency:      row.Currency,
		BalanceBefore: row.BalanceBefore,
		BalanceAfter:  row.BalanceAfter,
		ReferenceType: row.ReferenceType,
		ReferenceID:   row.ReferenceID,
		Metadata:      cloneMap(row.MetadataJSON),
		CreatedAt:     row.CreatedAt,
	}
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
