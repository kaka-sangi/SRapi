package payments

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entpaymentauditlog "github.com/srapi/srapi/apps/api/ent/paymentauditlog"
	entpaymentorder "github.com/srapi/srapi/apps/api/ent/paymentorder"
	entpaymentproviderinstance "github.com/srapi/srapi/apps/api/ent/paymentproviderinstance"
	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
)

var ErrInvalidStore = errors.New("invalid payments ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) CreateProviderInstance(ctx context.Context, input contract.CreateStoredProviderInstance) (contract.PaymentProviderInstance, error) {
	created, err := s.client.PaymentProviderInstance.Create().
		SetProvider(input.Provider).
		SetName(input.Name).
		SetStatus(string(input.Status)).
		SetConfigCiphertext([]byte(input.ConfigCiphertext)).
		SetConfigVersion(input.ConfigVersion).
		SetSupportedMethodsJSON(cloneStringSlice(input.SupportedMethods)).
		SetLimitsJSON(cloneMap(input.Limits)).
		SetSortOrder(input.SortOrder).
		SetMetadataJSON(cloneMap(input.Metadata)).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return contract.PaymentProviderInstance{}, err
		}
		return contract.PaymentProviderInstance{}, err
	}
	return toProvider(created), nil
}

func (s *Store) ListProviderInstances(ctx context.Context) ([]contract.PaymentProviderInstance, error) {
	rows, err := s.client.PaymentProviderInstance.Query().
		Where(entpaymentproviderinstance.DeletedAtIsNil()).
		Order(entpaymentproviderinstance.BySortOrder(), entpaymentproviderinstance.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.PaymentProviderInstance, 0, len(rows))
	for _, row := range rows {
		out = append(out, toProvider(row))
	}
	return out, nil
}

func (s *Store) FindProviderInstanceByID(ctx context.Context, id int) (contract.PaymentProviderInstance, error) {
	row, err := s.client.PaymentProviderInstance.Query().
		Where(entpaymentproviderinstance.IDEQ(id), entpaymentproviderinstance.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.PaymentProviderInstance{}, contract.ErrNotFound
		}
		return contract.PaymentProviderInstance{}, err
	}
	return toProvider(row), nil
}

func (s *Store) CreateOrder(ctx context.Context, input contract.CreateStoredOrder) (contract.PaymentOrder, error) {
	create := s.client.PaymentOrder.Create().
		SetUserID(input.UserID).
		SetOrderNo(input.OrderNo).
		SetProviderInstanceID(input.ProviderInstanceID).
		SetAmount(input.Amount).
		SetCurrency(input.Currency).
		SetStatus(string(input.Status)).
		SetProductType(string(input.ProductType)).
		SetProductID(input.ProductID).
		SetProviderSnapshotJSON(cloneMap(input.ProviderSnapshot)).
		SetNillableExpiresAt(input.ExpiresAt).
		SetMetadataJSON(cloneMap(input.Metadata))
	created, err := create.Save(ctx)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	return toOrder(created), nil
}

func (s *Store) UpdateOrder(ctx context.Context, input contract.PaymentOrder) (contract.PaymentOrder, error) {
	update := s.client.PaymentOrder.UpdateOneID(input.ID).
		SetUserID(input.UserID).
		SetOrderNo(input.OrderNo).
		SetProviderInstanceID(input.ProviderInstanceID).
		SetAmount(input.Amount).
		SetCurrency(input.Currency).
		SetStatus(string(input.Status)).
		SetProductType(string(input.ProductType)).
		SetProductID(input.ProductID).
		SetProviderSnapshotJSON(cloneMap(input.ProviderSnapshot)).
		SetMetadataJSON(cloneMap(input.Metadata)).
		SetNillableProviderTransactionID(input.ProviderTransactionID).
		SetNillableExpiresAt(input.ExpiresAt).
		SetNillablePaidAt(input.PaidAt).
		SetNillableClosedAt(input.ClosedAt)
	if !input.UpdatedAt.IsZero() {
		update.SetUpdatedAt(input.UpdatedAt)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.PaymentOrder{}, contract.ErrNotFound
		}
		return contract.PaymentOrder{}, err
	}
	return toOrder(updated), nil
}

func (s *Store) FindOrderByID(ctx context.Context, id int) (contract.PaymentOrder, error) {
	row, err := s.client.PaymentOrder.Query().
		Where(entpaymentorder.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.PaymentOrder{}, contract.ErrNotFound
		}
		return contract.PaymentOrder{}, err
	}
	return toOrder(row), nil
}

func (s *Store) FindOrderByOrderNo(ctx context.Context, orderNo string) (contract.PaymentOrder, error) {
	row, err := s.client.PaymentOrder.Query().
		Where(entpaymentorder.OrderNoEQ(orderNo)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.PaymentOrder{}, contract.ErrNotFound
		}
		return contract.PaymentOrder{}, err
	}
	return toOrder(row), nil
}

func (s *Store) ListOrders(ctx context.Context) ([]contract.PaymentOrder, error) {
	rows, err := s.client.PaymentOrder.Query().
		Order(entpaymentorder.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.PaymentOrder, 0, len(rows))
	for _, row := range rows {
		out = append(out, toOrder(row))
	}
	return out, nil
}

func (s *Store) ListOrdersByUser(ctx context.Context, userID int) ([]contract.PaymentOrder, error) {
	rows, err := s.client.PaymentOrder.Query().
		Where(entpaymentorder.UserIDEQ(userID)).
		Order(entpaymentorder.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.PaymentOrder, 0, len(rows))
	for _, row := range rows {
		out = append(out, toOrder(row))
	}
	return out, nil
}

func (s *Store) CreateAuditLog(ctx context.Context, input contract.PaymentAuditLog) (contract.PaymentAuditLog, bool, error) {
	if existing, err := s.findAuditLogByIdempotency(ctx, input.IdempotencyKey); err == nil {
		return existing, false, nil
	} else if !ent.IsNotFound(err) {
		return contract.PaymentAuditLog{}, false, err
	}
	create := s.client.PaymentAuditLog.Create().
		SetOrderID(input.OrderID).
		SetProviderInstanceID(input.ProviderInstanceID).
		SetEventType(input.EventType).
		SetIdempotencyKey(input.IdempotencyKey).
		SetPayloadJSON(cloneMap(input.Payload)).
		SetSignatureValid(input.SignatureValid)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			if existing, findErr := s.findAuditLogByIdempotency(ctx, input.IdempotencyKey); findErr == nil {
				return existing, false, nil
			}
		}
		return contract.PaymentAuditLog{}, false, err
	}
	return toAuditLog(created), true, nil
}

func (s *Store) ListAuditLogsByOrder(ctx context.Context, orderID int) ([]contract.PaymentAuditLog, error) {
	rows, err := s.client.PaymentAuditLog.Query().
		Where(entpaymentauditlog.OrderIDEQ(orderID)).
		Order(entpaymentauditlog.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.PaymentAuditLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAuditLog(row))
	}
	return out, nil
}

func (s *Store) findAuditLogByIdempotency(ctx context.Context, key string) (contract.PaymentAuditLog, error) {
	row, err := s.client.PaymentAuditLog.Query().
		Where(entpaymentauditlog.IdempotencyKeyEQ(key)).
		Only(ctx)
	if err != nil {
		return contract.PaymentAuditLog{}, err
	}
	return toAuditLog(row), nil
}

func toProvider(row *ent.PaymentProviderInstance) contract.PaymentProviderInstance {
	return contract.PaymentProviderInstance{
		ID:               row.ID,
		Provider:         row.Provider,
		Name:             row.Name,
		Status:           contract.ProviderStatus(row.Status),
		ConfigCiphertext: string(row.ConfigCiphertext),
		ConfigVersion:    row.ConfigVersion,
		SupportedMethods: cloneStringSlice(row.SupportedMethodsJSON),
		Limits:           cloneMap(row.LimitsJSON),
		SortOrder:        row.SortOrder,
		Metadata:         cloneMap(row.MetadataJSON),
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
		DeletedAt:        cloneTime(row.DeletedAt),
	}
}

func toOrder(row *ent.PaymentOrder) contract.PaymentOrder {
	return contract.PaymentOrder{
		ID:                    row.ID,
		UserID:                row.UserID,
		OrderNo:               row.OrderNo,
		ProviderInstanceID:    row.ProviderInstanceID,
		Amount:                row.Amount,
		Currency:              row.Currency,
		Status:                contract.OrderStatus(row.Status),
		ProductType:           contract.ProductType(row.ProductType),
		ProductID:             row.ProductID,
		ProviderTransactionID: cloneString(row.ProviderTransactionID),
		ProviderSnapshot:      cloneMap(row.ProviderSnapshotJSON),
		ExpiresAt:             cloneTime(row.ExpiresAt),
		PaidAt:                cloneTime(row.PaidAt),
		ClosedAt:              cloneTime(row.ClosedAt),
		Metadata:              cloneMap(row.MetadataJSON),
		CreatedAt:             row.CreatedAt,
		UpdatedAt:             row.UpdatedAt,
	}
}

func toAuditLog(row *ent.PaymentAuditLog) contract.PaymentAuditLog {
	return contract.PaymentAuditLog{
		ID:                 row.ID,
		OrderID:            row.OrderID,
		ProviderInstanceID: row.ProviderInstanceID,
		EventType:          row.EventType,
		IdempotencyKey:     row.IdempotencyKey,
		Payload:            cloneMap(row.PayloadJSON),
		SignatureValid:     row.SignatureValid,
		CreatedAt:          row.CreatedAt,
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

func cloneStringSlice(value []string) []string {
	if value == nil {
		return []string{}
	}
	out := make([]string, len(value))
	copy(out, value)
	return out
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
