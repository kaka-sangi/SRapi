package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"strings"

	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
)

var (
	ErrInvalidInput  = errors.New("invalid notification input")
	ErrNotHandled    = errors.New("notification event not handled")
	ErrNotConfigured = errors.New("notification email not configured")
)

type UserStore interface {
	FindByID(ctx context.Context, id int) (userscontract.StoredUser, error)
	List(ctx context.Context, filter userscontract.ListUsersFilter) ([]userscontract.StoredUser, error)
}

type Service struct {
	users            UserStore
	sender           notificationscontract.EmailSender
	config           notificationscontract.EmailConfig
	tokenKey         []byte
	templates        map[string]string
	templateProvider notificationscontract.EmailTemplateProvider
	preferences      *PreferenceService
	contacts         *ContactService
}

func New(users UserStore, sender notificationscontract.EmailSender, cfg notificationscontract.EmailConfig, masterKey string, templates map[string]string) (*Service, error) {
	return NewWithPreferences(users, sender, cfg, masterKey, templates, nil)
}

func NewWithPreferences(users UserStore, sender notificationscontract.EmailSender, cfg notificationscontract.EmailConfig, masterKey string, templates map[string]string, preferences *PreferenceService) (*Service, error) {
	return NewWithPreferencesAndContacts(users, sender, cfg, masterKey, templates, preferences, nil)
}

func NewWithPreferencesAndContacts(users UserStore, sender notificationscontract.EmailSender, cfg notificationscontract.EmailConfig, masterKey string, templates map[string]string, preferences *PreferenceService, contacts *ContactService) (*Service, error) {
	if users == nil || sender == nil {
		return nil, ErrInvalidInput
	}
	key, err := platformcrypto.DeriveAESKey(masterKey)
	if err != nil {
		return nil, ErrInvalidInput
	}
	cfg.PublicBaseURL = strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
	cfg.SMTPHost = strings.TrimSpace(cfg.SMTPHost)
	cfg.SMTPFrom = strings.TrimSpace(cfg.SMTPFrom)
	if cfg.SMTPPort <= 0 {
		cfg.SMTPPort = 587
	}
	return &Service{
		users:       users,
		sender:      sender,
		config:      cfg,
		tokenKey:    key,
		templates:   cloneTemplates(templates),
		preferences: preferences,
		contacts:    contacts,
	}, nil
}

func (s *Service) SetTemplateProvider(provider notificationscontract.EmailTemplateProvider) {
	s.templateProvider = provider
}

func (s *Service) HandleOutboxEvent(ctx context.Context, event eventscontract.OutboxEvent) error {
	switch event.EventType {
	case notificationscontract.EventAuthPasswordResetRequested:
		return s.handleAuthEmail(ctx, event, authEmailSpec{
			Template:       notificationscontract.TemplateAuthPasswordReset,
			TokenKey:       "reset_token_ciphertext",
			TokenVersion:   "v1",
			TokenAAD:       "auth.password_reset:v1",
			URLPathKey:     "reset_url_path",
			DefaultPath:    "/reset-password",
			DefaultSubject: "Reset your SRapi password",
			DefaultHTML:    `<p>Hello {{recipient_name}},</p><p>Use this link to reset your SRapi password:</p><p><a href="{{action_url}}">Reset password</a></p><p>This link expires at {{expires_at}}.</p>`,
		})
	case notificationscontract.EventAuthEmailVerificationRequested:
		spec := authEmailSpec{
			Template:       notificationscontract.TemplateAuthEmailVerification,
			TokenKey:       "verification_token_ciphertext",
			TokenVersion:   "v1",
			TokenAAD:       "auth.email_verification:v1",
			URLPathKey:     "verification_url_path",
			DefaultPath:    "/verify-email",
			DefaultSubject: "Verify your SRapi email",
			DefaultHTML:    `<p>Hello {{recipient_name}},</p><p>Use this link to verify your SRapi email address:</p><p><a href="{{action_url}}">Verify email</a></p><p>This link expires at {{expires_at}}.</p>`,
		}
		if payloadString(event.Payload, "template") == "auth.passwordless_login" {
			spec.Template = "auth.passwordless_login"
			spec.DefaultPath = "/auth/passwordless"
			spec.DefaultSubject = "Sign in to SRapi"
			spec.DefaultHTML = `<p>Hello {{recipient_name}},</p><p>Use this one-time link to sign in to SRapi:</p><p><a href="{{action_url}}">Sign in</a></p><p>This link expires at {{expires_at}}.</p>`
		}
		return s.handleAuthEmail(ctx, event, spec)
	case notificationscontract.EventPendingOAuthEmailCompletionRequested:
		return s.handlePendingOAuthEmailCompletion(ctx, event)
	case notificationscontract.EventNotificationContactVerificationRequested:
		return s.handleNotificationContactVerification(ctx, event)
	case notificationscontract.EventBalanceLowTriggered:
		return s.handleBalanceLow(ctx, event)
	case notificationscontract.EventSubscriptionExpiryReminder:
		return s.handleSubscriptionExpiry(ctx, event)
	case notificationscontract.EventAccountQuotaAlertTriggered:
		return s.handleAccountQuotaAlert(ctx, event)
	default:
		return ErrNotHandled
	}
}

func (s *Service) handleNotificationContactVerification(ctx context.Context, event eventscontract.OutboxEvent) error {
	if !s.configured() {
		return ErrNotConfigured
	}
	userID := payloadInt(event.Payload, "recipient_user_id")
	if userID <= 0 {
		return ErrInvalidInput
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Status != userscontract.StatusActive {
		return nil
	}
	contactEmail, err := s.decryptContactSecret(payloadString(event.Payload, "contact_email_ciphertext"), NotificationContactEmailAAD())
	if err != nil {
		return err
	}
	if emailHash(contactEmail) != payloadString(event.Payload, "recipient_email_hash") {
		return nil
	}
	token, err := s.decryptContactSecret(payloadString(event.Payload, "verification_token_ciphertext"), NotificationContactTokenAAD())
	if err != nil {
		return err
	}
	actionURL, err := s.actionURL(payloadString(event.Payload, "verification_url_path"), "/notification-contacts/verify", token)
	if err != nil {
		return err
	}
	subject, body := s.render(ctx, notificationEmailSpec{
		Template:       notificationscontract.TemplateNotificationContactVerification,
		DefaultSubject: "Verify your SRapi notification email",
		DefaultHTML:    `<p>Hello {{recipient_name}},</p><p>Use this link to verify this address for SRapi notifications:</p><p><a href="{{action_url}}">Verify notification email</a></p><p>This link expires at {{expires_at}}.</p>`,
	}, map[string]string{
		"recipient_name":  user.Name,
		"recipient_email": contactEmail,
		"action_url":      actionURL,
		"expires_at":      payloadString(event.Payload, "expires_at"),
	})
	return s.sender.Send(ctx, notificationscontract.EmailMessage{
		To:      contactEmail,
		Subject: subject,
		HTML:    body,
	})
}

func (s *Service) handlePendingOAuthEmailCompletion(ctx context.Context, event eventscontract.OutboxEvent) error {
	if !s.configured() {
		return ErrNotConfigured
	}
	email, err := s.decryptToken(payloadString(event.Payload, "recipient_email_ciphertext"), "v1", "auth.pending_oauth_email_completion.email:v1")
	if err != nil {
		return err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || emailHash(email) != payloadString(event.Payload, "recipient_email_hash") {
		return nil
	}
	tokenCiphertext := payloadString(event.Payload, "verification_token_ciphertext")
	if _, err := s.decryptToken(tokenCiphertext, "v1", "auth.pending_oauth_email_completion:v1"); err != nil {
		return err
	}
	actionURL, err := s.actionURL(payloadString(event.Payload, "verification_url_path"), "/oauth/pending/email-completion", tokenCiphertext)
	if err != nil {
		return err
	}
	subject, body := s.render(ctx, notificationEmailSpec{
		Template:       notificationscontract.TemplateAuthPendingOAuthEmailCompletion,
		DefaultSubject: "Complete your SRapi OAuth sign-in",
		DefaultHTML:    `<p>Hello {{recipient_name}},</p><p>Use this link to confirm this email address and continue your SRapi OAuth sign-in:</p><p><a href="{{action_url}}">Continue OAuth sign-in</a></p><p>This link expires at {{expires_at}}.</p>`,
	}, map[string]string{
		"recipient_name":  email,
		"recipient_email": email,
		"action_url":      actionURL,
		"expires_at":      payloadString(event.Payload, "expires_at"),
	})
	return s.sender.Send(ctx, notificationscontract.EmailMessage{
		To:      email,
		Subject: subject,
		HTML:    body,
	})
}

type authEmailSpec struct {
	Template       string
	TokenKey       string
	TokenVersion   string
	TokenAAD       string
	URLPathKey     string
	DefaultPath    string
	DefaultSubject string
	DefaultHTML    string
}

func (s *Service) handleAuthEmail(ctx context.Context, event eventscontract.OutboxEvent, spec authEmailSpec) error {
	if !s.configured() {
		return ErrNotConfigured
	}
	userID := payloadInt(event.Payload, "recipient_user_id")
	if userID <= 0 {
		return ErrInvalidInput
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Status != userscontract.StatusActive || emailHash(user.Email) != payloadString(event.Payload, "recipient_email_hash") {
		return nil
	}
	token, err := s.decryptToken(payloadString(event.Payload, spec.TokenKey), spec.TokenVersion, spec.TokenAAD)
	if err != nil {
		return err
	}
	actionURL, err := s.actionURL(payloadString(event.Payload, spec.URLPathKey), spec.DefaultPath, token)
	if err != nil {
		return err
	}
	subject, body := s.render(ctx, notificationEmailSpec{
		Template:       spec.Template,
		DefaultSubject: spec.DefaultSubject,
		DefaultHTML:    spec.DefaultHTML,
	}, map[string]string{
		"recipient_name":  user.Name,
		"recipient_email": user.Email,
		"action_url":      actionURL,
		"expires_at":      payloadString(event.Payload, "expires_at"),
	})
	return s.sender.Send(ctx, notificationscontract.EmailMessage{
		To:      user.Email,
		Subject: subject,
		HTML:    body,
	})
}

func (s *Service) handleBalanceLow(ctx context.Context, event eventscontract.OutboxEvent) error {
	if !s.configured() {
		return ErrNotConfigured
	}
	userID := payloadInt(event.Payload, "recipient_user_id")
	if userID <= 0 {
		return ErrInvalidInput
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Status != userscontract.StatusActive || emailHash(user.Email) != payloadString(event.Payload, "recipient_email_hash") {
		return nil
	}
	values := map[string]string{
		"recipient_name":  user.Name,
		"recipient_email": user.Email,
		"current_balance": formatNotificationMoney(payloadString(event.Payload, "balance_after")),
		"threshold":       formatNotificationMoney(payloadString(event.Payload, "threshold")),
		"currency":        payloadString(event.Payload, "currency"),
		"recharge_url":    s.balanceRechargeURL(payloadString(event.Payload, "recharge_url")),
		"unsubscribe_url": "",
	}

	return s.sendOptionalEmailToUser(ctx, user, notificationEmailSpec{
		Template:       notificationscontract.TemplateBalanceLow,
		DefaultSubject: "Your SRapi balance is low",
		DefaultHTML:    `<p>Hello {{recipient_name}},</p><p>Your current SRapi balance is {{current_balance}} {{currency}}, below the alert threshold of {{threshold}} {{currency}}.</p><p><a href="{{recharge_url}}">Recharge balance</a></p><p><a href="{{unsubscribe_url}}">Manage this alert</a></p>`,
	}, values)
}

func (s *Service) handleSubscriptionExpiry(ctx context.Context, event eventscontract.OutboxEvent) error {
	if !s.configured() {
		return ErrNotConfigured
	}
	userID := payloadInt(event.Payload, "recipient_user_id")
	if userID <= 0 {
		return ErrInvalidInput
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.Status != userscontract.StatusActive {
		return nil
	}
	values := map[string]string{
		"recipient_name":    user.Name,
		"recipient_email":   user.Email,
		"subscription_name": payloadString(event.Payload, "subscription_name"),
		"days_remaining":    payloadString(event.Payload, "days_remaining"),
		"expires_at":        payloadString(event.Payload, "expires_at"),
		"subscription_url":  s.consolePathURL(payloadString(event.Payload, "subscription_url"), "/subscriptions"),
		"unsubscribe_url":   "",
	}

	return s.sendOptionalEmailToUser(ctx, user, notificationEmailSpec{
		Template:       notificationscontract.TemplateSubscriptionExpiry,
		DefaultSubject: "Your SRapi subscription expires in {{days_remaining}} day(s)",
		DefaultHTML:    `<p>Hello {{recipient_name}},</p><p>Your SRapi subscription <strong>{{subscription_name}}</strong> expires in {{days_remaining}} day(s).</p><p>Expiry time: {{expires_at}}</p><p><a href="{{subscription_url}}">Review subscription</a></p><p><a href="{{unsubscribe_url}}">Manage this reminder</a></p>`,
	}, values)
}

func (s *Service) handleAccountQuotaAlert(ctx context.Context, event eventscontract.OutboxEvent) error {
	if !s.configured() {
		return ErrNotConfigured
	}
	recipients, err := s.adminRecipients(ctx)
	if err != nil {
		return err
	}
	for _, user := range recipients {
		values := map[string]string{
			"recipient_name":        user.Name,
			"recipient_email":       user.Email,
			"account_id":            payloadString(event.Payload, "account_id"),
			"account_name":          payloadString(event.Payload, "account_name"),
			"provider":              accountQuotaProvider(event.Payload),
			"quota_dimension":       payloadString(event.Payload, "quota_type"),
			"quota_used":            payloadString(event.Payload, "quota_used"),
			"quota_limit":           payloadString(event.Payload, "quota_limit"),
			"quota_remaining":       payloadString(event.Payload, "quota_remaining"),
			"quota_threshold":       formatNotificationRatio(payloadString(event.Payload, "quota_threshold")),
			"quota_remaining_ratio": formatNotificationRatio(payloadString(event.Payload, "quota_remaining_ratio")),
			"triggered_at":          payloadString(event.Payload, "triggered_at"),
			"account_url":           s.consolePathURL(payloadString(event.Payload, "account_url"), "/admin/accounts"),
			"unsubscribe_url":       "",
		}

		if err := s.sendOptionalEmailToUser(ctx, user, notificationEmailSpec{
			Template:       notificationscontract.TemplateAccountQuotaAlert,
			DefaultSubject: "SRapi account quota alert for {{account_name}}",
			DefaultHTML:    `<p>Hello {{recipient_name}},</p><p>Provider account <strong>{{account_name}}</strong> crossed the {{quota_threshold}} remaining quota threshold for {{quota_dimension}}.</p><p>Remaining: {{quota_remaining}} of {{quota_limit}} ({{quota_remaining_ratio}}).</p><p><a href="{{account_url}}">Review account quota</a></p><p><a href="{{unsubscribe_url}}">Manage this alert</a></p>`,
		}, values); err != nil {
			return err
		}
	}
	return nil
}

type notificationEmailSpec struct {
	Template       string
	DefaultSubject string
	DefaultHTML    string
}

func (s *Service) configured() bool {
	return s.config.PublicBaseURL != "" && s.config.SMTPHost != "" && s.config.SMTPFrom != ""
}

func (s *Service) decryptToken(ciphertextValue, wantVersion, aad string) (string, error) {
	return decryptSecretWithKey(s.tokenKey, ciphertextValue, wantVersion, aad)
}

func decryptSecretWithKey(key []byte, ciphertextValue, wantVersion, aad string) (string, error) {
	parts := strings.Split(ciphertextValue, ":")
	if len(parts) != 3 || parts[0] != wantVersion {
		return "", ErrInvalidInput
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrInvalidInput
	}
	rawCiphertext, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", ErrInvalidInput
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, nonce, rawCiphertext, []byte(aad))
	if err != nil {
		return "", ErrInvalidInput
	}
	return string(plaintext), nil
}

func (s *Service) decryptContactSecret(ciphertextValue, aad string) (string, error) {
	return decryptSecretWithKey(s.tokenKey, ciphertextValue, notificationContactTokenVersionV1, aad)
}

func (s *Service) actionURL(pathValue, defaultPath, token string) (string, error) {
	base, err := url.Parse(s.config.PublicBaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.RawQuery != "" || base.Fragment != "" {
		return "", ErrInvalidInput
	}
	pathValue = strings.TrimSpace(pathValue)
	if pathValue == "" {
		pathValue = defaultPath
	}
	if !strings.HasPrefix(pathValue, "/") || strings.Contains(pathValue, "://") || strings.ContainsAny(pathValue, "\r\n\t ") {
		return "", ErrInvalidInput
	}
	action := *base
	action.Path = strings.TrimRight(base.Path, "/") + pathValue
	query := action.Query()
	query.Set("token", token)
	action.RawQuery = query.Encode()
	return action.String(), nil
}

func (s *Service) render(ctx context.Context, spec notificationEmailSpec, values map[string]string) (string, string) {
	templates := s.currentTemplates(ctx)
	subject := strings.TrimSpace(templates[spec.Template+".subject"])
	if subject == "" {
		subject = spec.DefaultSubject
	}
	body := strings.TrimSpace(templates[spec.Template+".html"])
	if body == "" {
		body = spec.DefaultHTML
	}
	rendered, err := RenderEmailTemplate(spec.Template, subject, body, values)
	if err != nil {
		rendered, _ = RenderEmailTemplate(spec.Template, spec.DefaultSubject, spec.DefaultHTML, values)
	}
	return rendered.Subject, rendered.HTML
}

func (s *Service) sendOptionalEmailToUser(ctx context.Context, user userscontract.StoredUser, spec notificationEmailSpec, baseValues map[string]string) error {
	recipients := []optionalEmailRecipient{{
		Email: user.Email,
		Name:  user.Name,
	}}
	if s.contacts != nil {
		contacts, err := s.contacts.ListVerifiedContacts(ctx, user.ID)
		if err != nil {
			return err
		}
		for _, contact := range contacts {
			recipients = append(recipients, optionalEmailRecipient{
				Email: contact.Email,
				Name:  user.Name,
			})
		}
	}
	seen := map[string]bool{}
	for _, recipient := range recipients {
		email := strings.ToLower(strings.TrimSpace(recipient.Email))
		if email == "" || seen[email] {
			continue
		}
		seen[email] = true
		unsubscribed, err := s.optionalEmailUnsubscribed(ctx, recipient.Email, spec.Template)
		if err != nil {
			return err
		}
		if unsubscribed {
			continue
		}
		values := cloneStringMap(baseValues)
		values["recipient_name"] = recipient.Name
		values["recipient_email"] = recipient.Email
		headers, err := s.optionalEmailHeaders(ctx, recipient.Email, spec.Template, values)
		if err != nil {
			return err
		}
		renderedSubject, renderedBody := s.render(ctx, spec, values)
		if err := s.sender.Send(ctx, notificationscontract.EmailMessage{
			To:      recipient.Email,
			Subject: renderedSubject,
			HTML:    renderedBody,
			Headers: headers,
		}); err != nil {
			return err
		}
	}
	return nil
}

type optionalEmailRecipient struct {
	Email string
	Name  string
}

func (s *Service) optionalEmailUnsubscribed(ctx context.Context, email, event string) (bool, error) {
	if s.preferences == nil {
		return false, nil
	}
	return s.preferences.IsUnsubscribed(ctx, email, event)
}

func (s *Service) optionalEmailHeaders(_ context.Context, email, event string, values map[string]string) (map[string]string, error) {
	headers := map[string]string{}
	if s.preferences == nil {
		return headers, nil
	}
	unsubscribeURL, err := s.preferences.UnsubscribeURL(email, event)
	if err == nil {
		values["unsubscribe_url"] = unsubscribeURL
	} else if !errors.Is(err, ErrNotConfigured) {
		return nil, err
	}
	oneClickHeaders, err := s.preferences.OneClickHeaders(email, event)
	if err == nil {
		headers = oneClickHeaders
	} else if !errors.Is(err, ErrNotConfigured) {
		return nil, err
	}
	return headers, nil
}

func (s *Service) currentTemplates(ctx context.Context) map[string]string {
	if s.templateProvider == nil {
		return s.templates
	}
	templates := s.templateProvider.NotificationEmailTemplates(ctx)
	if templates == nil {
		return s.templates
	}
	return templates
}

func (s *Service) balanceRechargeURL(value string) string {
	return s.consolePathURL(value, "/billing")
}

func (s *Service) consolePathURL(value string, defaultPath string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		parsed, err := url.Parse(value)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.RawQuery == "" && parsed.Fragment == "" {
			return parsed.String()
		}
		if strings.HasPrefix(value, "/") && !strings.Contains(value, "://") && !strings.ContainsAny(value, "\r\n\t ") {
			defaultPath = value
		} else {
			return ""
		}
	}
	if s.config.PublicBaseURL == "" {
		return ""
	}
	base, err := url.Parse(s.config.PublicBaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return ""
	}
	defaultPath = strings.TrimSpace(defaultPath)
	if defaultPath == "" {
		defaultPath = "/"
	}
	if !strings.HasPrefix(defaultPath, "/") {
		defaultPath = "/" + defaultPath
	}
	base.Path = strings.TrimRight(base.Path, "/") + defaultPath
	base.RawQuery = ""
	base.Fragment = ""
	return base.String()
}

func formatNotificationMoney(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0.00"
	}
	amount, ok := new(big.Rat).SetString(value)
	if !ok {
		return value
	}
	return amount.FloatString(2)
}

func formatNotificationRatio(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	ratio, ok := new(big.Rat).SetString(value)
	if !ok {
		return value
	}
	percent := new(big.Rat).Mul(ratio, big.NewRat(100, 1))
	return percent.FloatString(0) + "%"
}

func accountQuotaProvider(payload map[string]any) string {
	provider := payloadString(payload, "provider")
	if provider != "" {
		return provider
	}
	providerID := payloadString(payload, "provider_id")
	if providerID != "" {
		return "provider " + providerID
	}
	return payloadString(payload, "runtime_class")
}

func (s *Service) adminRecipients(ctx context.Context) ([]userscontract.StoredUser, error) {
	status := userscontract.StatusActive
	users, err := s.users.List(ctx, userscontract.ListUsersFilter{Status: &status})
	if err != nil {
		return nil, err
	}
	out := make([]userscontract.StoredUser, 0)
	seen := map[string]bool{}
	for _, user := range users {
		if !notificationAdminUser(user) {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(user.Email))
		if email == "" || seen[email] {
			continue
		}
		seen[email] = true
		out = append(out, user)
	}
	return out, nil
}

func notificationAdminUser(user userscontract.StoredUser) bool {
	for _, role := range user.Roles {
		if role == userscontract.RoleOwner || role == userscontract.RoleAdmin {
			return true
		}
	}
	return false
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

func emailHash(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func cloneTemplates(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
