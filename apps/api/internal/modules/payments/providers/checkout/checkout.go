package checkout

import "errors"

var (
	ErrInvalidConfig = errors.New("invalid payment checkout provider config")
	ErrUnavailable   = errors.New("payment checkout provider unavailable")
)

type Request struct {
	Provider string
	Config   map[string]any
	OrderNo  string
	UserID   int
	Amount   string
	Currency string
	Product  Product
	Metadata map[string]any
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

type Provider interface {
	CreateSession(req Request) (Session, error)
}

type Registry map[string]Provider
