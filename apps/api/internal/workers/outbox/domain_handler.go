package outbox

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	affiliateservice "github.com/srapi/srapi/apps/api/internal/modules/affiliate/service"
	auditservice "github.com/srapi/srapi/apps/api/internal/modules/audit/service"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
)

func defaultEventHandler(events *eventsservice.Service, cfg Config) (eventsservice.OutboxHandler, error) {
	if cfg.AffiliateStore == nil {
		return eventsservice.OutboxHandlerFunc(func(context.Context, eventscontract.OutboxEvent) error {
			return nil
		}), nil
	}
	var audit *auditservice.Service
	if cfg.AuditStore != nil {
		var err error
		audit, err = auditservice.New(cfg.AuditStore, nil)
		if err != nil {
			return nil, err
		}
	}
	deps := affiliateservice.Dependencies{Events: events}
	if audit != nil {
		deps.Audit = audit
	}
	affiliate, err := affiliateservice.New(cfg.AffiliateStore, deps, nil)
	if err != nil {
		return nil, err
	}
	return domainEventHandler{affiliate: affiliate}, nil
}

type domainEventHandler struct {
	affiliate *affiliateservice.Service
}

func (h domainEventHandler) HandleOutboxEvent(ctx context.Context, event eventscontract.OutboxEvent) error {
	if h.affiliate == nil {
		return nil
	}
	switch event.EventType {
	case "PaymentOrderPaid":
		_, err := h.affiliate.AccrueRebate(ctx, affiliateAccrualFromEvent(event))
		return err
	case "PaymentOrderRefunded":
		_, err := h.affiliate.CompensateRefund(ctx, affiliateCompensationFromEvent(event))
		return err
	default:
		return nil
	}
}

func affiliateAccrualFromEvent(event eventscontract.OutboxEvent) affiliatecontract.AccrueRebateRequest {
	return affiliatecontract.AccrueRebateRequest{
		OrderID:               payloadInt(event.Payload, "order_id"),
		OrderNo:               payloadString(event.Payload, "order_no"),
		InviteeUserID:         payloadInt(event.Payload, "user_id"),
		Amount:                payloadString(event.Payload, "amount"),
		Currency:              payloadString(event.Payload, "currency"),
		PaidAt:                payloadTime(event.Payload, "paid_at"),
		ProviderTransactionID: payloadString(event.Payload, "provider_transaction_id"),
	}
}

func affiliateCompensationFromEvent(event eventscontract.OutboxEvent) affiliatecontract.CompensateRefundRequest {
	return affiliatecontract.CompensateRefundRequest{
		OrderID:      payloadInt(event.Payload, "order_id"),
		RefundID:     payloadString(event.Payload, "refund_id"),
		UserID:       payloadInt(event.Payload, "user_id"),
		RefundAmount: payloadString(event.Payload, "amount"),
		Currency:     payloadString(event.Payload, "currency"),
		Reason:       payloadString(event.Payload, "refund_reason"),
		RefundedAt:   payloadTime(event.Payload, "refunded_at"),
	}
}

func payloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func payloadInt(payload map[string]any, key string) int {
	value, ok := payload[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		parsed, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
		return parsed
	}
}

func payloadTime(payload map[string]any, key string) time.Time {
	value := payloadString(payload, key)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed.UTC()
	}
	parsed, err = time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}
