package service

import (
	"context"

	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	wechatprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/wechat"
)

func (s *Service) normalizeWechatWebhook(ctx context.Context, req contract.WebhookRequest) (normalizedWebhook, error) {
	raw := payloadString(req.Payload, "raw_body")
	if raw == "" {
		return normalizedWebhook{}, ErrInvalidInput
	}
	instances, err := s.store.ListProviderInstances(ctx)
	if err != nil {
		return normalizedWebhook{}, err
	}
	for _, instance := range instances {
		if instance.Provider != "wechat" || instance.Status != contract.ProviderStatusActive {
			continue
		}
		config, err := s.decryptConfig(instance, instance.ConfigCiphertext)
		if err != nil {
			return normalizedWebhook{}, err
		}
		notification, err := wechatprovider.ParseNotification(raw, req.Headers, config)
		if err != nil {
			continue
		}
		payload := notification.Payload
		payload["amount"] = notification.Amount
		payload["currency"] = notification.Currency
		payload["provider_transaction_id"] = notification.TransactionID
		return normalizedWebhook{
			OrderNo:            notification.OrderNo,
			TransactionID:      notification.TransactionID,
			Status:             normalizeProviderStatus(notification.TradeState),
			IdempotencyKey:     "wechat:" + notification.EventID,
			Payload:            payload,
			SignatureValid:     true,
			ProviderInstanceID: instance.ID,
		}, nil
	}
	return normalizedWebhook{}, ErrSignatureInvalid
}
