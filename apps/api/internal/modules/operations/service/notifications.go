package service

import (
	"context"
	"net/mail"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
)

const (
	defaultNotificationRetryDelay = time.Minute
	maxNotificationRecipients     = 20
)

// ListNotificationChannels returns configured SRapi-native Ops alert channels.
func (s *Service) ListNotificationChannels(ctx context.Context) ([]contract.NotificationChannel, error) {
	if s == nil || s.observabilityStore == nil {
		return nil, ErrInvalidInput
	}
	items, err := s.observabilityStore.ListNotificationChannels(ctx)
	if err != nil {
		return nil, err
	}
	for idx := range items {
		items[idx] = cloneNotificationChannel(items[idx])
	}
	return items, nil
}

// CreateNotificationChannel validates and persists an Ops alert channel.
func (s *Service) CreateNotificationChannel(ctx context.Context, req contract.CreateNotificationChannelRequest) (contract.NotificationChannel, error) {
	if s == nil || s.observabilityStore == nil {
		return contract.NotificationChannel{}, ErrInvalidInput
	}
	now := s.clock.Now()
	status := contract.NotificationChannelStatusActive
	if req.Status != nil {
		status = *req.Status
	}
	sendResolved := true
	if req.SendResolved != nil {
		sendResolved = *req.SendResolved
	}
	channel := contract.NotificationChannel{
		Name:            strings.TrimSpace(req.Name),
		Type:            req.Type,
		Status:          status,
		MinSeverity:     req.MinSeverity,
		EmailRecipients: normalizeEmailRecipients(req.EmailRecipients),
		SendResolved:    sendResolved,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	applyNotificationChannelDefaults(&channel)
	if err := validateNotificationChannel(channel); err != nil {
		return contract.NotificationChannel{}, err
	}
	return s.observabilityStore.CreateNotificationChannel(ctx, channel)
}

// UpdateNotificationChannel applies a partial update to an Ops alert channel.
func (s *Service) UpdateNotificationChannel(ctx context.Context, id int, req contract.UpdateNotificationChannelRequest) (contract.NotificationChannel, error) {
	if s == nil || s.observabilityStore == nil || id <= 0 {
		return contract.NotificationChannel{}, ErrInvalidInput
	}
	current, err := s.observabilityStore.FindNotificationChannelByID(ctx, id)
	if err != nil {
		return contract.NotificationChannel{}, err
	}
	if req.Name != nil {
		current.Name = strings.TrimSpace(*req.Name)
	}
	if req.Status != nil {
		current.Status = *req.Status
	}
	if req.MinSeverity != nil {
		current.MinSeverity = *req.MinSeverity
	}
	if req.EmailRecipients != nil {
		current.EmailRecipients = normalizeEmailRecipients(*req.EmailRecipients)
	}
	if req.SendResolved != nil {
		current.SendResolved = *req.SendResolved
	}
	current.UpdatedAt = s.clock.Now()
	applyNotificationChannelDefaults(&current)
	if err := validateNotificationChannel(current); err != nil {
		return contract.NotificationChannel{}, err
	}
	return s.observabilityStore.UpdateNotificationChannel(ctx, current)
}

// DeleteNotificationChannel removes an Ops alert channel.
func (s *Service) DeleteNotificationChannel(ctx context.Context, id int) error {
	if s == nil || s.observabilityStore == nil || id <= 0 {
		return ErrInvalidInput
	}
	return s.observabilityStore.DeleteNotificationChannel(ctx, id)
}

// ListNotificationDeliveries returns recent Ops alert delivery evidence.
func (s *Service) ListNotificationDeliveries(ctx context.Context, opts contract.DeliveryListOptions) ([]contract.NotificationDelivery, error) {
	if s == nil || s.observabilityStore == nil {
		return nil, ErrInvalidInput
	}
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Limit > 500 {
		opts.Limit = 500
	}
	items, err := s.observabilityStore.ListNotificationDeliveries(ctx, opts)
	if err != nil {
		return nil, err
	}
	for idx := range items {
		items[idx] = cloneNotificationDelivery(items[idx])
	}
	return items, nil
}

// ListDueNotificationDeliveries returns pending or retryable alert deliveries.
func (s *Service) ListDueNotificationDeliveries(ctx context.Context, limit int) ([]contract.DueDelivery, error) {
	if s == nil || s.observabilityStore == nil {
		return nil, ErrInvalidInput
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	items, err := s.observabilityStore.ListDueNotificationDeliveries(ctx, s.clock.Now(), limit)
	if err != nil {
		return nil, err
	}
	for idx := range items {
		items[idx].Delivery = cloneNotificationDelivery(items[idx].Delivery)
		items[idx].Channel = cloneNotificationChannel(items[idx].Channel)
		items[idx].Alert = cloneAlert(items[idx].Alert)
	}
	return items, nil
}

// MarkNotificationDeliveryDelivered records a successful delivery attempt.
func (s *Service) MarkNotificationDeliveryDelivered(ctx context.Context, delivery contract.NotificationDelivery) (contract.NotificationDelivery, error) {
	if s == nil || s.observabilityStore == nil || delivery.ID <= 0 {
		return contract.NotificationDelivery{}, ErrInvalidInput
	}
	now := s.clock.Now()
	delivery.Status = contract.NotificationDeliveryStatusDelivered
	delivery.AttemptCount++
	delivery.LastError = ""
	delivery.LastAttemptAt = &now
	delivery.DeliveredAt = &now
	delivery.UpdatedAt = now
	return s.observabilityStore.UpdateNotificationDelivery(ctx, delivery)
}

// MarkNotificationDeliveryFailed records a failed delivery attempt and schedules
// the next retry.
func (s *Service) MarkNotificationDeliveryFailed(ctx context.Context, delivery contract.NotificationDelivery, errText string) (contract.NotificationDelivery, error) {
	if s == nil || s.observabilityStore == nil || delivery.ID <= 0 {
		return contract.NotificationDelivery{}, ErrInvalidInput
	}
	now := s.clock.Now()
	next := now.Add(defaultNotificationRetryDelay)
	delivery.Status = contract.NotificationDeliveryStatusFailed
	delivery.AttemptCount++
	delivery.LastError = truncateDeliveryError(errText)
	delivery.LastAttemptAt = &now
	delivery.NextAttemptAt = next
	delivery.UpdatedAt = now
	return s.observabilityStore.UpdateNotificationDelivery(ctx, delivery)
}

func (s *Service) enqueueAlertNotifications(ctx context.Context, alert contract.AlertEvent) error {
	if s == nil || s.observabilityStore == nil || alert.ID <= 0 {
		return nil
	}
	if alert.Status != contract.AlertStatusFiring && alert.Status != contract.AlertStatusResolved {
		return nil
	}
	channels, err := s.observabilityStore.ListNotificationChannels(ctx)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	for _, channel := range channels {
		if !channelEligibleForAlert(channel, alert) {
			continue
		}
		for _, target := range deliveryTargets(channel) {
			if _, err := s.observabilityStore.FindNotificationDelivery(ctx, channel.ID, alert.ID, alert.Status, target); err == nil {
				continue
			} else if err != contract.ErrNotFound {
				return err
			}
			delivery := contract.NotificationDelivery{
				ChannelID:     channel.ID,
				AlertEventID:  alert.ID,
				AlertStatus:   alert.Status,
				Severity:      alert.Severity,
				Status:        contract.NotificationDeliveryStatusPending,
				Target:        target,
				NextAttemptAt: now,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			if _, err := s.observabilityStore.CreateNotificationDelivery(ctx, delivery); err != nil {
				return err
			}
		}
	}
	return nil
}

func channelEligibleForAlert(channel contract.NotificationChannel, alert contract.AlertEvent) bool {
	if channel.Status != contract.NotificationChannelStatusActive {
		return false
	}
	if channel.Type != contract.NotificationChannelTypeEmail {
		return false
	}
	if alert.Status == contract.AlertStatusResolved && !channel.SendResolved {
		return false
	}
	return severityRank(alert.Severity) >= severityRank(channel.MinSeverity)
}

func deliveryTargets(channel contract.NotificationChannel) []string {
	if channel.Type != contract.NotificationChannelTypeEmail {
		return nil
	}
	return normalizeEmailRecipients(channel.EmailRecipients)
}

func applyNotificationChannelDefaults(channel *contract.NotificationChannel) {
	if channel.Type == "" {
		channel.Type = contract.NotificationChannelTypeEmail
	}
	if channel.Status == "" {
		channel.Status = contract.NotificationChannelStatusActive
	}
	if channel.MinSeverity == "" {
		channel.MinSeverity = contract.AlertSeverityWarning
	}
}

func validateNotificationChannel(channel contract.NotificationChannel) error {
	if strings.TrimSpace(channel.Name) == "" {
		return ErrInvalidInput
	}
	if channel.Type != contract.NotificationChannelTypeEmail {
		return ErrInvalidInput
	}
	switch channel.Status {
	case contract.NotificationChannelStatusActive, contract.NotificationChannelStatusDisabled:
	default:
		return ErrInvalidInput
	}
	switch channel.MinSeverity {
	case contract.AlertSeverityCritical, contract.AlertSeverityWarning, contract.AlertSeverityTicket:
	default:
		return ErrInvalidInput
	}
	if len(channel.EmailRecipients) == 0 || len(channel.EmailRecipients) > maxNotificationRecipients {
		return ErrInvalidInput
	}
	for _, recipient := range channel.EmailRecipients {
		if _, err := mail.ParseAddress(recipient); err != nil {
			return ErrInvalidInput
		}
	}
	return nil
}

func normalizeEmailRecipients(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		recipient := strings.ToLower(strings.TrimSpace(value))
		if recipient == "" {
			continue
		}
		if _, ok := seen[recipient]; ok {
			continue
		}
		seen[recipient] = struct{}{}
		out = append(out, recipient)
	}
	return out
}

func severityRank(severity contract.AlertSeverity) int {
	switch severity {
	case contract.AlertSeverityCritical:
		return 3
	case contract.AlertSeverityWarning:
		return 2
	case contract.AlertSeverityTicket:
		return 1
	default:
		return 0
	}
}

func truncateDeliveryError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 500 {
		return value
	}
	return value[:500]
}

func cloneNotificationChannel(value contract.NotificationChannel) contract.NotificationChannel {
	value.EmailRecipients = append([]string(nil), value.EmailRecipients...)
	return value
}

func cloneNotificationDelivery(value contract.NotificationDelivery) contract.NotificationDelivery {
	if value.DeliveredAt != nil {
		deliveredAt := *value.DeliveredAt
		value.DeliveredAt = &deliveredAt
	}
	if value.LastAttemptAt != nil {
		lastAttemptAt := *value.LastAttemptAt
		value.LastAttemptAt = &lastAttemptAt
	}
	return value
}
