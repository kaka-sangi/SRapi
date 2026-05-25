package service

import (
	"context"
	"net/url"
	"strings"

	"github.com/smartwalle/alipay/v3"

	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
)

func (s *Service) normalizeAlipayWebhook(ctx context.Context, req contract.WebhookRequest) (normalizedWebhook, error) {
	values := alipayWebhookValues(req.Payload)
	orderNo := strings.TrimSpace(values.Get("out_trade_no"))
	if orderNo == "" {
		return normalizedWebhook{}, ErrInvalidInput
	}
	order, err := s.store.FindOrderByOrderNo(ctx, orderNo)
	if err != nil {
		return normalizedWebhook{}, err
	}
	instance, err := s.store.FindProviderInstanceByID(ctx, order.ProviderInstanceID)
	if err != nil {
		return normalizedWebhook{}, err
	}
	if instance.Provider != "alipay" || instance.Status != contract.ProviderStatusActive || instance.DeletedAt != nil {
		return normalizedWebhook{}, ErrOrderMismatch
	}
	config, err := s.decryptConfig(instance, instance.ConfigCiphertext)
	if err != nil {
		return normalizedWebhook{}, err
	}
	client, err := alipayClient(config)
	if err != nil {
		return normalizedWebhook{}, err
	}
	notification, err := client.DecodeNotification(ctx, values)
	if err != nil {
		return normalizedWebhook{}, ErrSignatureInvalid
	}
	payload := map[string]any{
		"notify_id":               notification.NotifyId,
		"notify_type":             notification.NotifyType,
		"out_trade_no":            notification.OutTradeNo,
		"trade_no":                notification.TradeNo,
		"trade_status":            string(notification.TradeStatus),
		"amount":                  notification.TotalAmount,
		"currency":                "CNY",
		"provider_transaction_id": notification.TradeNo,
	}
	return normalizedWebhook{
		OrderNo:            notification.OutTradeNo,
		TransactionID:      notification.TradeNo,
		Status:             normalizeProviderStatus(string(notification.TradeStatus)),
		IdempotencyKey:     "alipay:" + notification.NotifyId,
		Payload:            payload,
		SignatureValid:     true,
		ProviderInstanceID: instance.ID,
	}, nil
}

func alipayClient(config map[string]any) (*alipay.Client, error) {
	opts := []alipay.OptionFunc{}
	production := payloadBool(config, "production", "prod")
	gatewayURL := payloadString(config, "gateway_url", "api_gateway")
	if gatewayURL != "" {
		if production {
			opts = append(opts, alipay.WithProductionGateway(gatewayURL))
		} else {
			opts = append(opts, alipay.WithSandboxGateway(gatewayURL))
		}
	}
	client, err := alipay.New(payloadString(config, "app_id", "appId"), payloadString(config, "private_key", "app_private_key"), production, opts...)
	if err != nil {
		return nil, err
	}
	if publicKey := payloadString(config, "alipay_public_key", "public_key"); publicKey != "" {
		if err := client.LoadAliPayPublicKey(publicKey); err != nil {
			return nil, err
		}
	}
	return client, nil
}

func alipayWebhookValues(payload map[string]any) url.Values {
	values := url.Values{}
	for key, value := range payload {
		key = strings.TrimSpace(key)
		if key == "" || value == nil {
			continue
		}
		values.Set(key, payloadString(map[string]any{key: value}, key))
	}
	return values
}

func payloadBool(payload map[string]any, keys ...string) bool {
	raw := strings.ToLower(payloadString(payload, keys...))
	return raw == "true" || raw == "1" || raw == "yes" || raw == "production"
}
