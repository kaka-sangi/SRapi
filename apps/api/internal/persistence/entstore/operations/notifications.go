package operations

import (
	"context"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/srapi/srapi/apps/api/ent"
	entobsnotificationchannel "github.com/srapi/srapi/apps/api/ent/obsnotificationchannel"
	entobsnotificationdelivery "github.com/srapi/srapi/apps/api/ent/obsnotificationdelivery"
	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
)

func (s *Store) CreateNotificationChannel(ctx context.Context, input contract.NotificationChannel) (contract.NotificationChannel, error) {
	create := s.client.ObsNotificationChannel.Create().
		SetName(input.Name).
		SetChannelType(string(input.Type)).
		SetStatus(string(input.Status)).
		SetMinSeverity(string(input.MinSeverity)).
		SetConfigJSON(notificationChannelConfigJSON(input)).
		SetSendResolved(input.SendResolved)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt)
	}
	if !input.UpdatedAt.IsZero() {
		create.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return contract.NotificationChannel{}, err
	}
	return toNotificationChannel(row), nil
}

func (s *Store) UpdateNotificationChannel(ctx context.Context, input contract.NotificationChannel) (contract.NotificationChannel, error) {
	update := s.client.ObsNotificationChannel.UpdateOneID(input.ID).
		SetName(input.Name).
		SetChannelType(string(input.Type)).
		SetStatus(string(input.Status)).
		SetMinSeverity(string(input.MinSeverity)).
		SetConfigJSON(notificationChannelConfigJSON(input)).
		SetSendResolved(input.SendResolved)
	if !input.UpdatedAt.IsZero() {
		update.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := update.Save(ctx)
	if err != nil {
		return contract.NotificationChannel{}, mapNotFound(err)
	}
	return toNotificationChannel(row), nil
}

func (s *Store) FindNotificationChannelByID(ctx context.Context, id int) (contract.NotificationChannel, error) {
	row, err := s.client.ObsNotificationChannel.Get(ctx, id)
	if err != nil {
		return contract.NotificationChannel{}, mapNotFound(err)
	}
	return toNotificationChannel(row), nil
}

func (s *Store) ListNotificationChannels(ctx context.Context) ([]contract.NotificationChannel, error) {
	rows, err := s.client.ObsNotificationChannel.Query().
		Order(entobsnotificationchannel.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.NotificationChannel, 0, len(rows))
	for _, row := range rows {
		out = append(out, toNotificationChannel(row))
	}
	return out, nil
}

func (s *Store) DeleteNotificationChannel(ctx context.Context, id int) error {
	if err := s.client.ObsNotificationChannel.DeleteOneID(id).Exec(ctx); err != nil {
		return mapNotFound(err)
	}
	return nil
}

func (s *Store) CreateNotificationDelivery(ctx context.Context, input contract.NotificationDelivery) (contract.NotificationDelivery, error) {
	create := s.client.ObsNotificationDelivery.Create().
		SetChannelID(input.ChannelID).
		SetAlertEventID(input.AlertEventID).
		SetAlertStatus(string(input.AlertStatus)).
		SetSeverity(string(input.Severity)).
		SetStatus(string(input.Status)).
		SetTarget(input.Target).
		SetAttemptCount(input.AttemptCount).
		SetLastError(input.LastError).
		SetNextAttemptAt(input.NextAttemptAt).
		SetNillableDeliveredAt(input.DeliveredAt).
		SetNillableLastAttemptAt(input.LastAttemptAt)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt)
	}
	if !input.UpdatedAt.IsZero() {
		create.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return contract.NotificationDelivery{}, err
	}
	return s.hydrateNotificationDelivery(ctx, toNotificationDelivery(row))
}

func (s *Store) UpdateNotificationDelivery(ctx context.Context, input contract.NotificationDelivery) (contract.NotificationDelivery, error) {
	update := s.client.ObsNotificationDelivery.UpdateOneID(input.ID).
		SetChannelID(input.ChannelID).
		SetAlertEventID(input.AlertEventID).
		SetAlertStatus(string(input.AlertStatus)).
		SetSeverity(string(input.Severity)).
		SetStatus(string(input.Status)).
		SetTarget(input.Target).
		SetAttemptCount(input.AttemptCount).
		SetLastError(input.LastError).
		SetNextAttemptAt(input.NextAttemptAt).
		SetNillableDeliveredAt(input.DeliveredAt).
		SetNillableLastAttemptAt(input.LastAttemptAt)
	if !input.UpdatedAt.IsZero() {
		update.SetUpdatedAt(input.UpdatedAt)
	}
	row, err := update.Save(ctx)
	if err != nil {
		return contract.NotificationDelivery{}, mapNotFound(err)
	}
	return s.hydrateNotificationDelivery(ctx, toNotificationDelivery(row))
}

func (s *Store) ListNotificationDeliveries(ctx context.Context, opts contract.DeliveryListOptions) ([]contract.NotificationDelivery, error) {
	query := s.client.ObsNotificationDelivery.Query()
	if opts.ChannelID > 0 {
		query = query.Where(entobsnotificationdelivery.ChannelIDEQ(opts.ChannelID))
	}
	if opts.Status != "" {
		query = query.Where(entobsnotificationdelivery.StatusEQ(string(opts.Status)))
	}
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	rows, err := query.
		Order(entobsnotificationdelivery.ByCreatedAt(entsql.OrderDesc()), entobsnotificationdelivery.ByID(entsql.OrderDesc())).
		Limit(opts.Limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.NotificationDelivery, 0, len(rows))
	for _, row := range rows {
		item, err := s.hydrateNotificationDelivery(ctx, toNotificationDelivery(row))
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) ListDueNotificationDeliveries(ctx context.Context, now time.Time, limit int) ([]contract.DueDelivery, error) {
	if limit <= 0 {
		limit = 20
	}
	channels, err := s.client.ObsNotificationChannel.Query().
		Where(entobsnotificationchannel.StatusEQ(string(contract.NotificationChannelStatusActive))).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, nil
	}
	activeChannelIDs := make([]int, 0, len(channels))
	for _, channel := range channels {
		activeChannelIDs = append(activeChannelIDs, channel.ID)
	}
	rows, err := s.client.ObsNotificationDelivery.Query().
		Where(
			entobsnotificationdelivery.ChannelIDIn(activeChannelIDs...),
			entobsnotificationdelivery.Or(
				entobsnotificationdelivery.StatusEQ(string(contract.NotificationDeliveryStatusPending)),
				entobsnotificationdelivery.StatusEQ(string(contract.NotificationDeliveryStatusFailed)),
			),
			entobsnotificationdelivery.NextAttemptAtLTE(now),
		).
		Order(entobsnotificationdelivery.ByNextAttemptAt(), entobsnotificationdelivery.ByID()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.DueDelivery, 0, len(rows))
	for _, row := range rows {
		delivery := toNotificationDelivery(row)
		channel, err := s.FindNotificationChannelByID(ctx, delivery.ChannelID)
		if err != nil {
			if err == contract.ErrNotFound {
				continue
			}
			return nil, err
		}
		if channel.Status != contract.NotificationChannelStatusActive {
			continue
		}
		alert, err := s.FindAlertByID(ctx, delivery.AlertEventID)
		if err != nil {
			if err == contract.ErrNotFound {
				continue
			}
			return nil, err
		}
		delivery, err = s.hydrateNotificationDelivery(ctx, delivery)
		if err != nil {
			return nil, err
		}
		out = append(out, contract.DueDelivery{Delivery: delivery, Channel: channel, Alert: alert})
	}
	return out, nil
}

func (s *Store) FindNotificationDelivery(ctx context.Context, channelID int, alertEventID int, alertStatus contract.AlertStatus, target string) (contract.NotificationDelivery, error) {
	row, err := s.client.ObsNotificationDelivery.Query().
		Where(
			entobsnotificationdelivery.ChannelIDEQ(channelID),
			entobsnotificationdelivery.AlertEventIDEQ(alertEventID),
			entobsnotificationdelivery.AlertStatusEQ(string(alertStatus)),
			entobsnotificationdelivery.TargetEQ(target),
		).
		Only(ctx)
	if err != nil {
		return contract.NotificationDelivery{}, mapNotFound(err)
	}
	return s.hydrateNotificationDelivery(ctx, toNotificationDelivery(row))
}

func (s *Store) hydrateNotificationDelivery(ctx context.Context, value contract.NotificationDelivery) (contract.NotificationDelivery, error) {
	channel, err := s.client.ObsNotificationChannel.Get(ctx, value.ChannelID)
	if err == nil {
		value.ChannelName = channel.Name
		value.ChannelType = contract.NotificationChannelType(channel.ChannelType)
	} else if !ent.IsNotFound(err) {
		return contract.NotificationDelivery{}, err
	}
	alert, err := s.client.ObsAlertEvent.Get(ctx, value.AlertEventID)
	if err == nil {
		value.AlertSummary = alert.Summary
		value.AlertStartedAt = alert.StartedAt
		value.AlertUpdatedAt = alert.UpdatedAt
	} else if !ent.IsNotFound(err) {
		return contract.NotificationDelivery{}, err
	}
	return value, nil
}

func toNotificationChannel(row *ent.ObsNotificationChannel) contract.NotificationChannel {
	return contract.NotificationChannel{
		ID:              row.ID,
		Name:            row.Name,
		Type:            contract.NotificationChannelType(row.ChannelType),
		Status:          contract.NotificationChannelStatus(row.Status),
		MinSeverity:     contract.AlertSeverity(row.MinSeverity),
		EmailRecipients: stringSliceFromMap(row.ConfigJSON, "email_recipients"),
		SendResolved:    row.SendResolved,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func toNotificationDelivery(row *ent.ObsNotificationDelivery) contract.NotificationDelivery {
	return contract.NotificationDelivery{
		ID:            row.ID,
		ChannelID:     row.ChannelID,
		AlertEventID:  row.AlertEventID,
		AlertStatus:   contract.AlertStatus(row.AlertStatus),
		Severity:      contract.AlertSeverity(row.Severity),
		Status:        contract.NotificationDeliveryStatus(row.Status),
		Target:        row.Target,
		AttemptCount:  row.AttemptCount,
		LastError:     row.LastError,
		NextAttemptAt: row.NextAttemptAt,
		DeliveredAt:   cloneTime(row.DeliveredAt),
		LastAttemptAt: cloneTime(row.LastAttemptAt),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func notificationChannelConfigJSON(channel contract.NotificationChannel) map[string]any {
	return map[string]any{
		"email_recipients": append([]string(nil), channel.EmailRecipients...),
	}
}

func stringSliceFromMap(value map[string]any, key string) []string {
	raw, ok := value[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}
