package accounts

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entaccountgroupmember "github.com/srapi/srapi/apps/api/ent/accountgroupmember"
	entaccount "github.com/srapi/srapi/apps/api/ent/provideraccount"
	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

var ErrInvalidStore = errors.New("invalid accounts ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.CreateStoredAccount) (contract.ProviderAccount, error) {
	created, err := s.client.ProviderAccount.Create().
		SetProviderID(input.ProviderID).
		SetName(input.Name).
		SetRuntimeClass(string(input.RuntimeClass)).
		SetAccountType(string(input.RuntimeClass)).
		SetNillableUpstreamClient(input.UpstreamClient).
		SetCredentialCiphertext([]byte(input.CredentialCiphertext)).
		SetCredentialVersion(credentialVersionToInt(input.CredentialVersion)).
		SetNillableProxyID(input.ProxyID).
		SetStatus(string(input.Status)).
		SetPriority(input.Priority).
		SetWeight(float64(input.Weight)).
		SetMetadataJSON(cloneMap(input.Metadata)).
		Save(ctx)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	return toAccount(created), nil
}

func (s *Store) Update(ctx context.Context, account contract.ProviderAccount) (contract.ProviderAccount, error) {
	update := s.client.ProviderAccount.UpdateOneID(account.ID).
		Where(entaccount.DeletedAtIsNil()).
		SetProviderID(account.ProviderID).
		SetName(account.Name).
		SetRuntimeClass(string(account.RuntimeClass)).
		SetAccountType(string(account.RuntimeClass)).
		SetNillableUpstreamClient(account.UpstreamClient).
		SetCredentialCiphertext([]byte(account.CredentialCiphertext)).
		SetCredentialVersion(credentialVersionToInt(account.CredentialVersion)).
		SetStatus(string(account.Status)).
		SetPriority(account.Priority).
		SetWeight(float64(account.Weight)).
		SetMetadataJSON(cloneMap(account.Metadata))
	if account.ProxyID == nil {
		update.ClearProxyID()
	} else {
		update.SetProxyID(*account.ProxyID)
	}
	if account.RiskLevel != nil {
		update.SetRiskLevel(*account.RiskLevel)
	}
	if !account.UpdatedAt.IsZero() {
		update.SetUpdatedAt(account.UpdatedAt)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	return toAccount(updated), nil
}

func (s *Store) FindByID(ctx context.Context, id int) (contract.ProviderAccount, error) {
	found, err := s.client.ProviderAccount.Query().
		Where(entaccount.IDEQ(id), entaccount.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	return toAccount(found), nil
}

func (s *Store) List(ctx context.Context) ([]contract.ProviderAccount, error) {
	rows, err := s.client.ProviderAccount.Query().
		Where(entaccount.DeletedAtIsNil()).
		Order(entaccount.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ProviderAccount, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAccount(row))
	}
	return out, nil
}

func (s *Store) ListGroupIDsByAccount(ctx context.Context, accountID int) ([]int, error) {
	rows, err := s.client.AccountGroupMember.Query().
		Where(entaccountgroupmember.AccountIDEQ(accountID)).
		Order(entaccountgroupmember.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.AccountGroupID)
	}
	return out, nil
}

func toAccount(row *ent.ProviderAccount) contract.ProviderAccount {
	return contract.ProviderAccount{
		ID:                   row.ID,
		ProviderID:           row.ProviderID,
		Name:                 row.Name,
		RuntimeClass:         contract.RuntimeClass(row.RuntimeClass),
		UpstreamClient:       cloneString(row.UpstreamClient),
		CredentialCiphertext: string(row.CredentialCiphertext),
		CredentialVersion:    credentialVersionToString(row.CredentialVersion),
		ProxyID:              cloneString(row.ProxyID),
		Status:               contract.Status(row.Status),
		Priority:             row.Priority,
		Weight:               float32(row.Weight),
		RiskLevel:            nonEmptyStringPtr(row.RiskLevel),
		Metadata:             cloneMap(row.MetadataJSON),
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
		DeletedAt:            cloneTime(row.DeletedAt),
	}
}

func credentialVersionToInt(value string) int {
	trimmed := strings.TrimSpace(strings.TrimPrefix(value, "v"))
	if trimmed == "" {
		return 1
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed <= 0 {
		return 1
	}
	return parsed
}

func credentialVersionToString(value int) string {
	if value <= 0 {
		value = 1
	}
	return "v" + strconv.Itoa(value)
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, val := range value {
		cloned[key] = val
	}
	return cloned
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

func nonEmptyStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	cloned := value
	return &cloned
}
