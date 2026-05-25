package stripeprovider

import (
	"errors"
	"math/big"
	"net/url"
	"strconv"
	"strings"

	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
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

type Provider struct {
	Creator CheckoutCreator
}

func New() Provider {
	return Provider{}
}

func (p Provider) CreateSession(req checkoutprovider.Request) (checkoutprovider.Session, error) {
	secretKey := configString(req.Config, "secret_key", "api_key")
	successURL := checkoutURL(req.Config, "success_url", req.OrderNo)
	cancelURL := checkoutURL(req.Config, "cancel_url", req.OrderNo)
	if secretKey == "" || successURL == "" || cancelURL == "" {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}
	amount, ok := MinorAmount(req.Amount, req.Currency)
	if !ok {
		return checkoutprovider.Session{}, checkoutprovider.ErrInvalidConfig
	}
	session, err := p.createCheckoutSession(CheckoutSessionRequest{
		APIKey:     secretKey,
		OrderNo:    req.OrderNo,
		Amount:     amount,
		Currency:   strings.ToLower(req.Currency),
		SuccessURL: successURL,
		CancelURL:  cancelURL,
		Metadata: map[string]string{
			"order_no":     req.OrderNo,
			"user_id":      strconv.Itoa(req.UserID),
			"product_type": req.Product.Type,
			"product_id":   req.Product.ID,
		},
	})
	if err != nil {
		return checkoutprovider.Session{}, errors.Join(checkoutprovider.ErrUnavailable, err)
	}
	return checkoutprovider.Session{
		ID:  session.ID,
		URL: session.URL,
		Metadata: map[string]any{
			"stripe_checkout_session_id": session.ID,
			"stripe_checkout_url":        session.URL,
		},
	}, nil
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

func (p Provider) createCheckoutSession(req CheckoutSessionRequest) (CheckoutSession, error) {
	if p.Creator != nil {
		return p.Creator.CreateCheckoutSession(req)
	}
	return p.CreateCheckoutSession(req)
}

func MinorAmount(amount string, currency string) (int64, bool) {
	rat, ok := decimalRat(amount)
	if !ok || rat.Sign() < 0 {
		return 0, false
	}
	scale := int64(100)
	if ZeroDecimalCurrency(currency) {
		scale = 1
	}
	rat.Mul(rat, new(big.Rat).SetInt64(scale))
	if !rat.IsInt() {
		return 0, false
	}
	value := rat.Num()
	if !value.IsInt64() || value.Sign() < 0 {
		return 0, false
	}
	return value.Int64(), true
}

func checkoutURL(config map[string]any, key string, orderNo string) string {
	raw := configString(config, key)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	query := parsed.Query()
	if query.Get("order_no") == "" {
		query.Set("order_no", orderNo)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func configString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		return strings.TrimSpace(stripeValueString(value))
	}
	return ""
}

func stripeValueString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

func decimalRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "eE") {
		return nil, false
	}
	rat := new(big.Rat)
	if _, ok := rat.SetString(value); !ok {
		return nil, false
	}
	return rat, true
}

func ZeroDecimalCurrency(currency string) bool {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "BIF", "CLP", "DJF", "GNF", "JPY", "KMF", "KRW", "MGA", "PYG", "RWF", "UGX", "VND", "VUV", "XAF", "XOF", "XPF":
		return true
	default:
		return false
	}
}
