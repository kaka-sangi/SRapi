package stripeprovider

import (
	stripe "github.com/stripe/stripe-go/v78"
	stripesession "github.com/stripe/stripe-go/v78/checkout/session"
)

type CheckoutSessionRequest struct {
	APIKey     string
	OrderNo    string
	Amount     int64
	Currency   string
	SuccessURL string
	CancelURL  string
	Metadata   map[string]string
}

type CheckoutSession struct {
	ID  string
	URL string
}

type CheckoutCreator interface {
	CreateCheckoutSession(req CheckoutSessionRequest) (CheckoutSession, error)
}

type Provider struct{}

func New() Provider {
	return Provider{}
}

func (Provider) CreateCheckoutSession(req CheckoutSessionRequest) (CheckoutSession, error) {
	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(req.OrderNo),
		Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL:        stripe.String(req.SuccessURL),
		CancelURL:         stripe.String(req.CancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String(req.Currency),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String("SRapi order " + req.OrderNo),
					},
					UnitAmount: stripe.Int64(req.Amount),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Metadata: req.Metadata,
	}
	session, err := stripesession.Client{Key: req.APIKey, B: stripe.GetBackend(stripe.APIBackend)}.New(params)
	if err != nil {
		return CheckoutSession{}, err
	}
	return CheckoutSession{ID: session.ID, URL: session.URL}, nil
}
