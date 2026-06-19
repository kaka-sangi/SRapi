package alertnotifications

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsmemory "github.com/srapi/srapi/apps/api/internal/modules/operations/store/memory"
)

func TestWorkerDeliversDueEmailNotifications(t *testing.T) {
	store := operationsmemory.New()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	channel, err := store.CreateNotificationChannel(t.Context(), contract.NotificationChannel{
		Name:            "Ops",
		Type:            contract.NotificationChannelTypeEmail,
		Status:          contract.NotificationChannelStatusActive,
		MinSeverity:     contract.AlertSeverityWarning,
		EmailRecipients: []string{"ops@example.com"},
		SendResolved:    true,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	alert, err := store.CreateAlert(t.Context(), contract.AlertEvent{
		RuleID:      "rule.1",
		Severity:    contract.AlertSeverityCritical,
		Status:      contract.AlertStatusFiring,
		Fingerprint: "rule.1",
		Summary:     "Gateway errors high",
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create alert: %v", err)
	}
	if _, err := store.CreateNotificationDelivery(t.Context(), contract.NotificationDelivery{
		ChannelID:     channel.ID,
		AlertEventID:  alert.ID,
		AlertStatus:   contract.AlertStatusFiring,
		Severity:      contract.AlertSeverityCritical,
		Status:        contract.NotificationDeliveryStatusPending,
		Target:        "ops@example.com",
		NextAttemptAt: now.Add(-time.Second),
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	sender := &captureEmailSender{}
	worker, err := New(store, slog.New(slog.DiscardHandler), Config{
		EmailSender:   sender,
		PublicBaseURL: "https://console.srapi.local",
		Clock:         fixedClock{now: now},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	delivered, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if delivered != 1 || len(sender.messages) != 1 {
		t.Fatalf("expected one delivered message, delivered=%d messages=%+v", delivered, sender.messages)
	}
	if sender.messages[0].To != "ops@example.com" || sender.messages[0].Subject == "" || sender.messages[0].HTML == "" {
		t.Fatalf("unexpected email message: %+v", sender.messages[0])
	}
	deliveries, err := store.ListNotificationDeliveries(t.Context(), contract.DeliveryListOptions{})
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Status != contract.NotificationDeliveryStatusDelivered || deliveries[0].DeliveredAt == nil || deliveries[0].AttemptCount != 1 {
		t.Fatalf("expected delivered evidence, got %+v", deliveries)
	}
}

func TestWorkerRecordsFailedEmailNotification(t *testing.T) {
	store := operationsmemory.New()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	channel, _ := store.CreateNotificationChannel(t.Context(), contract.NotificationChannel{
		Name:            "Ops",
		Type:            contract.NotificationChannelTypeEmail,
		Status:          contract.NotificationChannelStatusActive,
		MinSeverity:     contract.AlertSeverityWarning,
		EmailRecipients: []string{"ops@example.com"},
		SendResolved:    true,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	alert, _ := store.CreateAlert(t.Context(), contract.AlertEvent{
		RuleID:      "rule.1",
		Severity:    contract.AlertSeverityCritical,
		Status:      contract.AlertStatusFiring,
		Fingerprint: "rule.1",
		Summary:     "Gateway errors high",
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	_, _ = store.CreateNotificationDelivery(t.Context(), contract.NotificationDelivery{
		ChannelID:     channel.ID,
		AlertEventID:  alert.ID,
		AlertStatus:   contract.AlertStatusFiring,
		Severity:      contract.AlertSeverityCritical,
		Status:        contract.NotificationDeliveryStatusPending,
		Target:        "ops@example.com",
		NextAttemptAt: now.Add(-time.Second),
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	worker, err := New(store, slog.New(slog.DiscardHandler), Config{
		EmailSender: &captureEmailSender{err: errors.New("smtp down")},
		Clock:       fixedClock{now: now},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	delivered, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if delivered != 0 {
		t.Fatalf("expected no delivered messages, got %d", delivered)
	}
	deliveries, err := store.ListNotificationDeliveries(t.Context(), contract.DeliveryListOptions{})
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Status != contract.NotificationDeliveryStatusFailed || deliveries[0].LastError != "smtp down" || deliveries[0].AttemptCount != 1 || !deliveries[0].NextAttemptAt.After(now) {
		t.Fatalf("expected failed retry evidence, got %+v", deliveries)
	}
}

func TestWorkerSkipsDisabledChannelWithoutBlockingBatch(t *testing.T) {
	store := operationsmemory.New()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	disabledChannel, _ := store.CreateNotificationChannel(t.Context(), contract.NotificationChannel{
		Name:            "Disabled",
		Type:            contract.NotificationChannelTypeEmail,
		Status:          contract.NotificationChannelStatusDisabled,
		MinSeverity:     contract.AlertSeverityWarning,
		EmailRecipients: []string{"disabled@example.com"},
		SendResolved:    true,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	activeChannel, _ := store.CreateNotificationChannel(t.Context(), contract.NotificationChannel{
		Name:            "Active",
		Type:            contract.NotificationChannelTypeEmail,
		Status:          contract.NotificationChannelStatusActive,
		MinSeverity:     contract.AlertSeverityWarning,
		EmailRecipients: []string{"active@example.com"},
		SendResolved:    true,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	alert, _ := store.CreateAlert(t.Context(), contract.AlertEvent{
		RuleID:      "rule.1",
		Severity:    contract.AlertSeverityCritical,
		Status:      contract.AlertStatusFiring,
		Fingerprint: "rule.1",
		Summary:     "Gateway errors high",
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	_, _ = store.CreateNotificationDelivery(t.Context(), contract.NotificationDelivery{
		ChannelID:     disabledChannel.ID,
		AlertEventID:  alert.ID,
		AlertStatus:   contract.AlertStatusFiring,
		Severity:      contract.AlertSeverityCritical,
		Status:        contract.NotificationDeliveryStatusPending,
		Target:        "disabled@example.com",
		NextAttemptAt: now.Add(-2 * time.Second),
		CreatedAt:     now.Add(-2 * time.Second),
		UpdatedAt:     now.Add(-2 * time.Second),
	})
	_, _ = store.CreateNotificationDelivery(t.Context(), contract.NotificationDelivery{
		ChannelID:     activeChannel.ID,
		AlertEventID:  alert.ID,
		AlertStatus:   contract.AlertStatusFiring,
		Severity:      contract.AlertSeverityCritical,
		Status:        contract.NotificationDeliveryStatusPending,
		Target:        "active@example.com",
		NextAttemptAt: now.Add(-time.Second),
		CreatedAt:     now.Add(-time.Second),
		UpdatedAt:     now.Add(-time.Second),
	})
	sender := &captureEmailSender{}
	worker, err := New(store, slog.New(slog.DiscardHandler), Config{
		BatchLimit:  1,
		EmailSender: sender,
		Clock:       fixedClock{now: now},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	delivered, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if delivered != 1 || len(sender.messages) != 1 || sender.messages[0].To != "active@example.com" {
		t.Fatalf("expected active delivery to bypass disabled channel, delivered=%d messages=%+v", delivered, sender.messages)
	}
}

type captureEmailSender struct {
	messages []notificationscontract.EmailMessage
	err      error
}

func (s *captureEmailSender) Send(_ context.Context, message notificationscontract.EmailMessage) error {
	if s.err != nil {
		return s.err
	}
	s.messages = append(s.messages, message)
	return nil
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }
