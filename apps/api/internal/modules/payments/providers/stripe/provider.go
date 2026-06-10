package stripeprovider

import (
	"errors"
	"math/big"
	"net/url"
	"strconv"
	"strings"

	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
	stripe "github.com/stripe/stripe-go/v78"
	stripesession "github.com/stripe/stripe-go/v78/checkout/session"
	stripepaymentintent "github.com/stripe/stripe-go/v78/paymentintent"
	striperefund "github.com/stripe/stripe-go/v78/refund"
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

type RefundRequest struct {
	APIKey          string
	SessionID       string
	PaymentIntentID string
	Amount          int64
	Currency        string
	Reason          string
	IdempotencyKey  string
	Metadata        map[string]string
}

type RefundResult struct {
	ID     string
	Status string
}

type RefundCreator interface {
	CreateRefund(req RefundRequest) (RefundResult, error)
}

type QueryOrderRequest struct {
	APIKey          string
	SessionID       string
	PaymentIntentID string
	OrderNo         string
}

type QueryOrderResult struct {
	Status                string
	ProviderTransactionID string
	Amount                int64
	Currency              string
}

type OrderQuerier interface {
	QueryOrder(req QueryOrderRequest) (QueryOrderResult, error)
}

type Provider struct {
	Creator  CheckoutCreator
	Refunder RefundCreator
	Querier  OrderQuerier
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

func (p Provider) Refund(req checkoutprovider.RefundRequest) (checkoutprovider.RefundResult, error) {
	secretKey := configString(req.Config, "secret_key", "api_key")
	if secretKey == "" {
		return checkoutprovider.RefundResult{}, checkoutprovider.ErrInvalidConfig
	}
	amount, ok := MinorAmount(req.Amount, req.Currency)
	if !ok || amount <= 0 {
		return checkoutprovider.RefundResult{}, checkoutprovider.ErrInvalidConfig
	}
	sessionID, paymentIntentID := stripeTransactionRefs(req.ProviderTransactionID)
	result, err := p.createRefund(RefundRequest{
		APIKey:          secretKey,
		SessionID:       firstNonEmpty(metadataString(req.Metadata, "stripe_checkout_session_id", "checkout_session_id"), sessionID),
		PaymentIntentID: firstNonEmpty(metadataString(req.Metadata, "payment_intent_id", "stripe_payment_intent_id"), paymentIntentID),
		Amount:          amount,
		Currency:        strings.ToLower(req.Currency),
		Reason:          stripeRefundReason(req.Reason),
		IdempotencyKey:  req.IdempotencyKey,
		Metadata:        map[string]string{"order_no": req.OrderNo},
	})
	if err != nil {
		return checkoutprovider.RefundResult{}, errors.Join(checkoutprovider.ErrUnavailable, err)
	}
	status := checkoutprovider.RefundStatusProcessing
	if result.Status == string(stripe.RefundStatusSucceeded) {
		status = checkoutprovider.RefundStatusSucceeded
	}
	if result.Status == string(stripe.RefundStatusFailed) || result.Status == string(stripe.RefundStatusCanceled) {
		status = checkoutprovider.RefundStatusFailed
	}
	return checkoutprovider.RefundResult{ID: result.ID, Status: status, Metadata: map[string]any{"stripe_refund_status": result.Status}}, nil
}

func (p Provider) QueryOrder(req checkoutprovider.QueryRequest) (checkoutprovider.QueryResult, error) {
	secretKey := configString(req.Config, "secret_key", "api_key")
	if secretKey == "" {
		return checkoutprovider.QueryResult{}, checkoutprovider.ErrInvalidConfig
	}
	sessionID, paymentIntentID := stripeTransactionRefs(req.ProviderTransactionID)
	result, err := p.queryOrder(QueryOrderRequest{
		APIKey:          secretKey,
		SessionID:       firstNonEmpty(metadataString(req.Metadata, "stripe_checkout_session_id", "checkout_session_id"), sessionID),
		PaymentIntentID: firstNonEmpty(metadataString(req.Metadata, "payment_intent_id", "stripe_payment_intent_id"), paymentIntentID),
		OrderNo:         req.OrderNo,
	})
	if err != nil {
		return checkoutprovider.QueryResult{}, errors.Join(checkoutprovider.ErrUnavailable, err)
	}
	status := checkoutprovider.QueryStatusPending
	switch result.Status {
	case string(stripe.PaymentIntentStatusSucceeded), string(stripe.CheckoutSessionStatusComplete):
		status = checkoutprovider.QueryStatusPaid
	case string(stripe.PaymentIntentStatusCanceled), string(stripe.CheckoutSessionStatusExpired):
		status = checkoutprovider.QueryStatusCanceled
	}
	return checkoutprovider.QueryResult{
		Status:                status,
		ProviderTransactionID: firstNonEmpty(result.ProviderTransactionID, req.ProviderTransactionID),
		Amount:                stripeAmount(result.Amount, result.Currency),
		Currency:              strings.ToUpper(result.Currency),
		Metadata:              map[string]any{"stripe_status": result.Status},
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

func (Provider) CreateRefund(req RefundRequest) (RefundResult, error) {
	params := &stripe.RefundParams{
		Amount:        stripe.Int64(req.Amount),
		Currency:      stripe.String(req.Currency),
		Metadata:      req.Metadata,
		PaymentIntent: stripe.String(req.PaymentIntentID),
		Reason:        stripe.String(req.Reason),
	}
	if strings.TrimSpace(req.IdempotencyKey) != "" {
		params.SetIdempotencyKey(req.IdempotencyKey)
	}
	if params.PaymentIntent == nil || *params.PaymentIntent == "" {
		session, err := (&stripesession.Client{Key: req.APIKey, B: stripe.GetBackend(stripe.APIBackend)}).Get(req.SessionID, &stripe.CheckoutSessionParams{})
		if err != nil {
			return RefundResult{}, err
		}
		if session.PaymentIntent == nil || session.PaymentIntent.ID == "" {
			return RefundResult{}, checkoutprovider.ErrInvalidConfig
		}
		params.PaymentIntent = stripe.String(session.PaymentIntent.ID)
	}
	refund, err := (striperefund.Client{Key: req.APIKey, B: stripe.GetBackend(stripe.APIBackend)}).New(params)
	if err != nil {
		return RefundResult{}, err
	}
	return RefundResult{ID: refund.ID, Status: string(refund.Status)}, nil
}

func (Provider) queryStripeOrder(req QueryOrderRequest) (QueryOrderResult, error) {
	if strings.TrimSpace(req.PaymentIntentID) != "" {
		intent, err := (&stripepaymentintent.Client{Key: req.APIKey, B: stripe.GetBackend(stripe.APIBackend)}).Get(req.PaymentIntentID, &stripe.PaymentIntentParams{})
		if err != nil {
			return QueryOrderResult{}, err
		}
		return QueryOrderResult{Status: string(intent.Status), ProviderTransactionID: intent.ID, Amount: intent.AmountReceived, Currency: string(intent.Currency)}, nil
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return QueryOrderResult{}, checkoutprovider.ErrInvalidConfig
	}
	session, err := (&stripesession.Client{Key: req.APIKey, B: stripe.GetBackend(stripe.APIBackend)}).Get(req.SessionID, &stripe.CheckoutSessionParams{})
	if err != nil {
		return QueryOrderResult{}, err
	}
	transactionID := req.SessionID
	amount := session.AmountTotal
	currency := string(session.Currency)
	if session.PaymentIntent != nil && session.PaymentIntent.ID != "" {
		transactionID = session.PaymentIntent.ID
		if session.PaymentIntent.AmountReceived > 0 {
			amount = session.PaymentIntent.AmountReceived
		}
		if session.PaymentIntent.Currency != "" {
			currency = string(session.PaymentIntent.Currency)
		}
	}
	return QueryOrderResult{Status: string(session.Status), ProviderTransactionID: transactionID, Amount: amount, Currency: currency}, nil
}

func (p Provider) createCheckoutSession(req CheckoutSessionRequest) (CheckoutSession, error) {
	if p.Creator != nil {
		return p.Creator.CreateCheckoutSession(req)
	}
	return p.CreateCheckoutSession(req)
}

func (p Provider) createRefund(req RefundRequest) (RefundResult, error) {
	if p.Refunder != nil {
		return p.Refunder.CreateRefund(req)
	}
	return p.CreateRefund(req)
}

func (p Provider) queryOrder(req QueryOrderRequest) (QueryOrderResult, error) {
	if p.Querier != nil {
		return p.Querier.QueryOrder(req)
	}
	return p.queryStripeOrder(req)
}

func MinorAmount(amount string, currency string) (int64, bool) {
	rat, ok := money.RequiredDecimalRat(amount)
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
	return mapString(values, keys...)
}

func metadataString(values map[string]any, keys ...string) string {
	return mapString(values, keys...)
}

func mapString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		return strings.TrimSpace(stripeValueString(value))
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stripeTransactionRefs(value string) (sessionID string, paymentIntentID string) {
	trimmed := strings.TrimSpace(value)
	switch {
	case strings.HasPrefix(trimmed, "cs_"):
		return trimmed, ""
	case strings.HasPrefix(trimmed, "pi_"):
		return "", trimmed
	default:
		return "", ""
	}
}

func stripeRefundReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "duplicate", "fraudulent", "requested_by_customer":
		return strings.ToLower(strings.TrimSpace(reason))
	default:
		return "requested_by_customer"
	}
}

func stripeAmount(amount int64, currency string) string {
	scale := int64(100)
	if ZeroDecimalCurrency(currency) {
		scale = 1
	}
	whole := amount / scale
	fraction := amount % scale
	if scale == 1 {
		return strconv.FormatInt(whole, 10) + ".00000000"
	}
	return strconv.FormatInt(whole, 10) + "." + leftPad(strconv.FormatInt(fraction, 10), 2) + "000000"
}

func leftPad(value string, width int) string {
	for len(value) < width {
		value = "0" + value
	}
	return value
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

func ZeroDecimalCurrency(currency string) bool {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "BIF", "CLP", "DJF", "GNF", "JPY", "KMF", "KRW", "MGA", "PYG", "RWF", "UGX", "VND", "VUV", "XAF", "XOF", "XPF":
		return true
	default:
		return false
	}
}
