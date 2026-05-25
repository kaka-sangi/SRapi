package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	stripe "github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/webhook"

	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	stripeprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/stripe"
)

func (s *Service) normalizeStripeWebhook(ctx context.Context, req contract.WebhookRequest) (normalizedWebhook, error) {
	raw := payloadString(req.Payload, "raw_body")
	signature := strings.TrimSpace(req.Headers["Stripe-Signature"])
	if raw == "" || signature == "" {
		return normalizedWebhook{}, ErrInvalidInput
	}
	instances, err := s.store.ListProviderInstances(ctx)
	if err != nil {
		return normalizedWebhook{}, err
	}
	var (
		event      stripe.Event
		instanceID int
		verified   bool
	)
	for _, instance := range instances {
		if instance.Provider != "stripe" || instance.Status != contract.ProviderStatusActive || instance.DeletedAt != nil {
			continue
		}
		config, err := s.decryptConfig(instance, instance.ConfigCiphertext)
		if err != nil {
			return normalizedWebhook{}, err
		}
		secret := payloadString(config, "webhook_secret", "signing_secret")
		if secret == "" {
			continue
		}
		event, err = webhook.ConstructEventWithOptions([]byte(raw), signature, secret, webhook.ConstructEventOptions{
			IgnoreAPIVersionMismatch: true,
		})
		if err == nil {
			instanceID = instance.ID
			verified = true
			break
		}
	}
	if !verified {
		return normalizedWebhook{}, ErrSignatureInvalid
	}
	if event.Type != stripe.EventTypeCheckoutSessionCompleted {
		return normalizedWebhook{
			Status:             "",
			IdempotencyKey:     event.ID,
			Payload:            map[string]any{"event_id": event.ID, "event_type": string(event.Type)},
			SignatureValid:     true,
			ProviderInstanceID: instanceID,
		}, nil
	}
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return normalizedWebhook{}, ErrInvalidInput
	}
	orderNo := strings.TrimSpace(session.ClientReferenceID)
	if orderNo == "" && session.Metadata != nil {
		orderNo = strings.TrimSpace(session.Metadata["order_no"])
	}
	amount := stripeAmount(session.AmountTotal, string(session.Currency))
	payload := map[string]any{
		"event_id":                event.ID,
		"event_type":              string(event.Type),
		"order_no":                orderNo,
		"amount":                  amount,
		"currency":                strings.ToUpper(string(session.Currency)),
		"status":                  "paid",
		"provider_transaction_id": session.ID,
	}
	return normalizedWebhook{
		OrderNo:            orderNo,
		TransactionID:      session.ID,
		Status:             "paid",
		IdempotencyKey:     event.ID,
		Payload:            payload,
		SignatureValid:     true,
		ProviderInstanceID: instanceID,
	}, nil
}

func stripeAmount(amount int64, currency string) string {
	scale := int64(100)
	if stripeprovider.ZeroDecimalCurrency(currency) {
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
