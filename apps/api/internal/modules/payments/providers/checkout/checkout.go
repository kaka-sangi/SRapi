package checkout

import "errors"

var (
	ErrInvalidConfig = errors.New("invalid payment checkout provider config")
	ErrUnavailable   = errors.New("payment checkout provider unavailable")
)

type Request struct {
	Provider      string
	Config        map[string]any
	OrderNo       string
	UserID        int
	Amount        string
	Currency      string
	Product       Product
	PayerOpenID   string
	PayerClientIP string
	Metadata      map[string]any
}

type Product struct {
	Type string
	ID   string
}

type Session struct {
	ID       string
	URL      string
	Metadata map[string]any
}

type RefundRequest struct {
	Provider              string
	Config                map[string]any
	OrderNo               string
	ProviderTransactionID string
	Amount                string
	OriginalAmount        string
	Currency              string
	Reason                string
	IdempotencyKey        string
	Metadata              map[string]any
}

type RefundStatus string

const (
	RefundStatusSucceeded  RefundStatus = "succeeded"
	RefundStatusProcessing RefundStatus = "processing"
	RefundStatusFailed     RefundStatus = "failed"
)

type RefundResult struct {
	ID       string
	Status   RefundStatus
	Metadata map[string]any
}

type QueryRequest struct {
	Provider              string
	Config                map[string]any
	OrderNo               string
	ProviderTransactionID string
	Amount                string
	Currency              string
	Metadata              map[string]any
}

type QueryStatus string

const (
	QueryStatusPending  QueryStatus = "pending"
	QueryStatusPaid     QueryStatus = "paid"
	QueryStatusFailed   QueryStatus = "failed"
	QueryStatusCanceled QueryStatus = "canceled"
)

type QueryResult struct {
	Status                QueryStatus
	ProviderTransactionID string
	Amount                string
	Currency              string
	Metadata              map[string]any
}

type Provider interface {
	CreateSession(req Request) (Session, error)
	Refund(req RefundRequest) (RefundResult, error)
	QueryOrder(req QueryRequest) (QueryResult, error)
}

type Registry map[string]Provider
