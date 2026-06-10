package outbox

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	affiliateservice "github.com/srapi/srapi/apps/api/internal/modules/affiliate/service"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	auditservice "github.com/srapi/srapi/apps/api/internal/modules/audit/service"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	notificationsservice "github.com/srapi/srapi/apps/api/internal/modules/notifications/service"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	subscriptionservice "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	usageservice "github.com/srapi/srapi/apps/api/internal/modules/usage/service"
)

func defaultEventHandler(events *eventsservice.Service, cfg Config) (eventsservice.OutboxHandler, error) {
	var audit *auditservice.Service
	if cfg.AuditStore != nil {
		var err error
		audit, err = auditservice.New(cfg.AuditStore, nil)
		if err != nil {
			return nil, err
		}
	}
	var affiliate *affiliateservice.Service
	if cfg.AffiliateStore != nil {
		deps := affiliateservice.Dependencies{Events: events}
		if audit != nil {
			deps.Audit = audit
		}
		var err error
		affiliate, err = affiliateservice.New(cfg.AffiliateStore, deps, nil)
		if err != nil {
			return nil, err
		}
	}
	var subscriptions *subscriptionservice.Service
	if cfg.SubscriptionStore != nil {
		var err error
		subscriptions, err = subscriptionservice.New(cfg.SubscriptionStore, nil)
		if err != nil {
			return nil, err
		}
	}
	var notifications *notificationsservice.Service
	if cfg.UserStore != nil {
		var err error
		emailConfig := notificationEmailConfig(cfg)
		sender := cfg.EmailSender
		if sender == nil {
			sender = notificationsservice.NewSMTPSender(emailConfig)
		}
		var preferences *notificationsservice.PreferenceService
		var contacts *notificationsservice.ContactService
		if cfg.AdminControlStore != nil {
			preferences, err = notificationsservice.NewPreferenceService(cfg.AdminControlStore, cfg.MasterKey, emailConfig.PublicBaseURL)
			if err != nil {
				return nil, err
			}
			contacts, err = notificationsservice.NewContactService(cfg.AdminControlStore, cfg.MasterKey, emailConfig.PublicBaseURL, nil)
			if err != nil {
				return nil, err
			}
		}
		notifications, err = notificationsservice.NewWithPreferencesAndContacts(cfg.UserStore, sender, emailConfig, cfg.MasterKey, cfg.EmailTemplates, preferences, contacts)
		if err != nil {
			return nil, err
		}
		templateProvider := cfg.EmailTemplateProvider
		if templateProvider == nil && cfg.AdminControlStore != nil {
			templateProvider, err = admincontrolservice.New(cfg.AdminControlStore, nil)
			if err != nil {
				return nil, err
			}
		}
		if templateProvider != nil {
			notifications.SetTemplateProvider(templateProvider)
		}
	}
	var adminControl *admincontrolservice.Service
	if cfg.AdminControlStore != nil {
		var err error
		adminControl, err = admincontrolservice.New(cfg.AdminControlStore, nil)
		if err != nil {
			return nil, err
		}
	}
	var accounts *accountservice.Service
	if cfg.AccountStore != nil {
		var err error
		accounts, err = accountservice.New(cfg.AccountStore, cfg.MasterKey, nil)
		if err != nil {
			return nil, err
		}
	}
	var usage *usageservice.Service
	if cfg.UsageStore != nil {
		var err error
		usage, err = usageservice.New(cfg.UsageStore, nil)
		if err != nil {
			return nil, err
		}
	}
	return domainEventHandler{accounts: accounts, affiliate: affiliate, audit: audit, adminControl: adminControl, subscriptions: subscriptions, notifications: notifications, usage: usage}, nil
}

func notificationEmailConfig(cfg Config) notificationscontract.EmailConfig {
	emailConfig := cfg.EmailConfig
	if emailConfig.PublicBaseURL == "" {
		emailConfig.PublicBaseURL = cfg.EmailPublicBaseURL
	}
	if emailConfig.SMTPHost == "" {
		emailConfig.SMTPHost = cfg.EmailSMTPHost
	}
	if emailConfig.SMTPPort == 0 {
		emailConfig.SMTPPort = cfg.EmailSMTPPort
	}
	if emailConfig.SMTPUsername == "" {
		emailConfig.SMTPUsername = cfg.EmailSMTPUsername
	}
	if emailConfig.SMTPPassword == "" {
		emailConfig.SMTPPassword = cfg.EmailSMTPPassword
	}
	if emailConfig.SMTPFrom == "" {
		emailConfig.SMTPFrom = cfg.EmailSMTPFrom
	}
	if emailConfig.SMTPFromName == "" {
		emailConfig.SMTPFromName = cfg.EmailSMTPFromName
	}
	if !emailConfig.SMTPUseTLS {
		emailConfig.SMTPUseTLS = cfg.EmailSMTPUseTLS
	}
	return emailConfig
}

type domainEventHandler struct {
	accounts      *accountservice.Service
	affiliate     *affiliateservice.Service
	audit         *auditservice.Service
	adminControl  *admincontrolservice.Service
	notifications *notificationsservice.Service
	subscriptions *subscriptionservice.Service
	usage         *usageservice.Service
}

func (h domainEventHandler) HandleOutboxEvent(ctx context.Context, event eventscontract.OutboxEvent) error {
	switch event.EventType {
	case "GatewayAccountSnapshotRefreshRequested":
		return h.refreshGatewayAccountSnapshot(ctx, event)
	case "PaymentOrderPaid":
		if err := h.activateSubscription(ctx, event); err != nil {
			return err
		}
		if h.affiliate == nil {
			return nil
		}
		if !h.invitationRebateEnabled(ctx) {
			return h.recordInvitationRebateSkipped(ctx, event)
		}
		_, err := h.affiliate.AccrueRebate(ctx, affiliateAccrualFromEvent(event))
		return err
	case "PaymentOrderRefunded":
		if h.affiliate == nil {
			return nil
		}
		_, err := h.affiliate.CompensateRefund(ctx, affiliateCompensationFromEvent(event))
		return err
	case notificationscontract.EventAuthPasswordResetRequested, notificationscontract.EventAuthEmailVerificationRequested, notificationscontract.EventPendingOAuthEmailCompletionRequested, notificationscontract.EventNotificationContactVerificationRequested, notificationscontract.EventBalanceLowTriggered, notificationscontract.EventSubscriptionExpiryReminder, notificationscontract.EventAccountQuotaAlertTriggered:
		if h.notifications == nil {
			return notificationsservice.ErrNotConfigured
		}
		return h.notifications.HandleOutboxEvent(ctx, event)
	default:
		return nil
	}
}

func (h domainEventHandler) invitationRebateEnabled(ctx context.Context) bool {
	if h.adminControl == nil {
		return true
	}
	settings, err := h.adminControl.GetAdminSettings(ctx)
	if err != nil {
		return false
	}
	return settings.Features.InvitationRebateEnabled
}

func (h domainEventHandler) recordInvitationRebateSkipped(ctx context.Context, event eventscontract.OutboxEvent) error {
	if h.audit == nil {
		return nil
	}
	_, err := h.audit.Record(ctx, auditcontract.RecordRequest{
		Action:       "affiliate.rebate.skipped",
		ResourceType: "payment_order",
		ResourceID:   payloadString(event.Payload, "order_no"),
		TraceID:      payloadString(event.Payload, "request_id"),
		After: map[string]any{
			"reason":          "invitation_rebate_disabled",
			"order_id":        payloadInt(event.Payload, "order_id"),
			"invitee_user_id": payloadInt(event.Payload, "user_id"),
			"event_id":        event.EventID,
		},
	})
	return err
}

func (h domainEventHandler) activateSubscription(ctx context.Context, event eventscontract.OutboxEvent) error {
	if h.subscriptions == nil || payloadString(event.Payload, "product_type") != "subscription_plan" {
		return nil
	}
	planID := payloadInt(event.Payload, "product_id")
	if planID <= 0 {
		return subscriptionservice.ErrInvalidInput
	}
	_, err := h.subscriptions.CreateUserSubscription(ctx, subscriptioncontract.CreateSubscriptionRequest{
		UserID:     payloadInt(event.Payload, "user_id"),
		PlanID:     planID,
		SourceType: "payment_order",
		SourceID:   payloadString(event.Payload, "order_no"),
	})
	return err
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
