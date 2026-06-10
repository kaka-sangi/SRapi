package payments

import (
	"context"
	stdsql "database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entpaymentauditlog "github.com/srapi/srapi/apps/api/ent/paymentauditlog"
	entpaymentorder "github.com/srapi/srapi/apps/api/ent/paymentorder"
	entpaymentproviderinstance "github.com/srapi/srapi/apps/api/ent/paymentproviderinstance"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	admincontrolstore "github.com/srapi/srapi/apps/api/internal/persistence/entstore/admincontrol"
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
		SetFeeRate(defaultString(input.FeeRate, "0.00000000")).
		SetWeight(defaultWeight(input.Weight)).
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

func (s *Store) UpdateProviderInstance(ctx context.Context, input contract.PaymentProviderInstance) (contract.PaymentProviderInstance, error) {
	update := s.client.PaymentProviderInstance.UpdateOneID(input.ID).
		Where(entpaymentproviderinstance.DeletedAtIsNil()).
		SetProvider(input.Provider).
		SetName(input.Name).
		SetStatus(string(input.Status)).
		SetConfigCiphertext([]byte(input.ConfigCiphertext)).
		SetConfigVersion(input.ConfigVersion).
		SetSupportedMethodsJSON(cloneStringSlice(input.SupportedMethods)).
		SetLimitsJSON(cloneMap(input.Limits)).
		SetSortOrder(input.SortOrder).
		SetFeeRate(defaultString(input.FeeRate, "0.00000000")).
		SetWeight(defaultWeight(input.Weight)).
		SetMetadataJSON(cloneMap(input.Metadata))
	if !input.UpdatedAt.IsZero() {
		update.SetUpdatedAt(input.UpdatedAt)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.PaymentProviderInstance{}, contract.ErrNotFound
		}
		if ent.IsConstraintError(err) {
			return contract.PaymentProviderInstance{}, contract.ErrConflict
		}
		return contract.PaymentProviderInstance{}, err
	}
	return toProvider(updated), nil
}

func (s *Store) SoftDeleteProviderInstance(ctx context.Context, id int) error {
	affected, err := s.client.PaymentProviderInstance.Update().
		Where(entpaymentproviderinstance.IDEQ(id), entpaymentproviderinstance.DeletedAtIsNil()).
		SetDeletedAt(time.Now().UTC()).
		SetStatus(string(contract.ProviderStatusDisabled)).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return contract.ErrNotFound
	}
	return nil
}

func (s *Store) PreviewPromoCode(ctx context.Context, input contract.PromoCodePreviewInput) (contract.PromoCodeApplication, error) {
	application, err := admincontrolstore.PreviewPromoCodeWithClient(ctx, s.client, admincontrolcontract.PromoCodePreviewInput{
		UserID:   input.UserID,
		Code:     input.Code,
		Amount:   input.Amount,
		Currency: input.Currency,
		Now:      input.Now,
	})
	if err != nil {
		return contract.PromoCodeApplication{}, paymentPromoError(err)
	}
	return toPaymentPromoCodeApplication(application), nil
}

func (s *Store) ReleasePromoCode(ctx context.Context, input contract.PromoCodeReleaseInput) (contract.PromoCodeApplication, bool, error) {
	application, released, err := admincontrolstore.ReleasePromoCodeWithClient(ctx, s.client, admincontrolcontract.PromoCodeReleaseInput{
		PaymentOrderID: input.PaymentOrderID,
		ReleasedAt:     input.ReleasedAt,
		Reason:         input.Reason,
	})
	if err != nil {
		return contract.PromoCodeApplication{}, false, paymentPromoError(err)
	}
	return toPaymentPromoCodeApplication(application), released, nil
}

func (s *Store) CreateOrder(ctx context.Context, input contract.CreateStoredOrder) (contract.PaymentOrder, error) {
	if strings.TrimSpace(input.PromoCode) == "" || input.PromoCodeID == nil {
		return s.createOrder(ctx, s.client, input)
	}
	tx, err := s.client.BeginTx(ctx, &stdsql.TxOptions{Isolation: stdsql.LevelSerializable})
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	order, err := s.createOrder(ctx, tx.Client(), input)
	if err != nil {
		_ = tx.Rollback()
		return contract.PaymentOrder{}, err
	}
	_, err = admincontrolstore.FinalizePromoCodeWithClient(ctx, tx.Client(), admincontrolcontract.PromoCodeFinalizeInput{
		UserID:         input.UserID,
		Code:           input.PromoCode,
		PaymentOrderID: order.ID,
		OrderNo:        order.OrderNo,
		OriginalAmount: order.OriginalAmount,
		FinalAmount:    order.Amount,
		Currency:       order.Currency,
		AppliedAt:      order.CreatedAt,
	})
	if err != nil {
		_ = tx.Rollback()
		return contract.PaymentOrder{}, paymentPromoError(err)
	}
	if err := tx.Commit(); err != nil {
		return contract.PaymentOrder{}, err
	}
	return order, nil
}

func (s *Store) createOrder(ctx context.Context, client *ent.Client, input contract.CreateStoredOrder) (contract.PaymentOrder, error) {
	originalAmount := defaultString(input.OriginalAmount, input.Amount)
	discountAmount := defaultString(input.DiscountAmount, "0.00000000")
	create := client.PaymentOrder.Create().
		SetUserID(input.UserID).
		SetOrderNo(input.OrderNo).
		SetProviderInstanceID(input.ProviderInstanceID).
		SetOriginalAmount(originalAmount).
		SetDiscountAmount(discountAmount).
		SetFeeAmount(defaultString(input.FeeAmount, "0.00000000")).
		SetPayableAmount(defaultString(input.PayableAmount, input.Amount)).
		SetNillablePromoCodeID(input.PromoCodeID).
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
		SetOriginalAmount(defaultString(input.OriginalAmount, input.Amount)).
		SetDiscountAmount(defaultString(input.DiscountAmount, "0.00000000")).
		SetFeeAmount(defaultString(input.FeeAmount, "0.00000000")).
		SetPayableAmount(defaultString(input.PayableAmount, input.Amount)).
		SetNillablePromoCodeID(input.PromoCodeID).
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

func (s *Store) ListPendingOrders(ctx context.Context, now time.Time) ([]contract.PaymentOrder, error) {
	rows, err := s.client.PaymentOrder.Query().
		Where(
			entpaymentorder.StatusEQ(string(contract.OrderStatusPending)),
			entpaymentorder.Or(
				entpaymentorder.ExpiresAtIsNil(),
				entpaymentorder.ExpiresAtGT(now.UTC()),
			),
		).
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

func (s *Store) ListExpiredPendingOrders(ctx context.Context, now time.Time) ([]contract.PaymentOrder, error) {
	rows, err := s.client.PaymentOrder.Query().
		Where(
			entpaymentorder.StatusEQ(string(contract.OrderStatusPending)),
			entpaymentorder.ExpiresAtNotNil(),
			entpaymentorder.ExpiresAtLT(now.UTC()),
		).
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

func (s *Store) CountInProgressOrdersByProviderInstance(ctx context.Context, providerInstanceID int) (int, error) {
	return s.client.PaymentOrder.Query().
		Where(
			entpaymentorder.ProviderInstanceIDEQ(providerInstanceID),
			entpaymentorder.StatusIn(inProgressOrderStatuses()...),
		).
		Count(ctx)
}

func (s *Store) ExpireOrder(ctx context.Context, orderID int, now time.Time) (contract.PaymentOrder, bool, error) {
	now = now.UTC()
	updated, err := s.client.PaymentOrder.UpdateOneID(orderID).
		Where(
			entpaymentorder.StatusEQ(string(contract.OrderStatusPending)),
			entpaymentorder.ExpiresAtNotNil(),
			entpaymentorder.ExpiresAtLT(now),
		).
		SetStatus(string(contract.OrderStatusExpired)).
		SetClosedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err == nil {
		return toOrder(updated), true, nil
	}
	if !ent.IsNotFound(err) {
		return contract.PaymentOrder{}, false, err
	}
	order, findErr := s.FindOrderByID(ctx, orderID)
	if findErr != nil {
		return contract.PaymentOrder{}, false, findErr
	}
	return order, false, nil
}

func inProgressOrderStatuses() []string {
	return []string{
		string(contract.OrderStatusPending),
		string(contract.OrderStatusPaid),
		string(contract.OrderStatusRefunding),
	}
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
		FeeRate:          defaultString(row.FeeRate, "0.00000000"),
		Weight:           defaultWeight(row.Weight),
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
		OriginalAmount:        defaultString(row.OriginalAmount, row.Amount),
		DiscountAmount:        defaultString(row.DiscountAmount, "0.00000000"),
		FeeAmount:             defaultString(row.FeeAmount, "0.00000000"),
		PayableAmount:         defaultString(row.PayableAmount, row.Amount),
		PromoCodeID:           cloneInt(row.PromoCodeID),
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

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultWeight(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func toPaymentPromoCodeApplication(application admincontrolcontract.PromoCodeApplication) contract.PromoCodeApplication {
	return contract.PromoCodeApplication{
		ID:             application.ID,
		UserID:         application.UserID,
		PromoCodeID:    application.PromoCodeID,
		PaymentOrderID: application.PaymentOrderID,
		OrderNo:        application.OrderNo,
		OriginalAmount: application.OriginalAmount,
		DiscountAmount: application.DiscountAmount,
		FinalAmount:    application.FinalAmount,
		Currency:       application.Currency,
		DiscountType:   string(application.DiscountType),
		AppliedAt:      application.AppliedAt,
		Metadata:       cloneMap(application.Metadata),
		CreatedAt:      application.CreatedAt,
		UpdatedAt:      application.UpdatedAt,
	}
}

func paymentPromoError(err error) error {
	switch {
	case errors.Is(err, admincontrolcontract.ErrInvalidInput):
		return contract.ErrInvalidInput
	case errors.Is(err, admincontrolcontract.ErrNotFound):
		return contract.ErrNotFound
	case errors.Is(err, admincontrolcontract.ErrConflict):
		return contract.ErrConflict
	default:
		return err
	}
}
