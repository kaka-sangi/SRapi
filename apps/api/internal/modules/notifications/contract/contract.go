package contract

import "context"

const (
	EventAuthPasswordResetRequested               = "AuthPasswordResetRequested"
	EventAuthEmailVerificationRequested           = "AuthEmailVerificationRequested"
	EventPendingOAuthEmailCompletionRequested     = "PendingOAuthEmailCompletionRequested"
	EventBalanceLowTriggered                      = "BalanceLowTriggered"
	EventSubscriptionExpiryReminder               = "SubscriptionExpiryReminderTriggered"
	EventAccountQuotaAlertTriggered               = "AccountQuotaAlertTriggered"
	EventNotificationContactVerificationRequested = "NotificationContactVerificationRequested"

	TemplateAuthPasswordReset               = "auth.password_reset"
	TemplateAuthEmailVerification           = "auth.email_verification"
	TemplateAuthPendingOAuthEmailCompletion = "auth.oauth_pending_email_completion"
	TemplateNotificationContactVerification = "notification.contact_verification"
	TemplateBalanceLow                      = "balance.low"
	TemplateSubscriptionExpiry              = "subscription.expiry_reminder"
	TemplateAccountQuotaAlert               = "account.quota_alert"
)

// EmailConfig contains non-secret and secret SMTP settings used by workers.
type EmailConfig struct {
	PublicBaseURL string
	SMTPHost      string
	SMTPPort      int
	SMTPUsername  string
	SMTPPassword  string
	SMTPFrom      string
	SMTPFromName  string
	SMTPUseTLS    bool
}

// EmailSender is the delivery adapter used by notification workers.
type EmailSender interface {
	Send(ctx context.Context, message EmailMessage) error
}

// EmailTemplateProvider returns the current notification template overrides.
type EmailTemplateProvider interface {
	NotificationEmailTemplates(ctx context.Context) map[string]string
}

// EmailMessage is a rendered email ready for delivery.
type EmailMessage struct {
	To      string
	Subject string
	HTML    string
	Headers map[string]string
}
