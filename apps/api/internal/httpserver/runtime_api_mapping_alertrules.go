package httpserver

import (
	"net/http"
	"strconv"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsservice "github.com/srapi/srapi/apps/api/internal/modules/operations/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func toAPIOpsAlertRule(rule operationscontract.AlertRule) apiopenapi.OpsAlertRule {
	return apiopenapi.OpsAlertRule{
		CooldownSeconds: rule.CooldownSeconds,
		CreatedAt:       rule.CreatedAt,
		Enabled:         rule.Enabled,
		Id:              apiopenapi.Id(strconv.Itoa(rule.ID)),
		MetricType:      apiopenapi.OpsAlertMetricType(rule.MetricType),
		MinRequestCount: rule.MinRequestCount,
		Name:            rule.Name,
		Operator:        apiopenapi.OpsAlertOperator(rule.Operator),
		Scope:           toAPIOpsAlertRuleScope(rule.Scope),
		Severity:        apiopenapi.OpsAlertSeverity(rule.Severity),
		Threshold:       float32(rule.Threshold),
		UpdatedAt:       rule.UpdatedAt,
		WindowSeconds:   rule.WindowSeconds,
	}
}

func toAPIOpsAlertRuleScope(scope operationscontract.AlertRuleScope) apiopenapi.OpsAlertRuleScope {
	out := apiopenapi.OpsAlertRuleScope{
		Model:          scope.Model,
		SourceEndpoint: scope.SourceEndpoint,
	}
	if id := optionalIDString(scope.ProviderID); id != nil {
		providerID := apiopenapi.Id(*id)
		out.ProviderId = &providerID
	}
	return out
}

func toAPIOpsAlertSilence(silence operationscontract.AlertSilence) apiopenapi.OpsAlertSilence {
	out := apiopenapi.OpsAlertSilence{
		CreatedAt: silence.CreatedAt,
		EndsAt:    silence.EndsAt,
		Id:        apiopenapi.Id(strconv.Itoa(silence.ID)),
		Matcher:   toAPIOpsAlertSilenceMatcher(silence.Matcher),
		StartsAt:  silence.StartsAt,
		UpdatedAt: silence.UpdatedAt,
	}
	if silence.Comment != "" {
		comment := silence.Comment
		out.Comment = &comment
	}
	if id := optionalIDString(silence.CreatedBy); id != nil {
		createdBy := apiopenapi.Id(*id)
		out.CreatedBy = &createdBy
	}
	return out
}

func toAPIOpsAlertSilenceMatcher(matcher operationscontract.AlertSilenceMatcher) apiopenapi.OpsAlertSilenceMatcher {
	out := apiopenapi.OpsAlertSilenceMatcher{}
	if matcher.RuleID != "" {
		ruleID := matcher.RuleID
		out.RuleId = &ruleID
	}
	if matcher.Severity != "" {
		severity := apiopenapi.OpsAlertSeverity(matcher.Severity)
		out.Severity = &severity
	}
	if matcher.SourceEndpoint != "" {
		sourceEndpoint := matcher.SourceEndpoint
		out.SourceEndpoint = &sourceEndpoint
	}
	if matcher.Model != "" {
		model := matcher.Model
		out.Model = &model
	}
	if id := optionalIDString(matcher.ProviderID); id != nil {
		providerID := apiopenapi.Id(*id)
		out.ProviderId = &providerID
	}
	return out
}

func toAPIOpsNotificationChannel(channel operationscontract.NotificationChannel) apiopenapi.OpsNotificationChannel {
	return apiopenapi.OpsNotificationChannel{
		CreatedAt:       channel.CreatedAt,
		EmailRecipients: toAPIEmailRecipients(channel.EmailRecipients),
		Id:              apiopenapi.Id(strconv.Itoa(channel.ID)),
		MinSeverity:     apiopenapi.OpsAlertSeverity(channel.MinSeverity),
		Name:            channel.Name,
		SendResolved:    channel.SendResolved,
		Status:          apiopenapi.OpsNotificationChannelStatus(channel.Status),
		Type:            apiopenapi.OpsNotificationChannelType(channel.Type),
		UpdatedAt:       channel.UpdatedAt,
	}
}

func toAPIOpsNotificationDelivery(delivery operationscontract.NotificationDelivery) apiopenapi.OpsNotificationDelivery {
	out := apiopenapi.OpsNotificationDelivery{
		AlertEventId:  apiopenapi.Id(strconv.Itoa(delivery.AlertEventID)),
		AlertStatus:   apiopenapi.OpsAlertStatus(delivery.AlertStatus),
		AttemptCount:  delivery.AttemptCount,
		ChannelId:     apiopenapi.Id(strconv.Itoa(delivery.ChannelID)),
		CreatedAt:     delivery.CreatedAt,
		Id:            apiopenapi.Id(strconv.Itoa(delivery.ID)),
		NextAttemptAt: delivery.NextAttemptAt,
		Severity:      apiopenapi.OpsAlertSeverity(delivery.Severity),
		Status:        apiopenapi.OpsNotificationDeliveryStatus(delivery.Status),
		Target:        delivery.Target,
		UpdatedAt:     delivery.UpdatedAt,
	}
	if delivery.AlertSummary != "" {
		alertSummary := delivery.AlertSummary
		out.AlertSummary = &alertSummary
	}
	if !delivery.AlertStartedAt.IsZero() {
		alertStartedAt := delivery.AlertStartedAt
		out.AlertStartedAt = &alertStartedAt
	}
	if !delivery.AlertUpdatedAt.IsZero() {
		alertUpdatedAt := delivery.AlertUpdatedAt
		out.AlertUpdatedAt = &alertUpdatedAt
	}
	if delivery.ChannelName != "" {
		channelName := delivery.ChannelName
		out.ChannelName = &channelName
	}
	if delivery.ChannelType != "" {
		channelType := apiopenapi.OpsNotificationChannelType(delivery.ChannelType)
		out.ChannelType = &channelType
	}
	if delivery.LastError != "" {
		lastError := delivery.LastError
		out.LastError = &lastError
	}
	out.DeliveredAt = delivery.DeliveredAt
	out.LastAttemptAt = delivery.LastAttemptAt
	return out
}

func toCreateAlertRuleRequest(body apiopenapi.CreateOpsAlertRuleRequest) (operationscontract.CreateAlertRuleRequest, error) {
	scope, err := toAlertRuleScope(body.Scope)
	if err != nil {
		return operationscontract.CreateAlertRuleRequest{}, err
	}
	req := operationscontract.CreateAlertRuleRequest{
		Name:            body.Name,
		MetricType:      operationscontract.AlertMetricType(body.MetricType),
		Operator:        operationscontract.AlertOperator(body.Operator),
		Threshold:       float64(body.Threshold),
		Enabled:         body.Enabled,
		WindowSeconds:   intValue(body.WindowSeconds),
		CooldownSeconds: intValue(body.CooldownSeconds),
		MinRequestCount: intValue(body.MinRequestCount),
		Scope:           scope,
	}
	if body.Severity != nil {
		req.Severity = operationscontract.AlertSeverity(*body.Severity)
	}
	return req, nil
}

func toUpdateAlertRuleRequest(body apiopenapi.UpdateOpsAlertRuleRequest) (operationscontract.UpdateAlertRuleRequest, error) {
	req := operationscontract.UpdateAlertRuleRequest{
		Name:            body.Name,
		Enabled:         body.Enabled,
		WindowSeconds:   body.WindowSeconds,
		CooldownSeconds: body.CooldownSeconds,
		MinRequestCount: body.MinRequestCount,
	}
	if body.MetricType != nil {
		metricType := operationscontract.AlertMetricType(*body.MetricType)
		req.MetricType = &metricType
	}
	if body.Operator != nil {
		operator := operationscontract.AlertOperator(*body.Operator)
		req.Operator = &operator
	}
	if body.Threshold != nil {
		threshold := float64(*body.Threshold)
		req.Threshold = &threshold
	}
	if body.Severity != nil {
		severity := operationscontract.AlertSeverity(*body.Severity)
		req.Severity = &severity
	}
	if body.Scope != nil {
		scope, err := toAlertRuleScope(body.Scope)
		if err != nil {
			return operationscontract.UpdateAlertRuleRequest{}, err
		}
		req.Scope = &scope
	}
	return req, nil
}

func toCreateNotificationChannelRequest(body apiopenapi.CreateOpsNotificationChannelRequest) operationscontract.CreateNotificationChannelRequest {
	req := operationscontract.CreateNotificationChannelRequest{
		Name:            body.Name,
		Type:            operationscontract.NotificationChannelType(body.Type),
		EmailRecipients: fromAPIEmailRecipients(body.EmailRecipients),
		SendResolved:    body.SendResolved,
	}
	if body.Status != nil {
		status := operationscontract.NotificationChannelStatus(*body.Status)
		req.Status = &status
	}
	if body.MinSeverity != nil {
		req.MinSeverity = operationscontract.AlertSeverity(*body.MinSeverity)
	}
	return req
}

func toUpdateNotificationChannelRequest(body apiopenapi.UpdateOpsNotificationChannelRequest) operationscontract.UpdateNotificationChannelRequest {
	req := operationscontract.UpdateNotificationChannelRequest{
		Name:         body.Name,
		SendResolved: body.SendResolved,
	}
	if body.Status != nil {
		status := operationscontract.NotificationChannelStatus(*body.Status)
		req.Status = &status
	}
	if body.MinSeverity != nil {
		severity := operationscontract.AlertSeverity(*body.MinSeverity)
		req.MinSeverity = &severity
	}
	if body.EmailRecipients != nil {
		recipients := fromAPIEmailRecipients(*body.EmailRecipients)
		req.EmailRecipients = &recipients
	}
	return req
}

func toCreateAlertSilenceRequest(body apiopenapi.CreateOpsAlertSilenceRequest) (operationscontract.CreateAlertSilenceRequest, error) {
	matcher, err := toAlertSilenceMatcher(body.Matcher)
	if err != nil {
		return operationscontract.CreateAlertSilenceRequest{}, err
	}
	req := operationscontract.CreateAlertSilenceRequest{
		Matcher: matcher,
		EndsAt:  body.EndsAt,
	}
	if body.Comment != nil {
		req.Comment = *body.Comment
	}
	if body.StartsAt != nil {
		req.StartsAt = *body.StartsAt
	}
	return req, nil
}

func notificationDeliveryListOptionsFromRequest(r *http.Request) (operationscontract.DeliveryListOptions, error) {
	opts := operationscontract.DeliveryListOptions{Limit: listLimitFromRequest(r, 100, 500)}
	q := r.URL.Query()
	if raw := strings.TrimSpace(q.Get("channel_id")); raw != "" {
		channelID, err := strconv.Atoi(raw)
		if err != nil || channelID <= 0 {
			return operationscontract.DeliveryListOptions{}, operationsservice.ErrInvalidInput
		}
		opts.ChannelID = channelID
	}
	if raw := strings.TrimSpace(q.Get("status")); raw != "" {
		status := operationscontract.NotificationDeliveryStatus(raw)
		switch status {
		case operationscontract.NotificationDeliveryStatusPending,
			operationscontract.NotificationDeliveryStatusDelivered,
			operationscontract.NotificationDeliveryStatusFailed:
			opts.Status = status
		default:
			return operationscontract.DeliveryListOptions{}, operationsservice.ErrInvalidInput
		}
	}
	return opts, nil
}

func listLimitFromRequest(r *http.Request, defaultLimit int, maxLimit int) int {
	page := 1
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			page = parsed
		}
	}
	pageSize := defaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("page_size")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			pageSize = parsed
		}
	}
	limit := page * pageSize
	if limit < defaultLimit {
		limit = defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func toAPIEmailRecipients(values []string) []openapi_types.Email {
	out := make([]openapi_types.Email, 0, len(values))
	for _, value := range values {
		out = append(out, openapi_types.Email(value))
	}
	return out
}

func fromAPIEmailRecipients(values []openapi_types.Email) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func toAlertRuleScope(value *apiopenapi.OpsAlertRuleScope) (operationscontract.AlertRuleScope, error) {
	if value == nil {
		return operationscontract.AlertRuleScope{}, nil
	}
	scope := operationscontract.AlertRuleScope{
		SourceEndpoint: value.SourceEndpoint,
		Model:          value.Model,
	}
	providerID, err := parseOptionalProviderID(value.ProviderId)
	if err != nil {
		return operationscontract.AlertRuleScope{}, err
	}
	scope.ProviderID = providerID
	return scope, nil
}

func toAlertSilenceMatcher(value *apiopenapi.OpsAlertSilenceMatcher) (operationscontract.AlertSilenceMatcher, error) {
	if value == nil {
		return operationscontract.AlertSilenceMatcher{}, nil
	}
	matcher := operationscontract.AlertSilenceMatcher{}
	if value.RuleId != nil {
		matcher.RuleID = *value.RuleId
	}
	if value.Severity != nil {
		matcher.Severity = operationscontract.AlertSeverity(*value.Severity)
	}
	if value.SourceEndpoint != nil {
		matcher.SourceEndpoint = *value.SourceEndpoint
	}
	if value.Model != nil {
		matcher.Model = *value.Model
	}
	providerID, err := parseOptionalProviderID(value.ProviderId)
	if err != nil {
		return operationscontract.AlertSilenceMatcher{}, err
	}
	matcher.ProviderID = providerID
	return matcher, nil
}

func parseOptionalProviderID(value *apiopenapi.Id) (*int, error) {
	if value == nil {
		return nil, nil
	}
	providerID, err := strconv.Atoi(string(*value))
	if err != nil || providerID <= 0 {
		return nil, operationsservice.ErrInvalidInput
	}
	return &providerID, nil
}
