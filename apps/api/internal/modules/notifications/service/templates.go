package service

import (
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"

	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
)

const (
	maxEmailTemplateSubjectLength = 200
	maxEmailTemplateHTMLLength    = 30000
)

var emailTemplatePlaceholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)

// EmailTemplateEventInfo describes an editable notification email template event.
type EmailTemplateEventInfo struct {
	Event        string
	Label        string
	Description  string
	Category     string
	Optional     bool
	Placeholders []string
}

// EmailTemplateDetail is the resolved admin-facing template body for one event.
type EmailTemplateDetail struct {
	Event        string
	Subject      string
	HTML         string
	IsCustom     bool
	Placeholders []string
}

// EmailTemplateList contains all editable email templates and placeholder metadata.
type EmailTemplateList struct {
	Events       []EmailTemplateEventInfo
	Templates    []EmailTemplateDetail
	Placeholders []string
}

// EmailTemplatePreviewInput renders a template without saving it.
type EmailTemplatePreviewInput struct {
	Event     string
	Subject   string
	HTML      string
	Variables map[string]string
}

// EmailTemplatePreview is a rendered email template preview.
type EmailTemplatePreview struct {
	Subject string
	HTML    string
}

type emailTemplateDefinition struct {
	EmailTemplateEventInfo
	defaultSubject  string
	defaultHTML     string
	sampleVariables map[string]string
	urlPlaceholders map[string]struct{}
}

var emailTemplateDefinitions = []emailTemplateDefinition{
	{
		EmailTemplateEventInfo: EmailTemplateEventInfo{
			Event:       notificationscontract.TemplateAuthPasswordReset,
			Label:       "Password reset",
			Description: "Transactional email sent when a user requests a password reset link.",
			Category:    "auth",
			Optional:    false,
			Placeholders: []string{
				"recipient_name",
				"recipient_email",
				"action_url",
				"expires_at",
			},
		},
		defaultSubject: "Reset your SRapi password",
		defaultHTML:    `<p>Hello {{recipient_name}},</p><p>Use this link to reset your SRapi password:</p><p><a href="{{action_url}}">Reset password</a></p><p>This link expires at {{expires_at}}.</p>`,
		sampleVariables: map[string]string{
			"recipient_name":  "Ada Lovelace",
			"recipient_email": "ada@example.com",
			"action_url":      "https://console.srapi.local/reset-password?token=preview",
			"expires_at":      "2026-05-29T10:30:00Z",
		},
		urlPlaceholders: stringSet("action_url"),
	},
	{
		EmailTemplateEventInfo: EmailTemplateEventInfo{
			Event:       notificationscontract.TemplateAuthEmailVerification,
			Label:       "Email verification",
			Description: "Transactional email sent when a user needs to verify their email address.",
			Category:    "auth",
			Optional:    false,
			Placeholders: []string{
				"recipient_name",
				"recipient_email",
				"action_url",
				"expires_at",
			},
		},
		defaultSubject: "Verify your SRapi email",
		defaultHTML:    `<p>Hello {{recipient_name}},</p><p>Use this link to verify your SRapi email address:</p><p><a href="{{action_url}}">Verify email</a></p><p>This link expires at {{expires_at}}.</p>`,
		sampleVariables: map[string]string{
			"recipient_name":  "Ada Lovelace",
			"recipient_email": "ada@example.com",
			"action_url":      "https://console.srapi.local/verify-email?token=preview",
			"expires_at":      "2026-05-29T10:30:00Z",
		},
		urlPlaceholders: stringSet("action_url"),
	},
	{
		EmailTemplateEventInfo: EmailTemplateEventInfo{
			Event:       notificationscontract.TemplateAuthPendingOAuthEmailCompletion,
			Label:       "OAuth email completion",
			Description: "Transactional email sent when an OAuth sign-in needs the user to prove an email address before account creation or bind-login.",
			Category:    "auth",
			Optional:    false,
			Placeholders: []string{
				"recipient_name",
				"recipient_email",
				"action_url",
				"expires_at",
			},
		},
		defaultSubject: "Complete your SRapi OAuth sign-in",
		defaultHTML:    `<p>Hello {{recipient_name}},</p><p>Use this link to confirm this email address and continue your SRapi OAuth sign-in:</p><p><a href="{{action_url}}">Continue OAuth sign-in</a></p><p>This link expires at {{expires_at}}.</p>`,
		sampleVariables: map[string]string{
			"recipient_name":  "ada@example.com",
			"recipient_email": "ada@example.com",
			"action_url":      "https://console.srapi.local/oauth/pending/email-completion?token=preview",
			"expires_at":      "2026-05-29T10:30:00Z",
		},
		urlPlaceholders: stringSet("action_url"),
	},
	{
		EmailTemplateEventInfo: EmailTemplateEventInfo{
			Event:       notificationscontract.TemplateNotificationContactVerification,
			Label:       "Notification contact verification",
			Description: "Transactional email sent when a user verifies a secondary notification email address.",
			Category:    "notifications",
			Optional:    false,
			Placeholders: []string{
				"recipient_name",
				"recipient_email",
				"action_url",
				"expires_at",
			},
		},
		defaultSubject: "Verify your SRapi notification email",
		defaultHTML:    `<p>Hello {{recipient_name}},</p><p>Use this link to verify this address for SRapi notifications:</p><p><a href="{{action_url}}">Verify notification email</a></p><p>This link expires at {{expires_at}}.</p>`,
		sampleVariables: map[string]string{
			"recipient_name":  "Ada Lovelace",
			"recipient_email": "alerts@example.com",
			"action_url":      "https://console.srapi.local/notification-contacts/verify?token=preview",
			"expires_at":      "2026-05-29T10:30:00Z",
		},
		urlPlaceholders: stringSet("action_url"),
	},
	{
		EmailTemplateEventInfo: EmailTemplateEventInfo{
			Event:       notificationscontract.TemplateBalanceLow,
			Label:       "Low balance alert",
			Description: "Optional alert sent when a user balance drops below the configured threshold.",
			Category:    "billing",
			Optional:    true,
			Placeholders: []string{
				"recipient_name",
				"recipient_email",
				"current_balance",
				"threshold",
				"currency",
				"recharge_url",
				"unsubscribe_url",
			},
		},
		defaultSubject: "Your SRapi balance is low",
		defaultHTML:    `<p>Hello {{recipient_name}},</p><p>Your current SRapi balance is {{current_balance}} {{currency}}, below the alert threshold of {{threshold}} {{currency}}.</p><p><a href="{{recharge_url}}">Recharge balance</a></p><p><a href="{{unsubscribe_url}}">Manage this alert</a></p>`,
		sampleVariables: map[string]string{
			"recipient_name":  "Ada Lovelace",
			"recipient_email": "ada@example.com",
			"current_balance": "4.20",
			"threshold":       "5.00",
			"currency":        "USD",
			"recharge_url":    "https://console.srapi.local/billing",
			"unsubscribe_url": "https://console.srapi.local/api/v1/notifications/unsubscribe?token=preview",
		},
		urlPlaceholders: stringSet("recharge_url", "unsubscribe_url"),
	},
	{
		EmailTemplateEventInfo: EmailTemplateEventInfo{
			Event:       notificationscontract.TemplateSubscriptionExpiry,
			Label:       "Subscription expiry reminder",
			Description: "Optional reminder sent before an active subscription expires.",
			Category:    "subscription",
			Optional:    true,
			Placeholders: []string{
				"recipient_name",
				"recipient_email",
				"subscription_name",
				"days_remaining",
				"expires_at",
				"subscription_url",
				"unsubscribe_url",
			},
		},
		defaultSubject: "Your SRapi subscription expires in {{days_remaining}} day(s)",
		defaultHTML:    `<p>Hello {{recipient_name}},</p><p>Your SRapi subscription <strong>{{subscription_name}}</strong> expires in {{days_remaining}} day(s).</p><p>Expiry time: {{expires_at}}</p><p><a href="{{subscription_url}}">Review subscription</a></p><p><a href="{{unsubscribe_url}}">Manage this reminder</a></p>`,
		sampleVariables: map[string]string{
			"recipient_name":    "Ada Lovelace",
			"recipient_email":   "ada@example.com",
			"subscription_name": "Pro",
			"days_remaining":    "3",
			"expires_at":        "2026-06-01T10:30:00Z",
			"subscription_url":  "https://console.srapi.local/subscriptions",
			"unsubscribe_url":   "https://console.srapi.local/api/v1/notifications/unsubscribe?token=preview",
		},
		urlPlaceholders: stringSet("subscription_url", "unsubscribe_url"),
	},
	{
		EmailTemplateEventInfo: EmailTemplateEventInfo{
			Event:       notificationscontract.TemplateAccountQuotaAlert,
			Label:       "Account quota alert",
			Description: "Optional alert sent when a provider account quota approaches a configured limit.",
			Category:    "operations",
			Optional:    true,
			Placeholders: []string{
				"recipient_name",
				"recipient_email",
				"account_id",
				"account_name",
				"provider",
				"quota_dimension",
				"quota_used",
				"quota_limit",
				"quota_remaining",
				"quota_threshold",
				"quota_remaining_ratio",
				"triggered_at",
				"account_url",
				"unsubscribe_url",
			},
		},
		defaultSubject: "SRapi account quota alert: {{account_name}}",
		defaultHTML:    `<p>Hello {{recipient_name}},</p><p>Provider account {{account_name}} on {{provider}} has {{quota_remaining}} remaining for {{quota_dimension}}. Usage is {{quota_used}} of {{quota_limit}}, crossing the {{quota_threshold}} threshold.</p><p><a href="{{account_url}}">Review account</a></p><p><a href="{{unsubscribe_url}}">Manage this alert</a></p>`,
		sampleVariables: map[string]string{
			"recipient_name":        "Admin",
			"recipient_email":       "admin@example.com",
			"account_id":            "acct_123",
			"account_name":          "primary-openai",
			"provider":              "OpenAI Compatible",
			"quota_dimension":       "monthly_tokens",
			"quota_used":            "950000",
			"quota_limit":           "1000000",
			"quota_remaining":       "50000",
			"quota_threshold":       "95%",
			"quota_remaining_ratio": "5%",
			"triggered_at":          "2026-05-29T10:30:00Z",
			"account_url":           "https://console.srapi.local/admin/accounts/acct_123",
			"unsubscribe_url":       "https://console.srapi.local/api/v1/notifications/unsubscribe?token=preview",
		},
		urlPlaceholders: stringSet("account_url", "unsubscribe_url"),
	},
}

// ListEmailTemplates returns resolved templates, event metadata, and the placeholder union.
func ListEmailTemplates(overrides map[string]string) EmailTemplateList {
	events := make([]EmailTemplateEventInfo, 0, len(emailTemplateDefinitions))
	templates := make([]EmailTemplateDetail, 0, len(emailTemplateDefinitions))
	placeholderSet := map[string]struct{}{}
	for _, def := range emailTemplateDefinitions {
		events = append(events, eventInfoFromDefinition(def))
		templates = append(templates, detailFromDefinition(def, overrides))
		for _, placeholder := range def.Placeholders {
			placeholderSet[placeholder] = struct{}{}
		}
	}
	placeholders := make([]string, 0, len(placeholderSet))
	for placeholder := range placeholderSet {
		placeholders = append(placeholders, placeholder)
	}
	sort.Strings(placeholders)
	return EmailTemplateList{Events: events, Templates: templates, Placeholders: placeholders}
}

// GetEmailTemplate returns the resolved template for one notification event.
func GetEmailTemplate(overrides map[string]string, event string) (EmailTemplateDetail, error) {
	def, ok := emailTemplateDefinitionByEvent(event)
	if !ok {
		return EmailTemplateDetail{}, ErrUnsupportedNotificationEvent
	}
	return detailFromDefinition(def, overrides), nil
}

// UpdateEmailTemplate validates and returns an updated template override map.
func UpdateEmailTemplate(overrides map[string]string, event, subject, htmlBody string) (map[string]string, EmailTemplateDetail, error) {
	def, ok := emailTemplateDefinitionByEvent(event)
	if !ok {
		return nil, EmailTemplateDetail{}, ErrUnsupportedNotificationEvent
	}
	subject, htmlBody, err := validateEmailTemplate(def, subject, htmlBody)
	if err != nil {
		return nil, EmailTemplateDetail{}, err
	}
	updated := copyStringMap(overrides)
	updated[templateSubjectKey(def.Event)] = subject
	updated[templateHTMLKey(def.Event)] = htmlBody
	return updated, detailFromDefinition(def, updated), nil
}

// RestoreEmailTemplate removes one event override and returns the default template.
func RestoreEmailTemplate(overrides map[string]string, event string) (map[string]string, EmailTemplateDetail, error) {
	def, ok := emailTemplateDefinitionByEvent(event)
	if !ok {
		return nil, EmailTemplateDetail{}, ErrUnsupportedNotificationEvent
	}
	updated := copyStringMap(overrides)
	delete(updated, templateSubjectKey(def.Event))
	delete(updated, templateHTMLKey(def.Event))
	return updated, detailFromDefinition(def, updated), nil
}

// PreviewEmailTemplate renders a saved or request-supplied template without changing state.
func PreviewEmailTemplate(overrides map[string]string, input EmailTemplatePreviewInput) (EmailTemplatePreview, error) {
	def, ok := emailTemplateDefinitionByEvent(input.Event)
	if !ok {
		return EmailTemplatePreview{}, ErrUnsupportedNotificationEvent
	}
	detail := detailFromDefinition(def, overrides)
	subject := input.Subject
	if strings.TrimSpace(subject) == "" {
		subject = detail.Subject
	}
	htmlBody := input.HTML
	if strings.TrimSpace(htmlBody) == "" {
		htmlBody = detail.HTML
	}
	subject, htmlBody, err := validateEmailTemplate(def, subject, htmlBody)
	if err != nil {
		return EmailTemplatePreview{}, err
	}
	return renderEmailTemplate(def, subject, htmlBody, mergeTemplateVariables(def.sampleVariables, input.Variables))
}

// RenderEmailTemplate renders a validated template for delivery.
func RenderEmailTemplate(event, subject, htmlBody string, values map[string]string) (EmailTemplatePreview, error) {
	def, ok := emailTemplateDefinitionByEvent(event)
	if !ok {
		return EmailTemplatePreview{}, ErrUnsupportedNotificationEvent
	}
	subject, htmlBody, err := validateEmailTemplate(def, subject, htmlBody)
	if err != nil {
		return EmailTemplatePreview{}, err
	}
	return renderEmailTemplate(def, subject, htmlBody, values)
}

func emailTemplateDefinitionByEvent(event string) (emailTemplateDefinition, bool) {
	event = strings.TrimSpace(event)
	for _, def := range emailTemplateDefinitions {
		if def.Event == event {
			return def, true
		}
	}
	return emailTemplateDefinition{}, false
}

func eventInfoFromDefinition(def emailTemplateDefinition) EmailTemplateEventInfo {
	return EmailTemplateEventInfo{
		Event:        def.Event,
		Label:        def.Label,
		Description:  def.Description,
		Category:     def.Category,
		Optional:     def.Optional,
		Placeholders: append([]string(nil), def.Placeholders...),
	}
}

func detailFromDefinition(def emailTemplateDefinition, overrides map[string]string) EmailTemplateDetail {
	subject, hasSubject := templateOverride(overrides, templateSubjectKey(def.Event))
	htmlBody, hasHTML := templateOverride(overrides, templateHTMLKey(def.Event))
	if subject == "" {
		subject = def.defaultSubject
	}
	if htmlBody == "" {
		htmlBody = def.defaultHTML
	}
	return EmailTemplateDetail{
		Event:        def.Event,
		Subject:      subject,
		HTML:         htmlBody,
		IsCustom:     hasSubject || hasHTML,
		Placeholders: append([]string(nil), def.Placeholders...),
	}
}

func validateEmailTemplate(def emailTemplateDefinition, subject, htmlBody string) (string, string, error) {
	subject = sanitizeHeader(subject)
	htmlBody = strings.TrimSpace(htmlBody)
	if subject == "" || htmlBody == "" {
		return "", "", ErrInvalidInput
	}
	if len(subject) > maxEmailTemplateSubjectLength || len(htmlBody) > maxEmailTemplateHTMLLength {
		return "", "", ErrInvalidInput
	}
	if err := validateTemplatePlaceholders(def, subject); err != nil {
		return "", "", err
	}
	if err := validateTemplatePlaceholders(def, htmlBody); err != nil {
		return "", "", err
	}
	return subject, htmlBody, nil
}

func validateTemplatePlaceholders(def emailTemplateDefinition, value string) error {
	matches := emailTemplatePlaceholderPattern.FindAllStringSubmatchIndex(value, -1)
	allowed := stringSet(def.Placeholders...)
	for _, match := range matches {
		name := value[match[2]:match[3]]
		if _, ok := allowed[name]; !ok {
			return ErrInvalidInput
		}
	}
	if strings.Contains(emailTemplatePlaceholderPattern.ReplaceAllString(value, ""), "{{") ||
		strings.Contains(emailTemplatePlaceholderPattern.ReplaceAllString(value, ""), "}}") {
		return ErrInvalidInput
	}
	return nil
}

func renderEmailTemplate(def emailTemplateDefinition, subject, htmlBody string, values map[string]string) (EmailTemplatePreview, error) {
	renderedSubject := renderTemplateValue(def, subject, values)
	renderedHTML := renderTemplateValue(def, htmlBody, values)
	return EmailTemplatePreview{Subject: sanitizeHeader(renderedSubject), HTML: renderedHTML}, nil
}

func renderTemplateValue(def emailTemplateDefinition, value string, values map[string]string) string {
	return emailTemplatePlaceholderPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := emailTemplatePlaceholderPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return ""
		}
		name := parts[1]
		raw := strings.TrimSpace(values[name])
		if _, ok := def.urlPlaceholders[name]; ok && !safeEmailTemplateURL(raw) {
			raw = ""
		}
		return html.EscapeString(raw)
	})
}

func safeEmailTemplateURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n\t ") {
		return false
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "mailto":
		return true
	default:
		return false
	}
}

func templateOverride(overrides map[string]string, key string) (string, bool) {
	value, ok := overrides[key]
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

func templateSubjectKey(event string) string {
	return event + ".subject"
}

func templateHTMLKey(event string) string {
	return event + ".html"
}

func mergeTemplateVariables(base, overrides map[string]string) map[string]string {
	out := copyStringMap(base)
	for key, value := range overrides {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func copyStringMap(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func stringSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
