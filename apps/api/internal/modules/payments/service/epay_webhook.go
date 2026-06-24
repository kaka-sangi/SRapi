package service

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
)

func (s *Service) normalizeEpayWebhook(ctx context.Context, req contract.WebhookRequest) (normalizedWebhook, error) {
	orderNo := payloadString(req.Payload, "out_trade_no", "order_no")
	if orderNo == "" {
		return normalizedWebhook{}, ErrInvalidInput
	}

	order, err := s.store.FindOrderByOrderNo(ctx, orderNo)
	if err != nil {
		return normalizedWebhook{
			OrderNo:       orderNo,
			TransactionID: payloadString(req.Payload, "trade_no", "transaction_id"),
			Status:        normalizeProviderStatus(payloadString(req.Payload, "trade_status", "status")),
			Payload:       req.Payload,
		}, nil
	}

	instance, err := s.store.FindProviderInstanceByID(ctx, order.ProviderInstanceID)
	if err != nil {
		return normalizedWebhook{}, err
	}
	if instance.Provider != "linuxdo" && instance.Provider != "easypay" {
		return normalizedWebhook{}, ErrOrderMismatch
	}

	config, err := s.decryptConfig(instance, instance.ConfigCiphertext)
	if err != nil {
		return normalizedWebhook{}, err
	}

	signingSecret := payloadString(config, "client_secret", "key", "signing_secret", "secret")
	signatureValid := false
	if signingSecret != "" {
		signatureValid = verifyEpaySign(req.Payload, signingSecret)
	}

	idempotencyKey := payloadString(req.Payload, "idempotency_key", "event_id")
	if idempotencyKey == "" {
		tradeNo := payloadString(req.Payload, "trade_no", "transaction_id")
		idempotencyKey = fmt.Sprintf("epay:%s:%s", orderNo, tradeNo)
	}

	return normalizedWebhook{
		OrderNo:            orderNo,
		TransactionID:      payloadString(req.Payload, "trade_no", "transaction_id"),
		Status:             normalizeProviderStatus(payloadString(req.Payload, "trade_status", "status")),
		IdempotencyKey:     idempotencyKey,
		Payload:            req.Payload,
		SignatureValid:     signatureValid,
		ProviderInstanceID: instance.ID,
	}, nil
}

func verifyEpaySign(payload map[string]any, secret string) bool {
	receivedSign := payloadString(payload, "sign")
	if receivedSign == "" {
		return false
	}

	signType := strings.ToUpper(payloadString(payload, "sign_type"))
	if signType == "" {
		signType = "MD5"
	}

	keys := make([]string, 0, len(payload))
	for key := range payload {
		if key == "sign" || key == "sign_type" {
			continue
		}
		val := strings.TrimSpace(fmt.Sprint(payload[key]))
		if val == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+fmt.Sprint(payload[key]))
	}
	canonical := strings.Join(parts, "&")

	var computed string
	if signType == "HMAC-SHA256" || signType == "SHA256" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(canonical))
		computed = hex.EncodeToString(mac.Sum(nil))
	} else {
		sum := md5.Sum([]byte(canonical + secret))
		computed = hex.EncodeToString(sum[:])
	}

	return strings.EqualFold(computed, receivedSign)
}
