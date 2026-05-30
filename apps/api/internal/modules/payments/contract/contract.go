package contract

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("payment resource not found")
	ErrConflict     = errors.New("payment resource conflict")
	ErrInvalidInput = errors.New("invalid payment input")
)

type ProviderStatus string

const (
	ProviderStatusActive   ProviderStatus = "active"
	ProviderStatusDisabled ProviderStatus = "disabled"
	ProviderStatusArchived ProviderStatus = "archived"
)

type OrderStatus string

const (
	OrderStatusPending           OrderStatus = "pending"
	OrderStatusPaid              OrderStatus = "paid"
	OrderStatusFulfilled         OrderStatus = "fulfilled"
	OrderStatusPartiallyRefunded OrderStatus = "partially_refunded"
	OrderStatusRefunded          OrderStatus = "refunded"
	OrderStatusExpired           OrderStatus = "expired"
	OrderStatusCanceled          OrderStatus = "canceled"
	OrderStatusFailed            OrderStatus = "failed"
)

type ProductType string

const (
	ProductTypeBalanceCredit    ProductType = "balance_credit"
	ProductTypeSubscriptionPlan ProductType = "subscription_plan"
)

type PaymentProviderInstance struct {
	ID               int
	Provider         string
	Name             string
	Status           ProviderStatus
	ConfigCiphertext string
	ConfigVersion    int
	SupportedMethods []string
	Limits           map[string]any
	SortOrder        int
	Metadata         map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

type PaymentMethod struct {
	Method             string
	Provider           string
	ProviderInstanceID int
	Name               string
	Metadata           map[string]any
}

type PaymentOrder struct {
	ID                    int
	UserID                int
	OrderNo               string
	ProviderInstanceID    int
	OriginalAmount        string
	DiscountAmount        string
	PromoCodeID           *int
	Amount                string
	Currency              string
	Status                OrderStatus
	ProductType           ProductType
	ProductID             string
	ProviderTransactionID *string
	ProviderSnapshot      map[string]any
	ExpiresAt             *time.Time
	PaidAt                *time.Time
	ClosedAt              *time.Time
	Metadata              map[string]any
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type PaymentAuditLog struct {
	ID                 int
	OrderID            int
	ProviderInstanceID int
	EventType          string
	IdempotencyKey     string
	Payload            map[string]any
	SignatureValid     bool
	CreatedAt          time.Time
}

type CreateProviderInstanceRequest struct {
	Provider         string
	Name             string
	Status           *ProviderStatus
	Config           map[string]any
	SupportedMethods []string
	Limits           map[string]any
	SortOrder        *int
	Metadata         map[string]any
}

// UpdateProviderInstanceRequest patches mutable payment provider instance fields.
type UpdateProviderInstanceRequest struct {
	Name             *string
	Status           *ProviderStatus
	Config           *map[string]any
	SupportedMethods *[]string
	Limits           *map[string]any
	SortOrder        *int
	Metadata         *map[string]any
}

type CreateStoredProviderInstance struct {
	Provider         string
	Name             string
	Status           ProviderStatus
	ConfigCiphertext string
	ConfigVersion    int
	SupportedMethods []string
	Limits           map[string]any
	SortOrder        int
	Metadata         map[string]any
}

// ProviderInstanceTestResult reports local payment provider configuration health.
type ProviderInstanceTestResult struct {
	ProviderInstance PaymentProviderInstance
	OK               bool
	Status           string
	Message          string
	Checks           map[string]any
}

type CreateOrderRequest struct {
	UserID      int
	Method      string
	Amount      string
	Currency    string
	ProductType ProductType
	ProductID   string
	PromoCode   string
	ExpiresAt   *time.Time
	Metadata    map[string]any
}

type CreateStoredOrder struct {
	UserID             int
	OrderNo            string
	ProviderInstanceID int
	OriginalAmount     string
	DiscountAmount     string
	PromoCodeID        *int
	PromoCode          string
	Amount             string
	Currency           string
	Status             OrderStatus
	ProductType        ProductType
	ProductID          string
	ProviderSnapshot   map[string]any
	ExpiresAt          *time.Time
	Metadata           map[string]any
}

type PromoCodePreviewInput struct {
	UserID   int
	Code     string
	Amount   string
	Currency string
	Now      time.Time
}

type PromoCodeApplication struct {
	ID             int
	UserID         int
	PromoCodeID    int
	PaymentOrderID int
	OrderNo        string
	OriginalAmount string
	DiscountAmount string
	FinalAmount    string
	Currency       string
	DiscountType   string
	AppliedAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type WebhookRequest struct {
	Provider string
	Headers  map[string]string
	Payload  map[string]any
}

type WebhookResult struct {
	Order   PaymentOrder
	Handled bool
}

type RefundRequest struct {
	ActorUserID int
	OrderID     int
	Amount      string
	Reason      string
}

// ExpireOrdersResult reports the outcome of a pending-order expiration pass.
type ExpireOrdersResult struct {
	Selected int
	Expired  int
}

type Store interface {
	CreateProviderInstance(ctx context.Context, input CreateStoredProviderInstance) (PaymentProviderInstance, error)
	ListProviderInstances(ctx context.Context) ([]PaymentProviderInstance, error)
	FindProviderInstanceByID(ctx context.Context, id int) (PaymentProviderInstance, error)
	UpdateProviderInstance(ctx context.Context, input PaymentProviderInstance) (PaymentProviderInstance, error)
	PreviewPromoCode(ctx context.Context, input PromoCodePreviewInput) (PromoCodeApplication, error)
	CreateOrder(ctx context.Context, input CreateStoredOrder) (PaymentOrder, error)
	UpdateOrder(ctx context.Context, input PaymentOrder) (PaymentOrder, error)
	FindOrderByID(ctx context.Context, id int) (PaymentOrder, error)
	FindOrderByOrderNo(ctx context.Context, orderNo string) (PaymentOrder, error)
	ListOrders(ctx context.Context) ([]PaymentOrder, error)
	ListExpiredPendingOrders(ctx context.Context, now time.Time) ([]PaymentOrder, error)
	ListOrdersByUser(ctx context.Context, userID int) ([]PaymentOrder, error)
	CountInProgressOrdersByProviderInstance(ctx context.Context, providerInstanceID int) (int, error)
	ExpireOrder(ctx context.Context, orderID int, now time.Time) (PaymentOrder, bool, error)
	CreateAuditLog(ctx context.Context, input PaymentAuditLog) (PaymentAuditLog, bool, error)
	ListAuditLogsByOrder(ctx context.Context, orderID int) ([]PaymentAuditLog, error)
}
