package service_test

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	notificationsservice "github.com/srapi/srapi/apps/api/internal/modules/notifications/service"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	usermemory "github.com/srapi/srapi/apps/api/internal/modules/users/store/memory"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
)

const testMasterKey = "notification_master_key_32_bytes_min"

func TestAuthPasswordResetEventSendsRenderedEmail(t *testing.T) {
	users := usermemory.New()
	user, err := users.Create(context.Background(), userscontract.CreateStoredUser{
		Email:        "notify@srapi.local",
		Name:         "Notify User",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleUser},
		Balance:      "0.00000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sender := &fakeSender{}
	svc, err := notificationsservice.New(users, sender, notificationscontract.EmailConfig{
		PublicBaseURL: "https://console.srapi.local",
		SMTPHost:      "smtp.srapi.local",
		SMTPPort:      587,
		SMTPFrom:      "noreply@srapi.local",
	}, testMasterKey, map[string]string{
		"auth.password_reset.subject": "Reset {{recipient_email}}",
		"auth.password_reset.html":    `<a href="{{action_url}}">Reset {{recipient_name}}</a>`,
	})
	if err != nil {
		t.Fatalf("new notifications service: %v", err)
	}
	rawToken := "pwreset_test_token"
	event := eventscontract.OutboxEvent{
		EventType: notificationscontract.EventAuthPasswordResetRequested,
		Payload: map[string]any{
			"template":               notificationscontract.TemplateAuthPasswordReset,
			"recipient_user_id":      user.ID,
			"recipient_email_hash":   emailHash(user.Email),
			"reset_token_ciphertext": encryptToken(t, rawToken, "auth.password_reset:v1"),
			"reset_token_version":    "v1",
			"reset_url_path":         "/reset-password",
			"expires_at":             "2026-05-29T10:30:00Z",
		},
	}

	if err := svc.HandleOutboxEvent(context.Background(), event); err != nil {
		t.Fatalf("handle outbox event: %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("expected one message, got %+v", sender.messages)
	}
	message := sender.messages[0]
	if message.To != user.Email || message.Subject != "Reset notify@srapi.local" {
		t.Fatalf("unexpected message header: %+v", message)
	}
	if len(message.Headers) != 0 {
		t.Fatalf("auth email should not include optional unsubscribe headers: %+v", message.Headers)
	}
	if !strings.Contains(message.HTML, "https://console.srapi.local/reset-password?token=pwreset_test_token") {
		t.Fatalf("expected action URL with token, got %s", message.HTML)
	}
}

func TestAuthEmailEventSkipsStaleRecipientHash(t *testing.T) {
	users := usermemory.New()
	user, err := users.Create(context.Background(), userscontract.CreateStoredUser{
		Email:        "current@srapi.local",
		Name:         "Current User",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleUser},
		Balance:      "0.00000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sender := &fakeSender{}
	svc, err := notificationsservice.New(users, sender, notificationscontract.EmailConfig{
		PublicBaseURL: "https://console.srapi.local",
		SMTPHost:      "smtp.srapi.local",
		SMTPPort:      587,
		SMTPFrom:      "noreply@srapi.local",
	}, testMasterKey, nil)
	if err != nil {
		t.Fatalf("new notifications service: %v", err)
	}
	event := eventscontract.OutboxEvent{
		EventType: notificationscontract.EventAuthEmailVerificationRequested,
		Payload: map[string]any{
			"recipient_user_id":             user.ID,
			"recipient_email_hash":          emailHash("old@srapi.local"),
			"verification_token_ciphertext": encryptToken(t, "emailverify_test_token", "auth.email_verification:v1"),
			"verification_token_version":    "v1",
			"verification_url_path":         "/verify-email",
		},
	}

	if err := svc.HandleOutboxEvent(context.Background(), event); err != nil {
		t.Fatalf("handle stale event: %v", err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("stale recipient should not send email, got %+v", sender.messages)
	}
}

func TestPendingOAuthEmailCompletionEventSendsToEncryptedRecipient(t *testing.T) {
	users := usermemory.New()
	sender := &fakeSender{}
	svc, err := notificationsservice.New(users, sender, notificationscontract.EmailConfig{
		PublicBaseURL: "https://console.srapi.local",
		SMTPHost:      "smtp.srapi.local",
		SMTPPort:      587,
		SMTPFrom:      "noreply@srapi.local",
	}, testMasterKey, map[string]string{
		"auth.oauth_pending_email_completion.subject": "Continue OAuth",
		"auth.oauth_pending_email_completion.html":    `<a href="{{action_url}}">Continue {{recipient_email}}</a>`,
	})
	if err != nil {
		t.Fatalf("new notifications service: %v", err)
	}
	rawTokenCiphertext := encryptToken(t, "pending_oauth_email_token", "auth.pending_oauth_email_completion:v1")
	event := eventscontract.OutboxEvent{
		EventType: notificationscontract.EventPendingOAuthEmailCompletionRequested,
		Payload: map[string]any{
			"recipient_email_hash":          emailHash("complete@srapi.local"),
			"recipient_email_ciphertext":    encryptToken(t, "complete@srapi.local", "auth.pending_oauth_email_completion.email:v1"),
			"verification_token_ciphertext": rawTokenCiphertext,
			"verification_token_version":    "v1",
			"verification_url_path":         "/oauth/pending/email-completion",
			"expires_at":                    "2026-05-30T15:30:00Z",
		},
	}

	if err := svc.HandleOutboxEvent(context.Background(), event); err != nil {
		t.Fatalf("handle pending oauth email completion event: %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("expected one message, got %+v", sender.messages)
	}
	message := sender.messages[0]
	if message.To != "complete@srapi.local" || message.Subject != "Continue OAuth" {
		t.Fatalf("unexpected pending oauth completion message: %+v", message)
	}
	if !strings.Contains(message.HTML, "https://console.srapi.local/oauth/pending/email-completion?token="+url.QueryEscape(rawTokenCiphertext)) {
		t.Fatalf("expected pending oauth action URL, got %s", message.HTML)
	}

	sender.messages = nil
	event.Payload["recipient_email_hash"] = emailHash("other@srapi.local")
	if err := svc.HandleOutboxEvent(context.Background(), event); err != nil {
		t.Fatalf("handle stale pending oauth email completion event: %v", err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("stale recipient hash should suppress send, got %+v", sender.messages)
	}
}

func TestAuthEmailEventRequiresConfiguredSMTPAndBaseURL(t *testing.T) {
	users := usermemory.New()
	sender := &fakeSender{}
	svc, err := notificationsservice.New(users, sender, notificationscontract.EmailConfig{
		SMTPHost: "smtp.srapi.local",
		SMTPFrom: "noreply@srapi.local",
	}, testMasterKey, nil)
	if err != nil {
		t.Fatalf("new notifications service: %v", err)
	}
	err = svc.HandleOutboxEvent(context.Background(), eventscontract.OutboxEvent{
		EventType: notificationscontract.EventAuthPasswordResetRequested,
	})
	if err != notificationsservice.ErrNotConfigured {
		t.Fatalf("expected not configured, got %v", err)
	}
}

func TestBalanceLowEventSendsOptionalEmailWithOneClickHeaders(t *testing.T) {
	users := usermemory.New()
	user, err := users.Create(context.Background(), userscontract.CreateStoredUser{
		Email:        "balance@srapi.local",
		Name:         "Balance User",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleUser},
		Balance:      "4.50000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	preferences, err := notificationsservice.NewPreferenceService(newFakePreferenceStore(), testMasterKey, "https://console.srapi.local")
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	sender := &fakeSender{}
	svc, err := notificationsservice.NewWithPreferences(users, sender, notificationscontract.EmailConfig{
		PublicBaseURL: "https://console.srapi.local",
		SMTPHost:      "smtp.srapi.local",
		SMTPPort:      587,
		SMTPFrom:      "noreply@srapi.local",
	}, testMasterKey, nil, preferences)
	if err != nil {
		t.Fatalf("new notifications service: %v", err)
	}

	err = svc.HandleOutboxEvent(context.Background(), eventscontract.OutboxEvent{
		EventType: notificationscontract.EventBalanceLowTriggered,
		Payload: map[string]any{
			"recipient_user_id":    user.ID,
			"recipient_email_hash": emailHash(user.Email),
			"balance_after":        "4.50000000",
			"threshold":            "5.00000000",
			"currency":             "USD",
			"recharge_url":         "https://console.srapi.local/billing",
		},
	})
	if err != nil {
		t.Fatalf("handle balance low event: %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("expected one balance low email, got %+v", sender.messages)
	}
	message := sender.messages[0]
	if message.To != user.Email || message.Subject != "Your SRapi balance is low" {
		t.Fatalf("unexpected balance low message: %+v", message)
	}
	if !strings.Contains(message.HTML, "4.50 USD") || !strings.Contains(message.HTML, "https://console.srapi.local/billing") {
		t.Fatalf("expected rendered balance values, got %s", message.HTML)
	}
	if message.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" || strings.Contains(strings.ToLower(message.Headers["List-Unsubscribe"]), user.Email) {
		t.Fatalf("expected safe one-click unsubscribe headers, got %+v", message.Headers)
	}
}

func TestBalanceLowEventRespectsUnsubscribePreference(t *testing.T) {
	users := usermemory.New()
	user, err := users.Create(context.Background(), userscontract.CreateStoredUser{
		Email:        "unsubscribed-balance@srapi.local",
		Name:         "No Balance Mail",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleUser},
		Balance:      "4.50000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	preferences, err := notificationsservice.NewPreferenceService(newFakePreferenceStore(), testMasterKey, "https://console.srapi.local")
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	if _, err := preferences.SetPreference(context.Background(), user.Email, notificationscontract.TemplateBalanceLow, false, "test", nil); err != nil {
		t.Fatalf("unsubscribe preference: %v", err)
	}
	sender := &fakeSender{}
	svc, err := notificationsservice.NewWithPreferences(users, sender, notificationscontract.EmailConfig{
		PublicBaseURL: "https://console.srapi.local",
		SMTPHost:      "smtp.srapi.local",
		SMTPPort:      587,
		SMTPFrom:      "noreply@srapi.local",
	}, testMasterKey, nil, preferences)
	if err != nil {
		t.Fatalf("new notifications service: %v", err)
	}

	err = svc.HandleOutboxEvent(context.Background(), eventscontract.OutboxEvent{
		EventType: notificationscontract.EventBalanceLowTriggered,
		Payload: map[string]any{
			"recipient_user_id":    user.ID,
			"recipient_email_hash": emailHash(user.Email),
			"balance_after":        "4.50000000",
			"threshold":            "5.00000000",
			"currency":             "USD",
		},
	})
	if err != nil {
		t.Fatalf("handle balance low event: %v", err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("unsubscribed balance low recipient should be suppressed, got %+v", sender.messages)
	}
}

func TestNotificationContactVerificationEventSendsToEncryptedContactEmail(t *testing.T) {
	users := usermemory.New()
	user, err := users.Create(context.Background(), userscontract.CreateStoredUser{
		Email:        "owner@srapi.local",
		Name:         "Owner User",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleUser},
		Balance:      "0.00000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	enqueuer := newFakeEventEnqueuer()
	contacts, err := notificationsservice.NewContactService(newFakePreferenceStore(), testMasterKey, "https://console.srapi.local", enqueuer)
	if err != nil {
		t.Fatalf("new contact service: %v", err)
	}
	result, err := contacts.RequestVerification(context.Background(), notificationsservice.ContactVerificationRequest{
		UserID:       user.ID,
		UserName:     user.Name,
		UserEmail:    user.Email,
		ContactEmail: "Alerts@SRapi.Local",
	}, &user.ID)
	if err != nil {
		t.Fatalf("request contact verification: %v", err)
	}
	if len(enqueuer.events) != 1 {
		t.Fatalf("expected one verification outbox event, got %+v", enqueuer.events)
	}
	payload := enqueuer.events[0].Payload
	if strings.Contains(fmt.Sprint(payload), "alerts@srapi.local") {
		t.Fatalf("contact verification event leaked plaintext email: %+v", payload)
	}
	sender := &fakeSender{}
	svc, err := notificationsservice.New(users, sender, notificationscontract.EmailConfig{
		PublicBaseURL: "https://console.srapi.local",
		SMTPHost:      "smtp.srapi.local",
		SMTPPort:      587,
		SMTPFrom:      "noreply@srapi.local",
	}, testMasterKey, map[string]string{
		"notification.contact_verification.subject": "Verify {{recipient_email}}",
		"notification.contact_verification.html":    `<a href="{{action_url}}">Verify {{recipient_name}}</a>`,
	})
	if err != nil {
		t.Fatalf("new notification service: %v", err)
	}

	if err := svc.HandleOutboxEvent(context.Background(), enqueuer.events[0]); err != nil {
		t.Fatalf("handle contact verification event: %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("expected one contact verification email, got %+v", sender.messages)
	}
	message := sender.messages[0]
	if message.To != "alerts@srapi.local" || message.Subject != "Verify alerts@srapi.local" {
		t.Fatalf("unexpected verification message: %+v", message)
	}
	if strings.Contains(message.HTML, result.Contact.EmailHash) || !strings.Contains(message.HTML, "https://console.srapi.local/notification-contacts/verify?token=") {
		t.Fatalf("expected verification action URL without hash leak, got %s", message.HTML)
	}
	if len(message.Headers) != 0 {
		t.Fatalf("verification mail must not include optional unsubscribe headers: %+v", message.Headers)
	}
}

func TestBalanceLowEventSendsToVerifiedContactsAndHonorsContactUnsubscribe(t *testing.T) {
	users := usermemory.New()
	user, err := users.Create(context.Background(), userscontract.CreateStoredUser{
		Email:        "balance-with-contact@srapi.local",
		Name:         "Balance Contact",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleUser},
		Balance:      "4.50000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	store := newFakePreferenceStore()
	events := newFakeEventEnqueuer()
	contacts, err := notificationsservice.NewContactService(store, testMasterKey, "https://console.srapi.local", events)
	if err != nil {
		t.Fatalf("new contact service: %v", err)
	}
	verified := verifyContactForTest(t, contacts, events, user.ID, user.Name, user.Email, "verified-alert@srapi.local")
	disabled := verifyContactForTest(t, contacts, events, user.ID, user.Name, user.Email, "disabled-alert@srapi.local")
	if _, err := contacts.SetContactDisabled(context.Background(), user.ID, disabled.ID, true, nil); err != nil {
		t.Fatalf("disable contact: %v", err)
	}
	preferences, err := notificationsservice.NewPreferenceService(store, testMasterKey, "https://console.srapi.local")
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	if _, err := preferences.SetPreference(context.Background(), verified.Email, notificationscontract.TemplateBalanceLow, false, "test", nil); err != nil {
		t.Fatalf("unsubscribe verified contact: %v", err)
	}
	sender := &fakeSender{}
	svc, err := notificationsservice.NewWithPreferencesAndContacts(users, sender, notificationscontract.EmailConfig{
		PublicBaseURL: "https://console.srapi.local",
		SMTPHost:      "smtp.srapi.local",
		SMTPPort:      587,
		SMTPFrom:      "noreply@srapi.local",
	}, testMasterKey, nil, preferences, contacts)
	if err != nil {
		t.Fatalf("new notification service: %v", err)
	}

	err = svc.HandleOutboxEvent(context.Background(), eventscontract.OutboxEvent{
		EventType: notificationscontract.EventBalanceLowTriggered,
		Payload: map[string]any{
			"recipient_user_id":    user.ID,
			"recipient_email_hash": emailHash(user.Email),
			"balance_after":        "4.50000000",
			"threshold":            "5.00000000",
			"currency":             "USD",
		},
	})
	if err != nil {
		t.Fatalf("handle balance low event: %v", err)
	}
	if len(sender.messages) != 1 || sender.messages[0].To != user.Email {
		t.Fatalf("contact unsubscribe and disabled contact should suppress extras, got %+v", sender.messages)
	}

	if _, err := preferences.SetPreference(context.Background(), verified.Email, notificationscontract.TemplateBalanceLow, true, "test", nil); err != nil {
		t.Fatalf("resubscribe verified contact: %v", err)
	}
	sender.messages = nil
	if err := svc.HandleOutboxEvent(context.Background(), eventscontract.OutboxEvent{
		EventType: notificationscontract.EventBalanceLowTriggered,
		Payload: map[string]any{
			"recipient_user_id":    user.ID,
			"recipient_email_hash": emailHash(user.Email),
			"balance_after":        "4.50000000",
			"threshold":            "5.00000000",
			"currency":             "USD",
		},
	}); err != nil {
		t.Fatalf("handle balance low event after resubscribe: %v", err)
	}
	if len(sender.messages) != 2 {
		t.Fatalf("expected primary plus one verified contact, got %+v", sender.messages)
	}
	if sender.messages[1].To != verified.Email {
		t.Fatalf("expected verified contact recipient, got %+v", sender.messages)
	}
}

func TestSubscriptionExpiryEventSendsOptionalEmailWithOneClickHeaders(t *testing.T) {
	users := usermemory.New()
	user, err := users.Create(context.Background(), userscontract.CreateStoredUser{
		Email:        "subscription@srapi.local",
		Name:         "Subscription User",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleUser},
		Balance:      "0.00000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	preferences, err := notificationsservice.NewPreferenceService(newFakePreferenceStore(), testMasterKey, "https://console.srapi.local")
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	sender := &fakeSender{}
	svc, err := notificationsservice.NewWithPreferences(users, sender, notificationscontract.EmailConfig{
		PublicBaseURL: "https://console.srapi.local/app",
		SMTPHost:      "smtp.srapi.local",
		SMTPPort:      587,
		SMTPFrom:      "noreply@srapi.local",
	}, testMasterKey, nil, preferences)
	if err != nil {
		t.Fatalf("new notifications service: %v", err)
	}

	err = svc.HandleOutboxEvent(context.Background(), eventscontract.OutboxEvent{
		EventType: notificationscontract.EventSubscriptionExpiryReminder,
		Payload: map[string]any{
			"recipient_user_id": user.ID,
			"subscription_name": "Pro",
			"days_remaining":    3,
			"expires_at":        "2026-06-01T10:30:00Z",
			"subscription_url":  "/subscriptions",
		},
	})
	if err != nil {
		t.Fatalf("handle subscription expiry event: %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("expected one subscription expiry email, got %+v", sender.messages)
	}
	message := sender.messages[0]
	if message.To != user.Email || message.Subject != "Your SRapi subscription expires in 3 day(s)" {
		t.Fatalf("unexpected subscription expiry message: %+v", message)
	}
	if !strings.Contains(message.HTML, "Pro") || !strings.Contains(message.HTML, "2026-06-01T10:30:00Z") || !strings.Contains(message.HTML, "https://console.srapi.local/app/subscriptions") {
		t.Fatalf("expected rendered subscription values, got %s", message.HTML)
	}
	if message.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" || strings.Contains(strings.ToLower(message.Headers["List-Unsubscribe"]), user.Email) {
		t.Fatalf("expected safe subscription one-click headers, got %+v", message.Headers)
	}
}

func TestSubscriptionExpiryEventRespectsUnsubscribePreference(t *testing.T) {
	users := usermemory.New()
	user, err := users.Create(context.Background(), userscontract.CreateStoredUser{
		Email:        "unsubscribed-subscription@srapi.local",
		Name:         "No Subscription Mail",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleUser},
		Balance:      "0.00000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	preferences, err := notificationsservice.NewPreferenceService(newFakePreferenceStore(), testMasterKey, "https://console.srapi.local")
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	if _, err := preferences.SetPreference(context.Background(), user.Email, notificationscontract.TemplateSubscriptionExpiry, false, "test", nil); err != nil {
		t.Fatalf("unsubscribe preference: %v", err)
	}
	sender := &fakeSender{}
	svc, err := notificationsservice.NewWithPreferences(users, sender, notificationscontract.EmailConfig{
		PublicBaseURL: "https://console.srapi.local",
		SMTPHost:      "smtp.srapi.local",
		SMTPPort:      587,
		SMTPFrom:      "noreply@srapi.local",
	}, testMasterKey, nil, preferences)
	if err != nil {
		t.Fatalf("new notifications service: %v", err)
	}

	err = svc.HandleOutboxEvent(context.Background(), eventscontract.OutboxEvent{
		EventType: notificationscontract.EventSubscriptionExpiryReminder,
		Payload: map[string]any{
			"recipient_user_id": user.ID,
			"subscription_name": "Pro",
			"days_remaining":    1,
			"expires_at":        "2026-05-30T10:30:00Z",
		},
	})
	if err != nil {
		t.Fatalf("handle subscription expiry event: %v", err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("unsubscribed subscription expiry recipient should be suppressed, got %+v", sender.messages)
	}
}

func TestAccountQuotaAlertEventSendsToActiveAdminsWithOneClickHeaders(t *testing.T) {
	users := usermemory.New()
	admin, err := users.Create(context.Background(), userscontract.CreateStoredUser{
		Email:        "admin@srapi.local",
		Name:         "Admin User",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleAdmin},
		Balance:      "0.00000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := users.Create(context.Background(), userscontract.CreateStoredUser{
		Email:        "user@srapi.local",
		Name:         "Normal User",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleUser},
		Balance:      "0.00000000",
		Currency:     "USD",
	}); err != nil {
		t.Fatalf("create normal user: %v", err)
	}
	preferences, err := notificationsservice.NewPreferenceService(newFakePreferenceStore(), testMasterKey, "https://console.srapi.local")
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	sender := &fakeSender{}
	svc, err := notificationsservice.NewWithPreferences(users, sender, notificationscontract.EmailConfig{
		PublicBaseURL: "https://console.srapi.local/app",
		SMTPHost:      "smtp.srapi.local",
		SMTPPort:      587,
		SMTPFrom:      "noreply@srapi.local",
	}, testMasterKey, nil, preferences)
	if err != nil {
		t.Fatalf("new notifications service: %v", err)
	}

	err = svc.HandleOutboxEvent(context.Background(), eventscontract.OutboxEvent{
		EventType: notificationscontract.EventAccountQuotaAlertTriggered,
		Payload: map[string]any{
			"account_id":            12,
			"account_name":          "codex-primary",
			"provider_id":           7,
			"quota_type":            "codex_5h_percent",
			"quota_used":            "85",
			"quota_limit":           "100",
			"quota_remaining":       "15",
			"quota_remaining_ratio": "0.15000000",
			"quota_threshold":       "0.20000000",
			"triggered_at":          "2026-05-29T12:00:00Z",
			"account_url":           "/admin/accounts/12",
		},
	})
	if err != nil {
		t.Fatalf("handle account quota event: %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("expected one admin quota alert email, got %+v", sender.messages)
	}
	message := sender.messages[0]
	if message.To != admin.Email || message.Subject != "SRapi account quota alert for codex-primary" {
		t.Fatalf("unexpected quota alert message: %+v", message)
	}
	if !strings.Contains(message.HTML, "codex-primary") || !strings.Contains(message.HTML, "20%") || !strings.Contains(message.HTML, "https://console.srapi.local/app/admin/accounts/12") {
		t.Fatalf("expected rendered quota alert values, got %s", message.HTML)
	}
	if message.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" || strings.Contains(strings.ToLower(message.Headers["List-Unsubscribe"]), admin.Email) {
		t.Fatalf("expected safe quota alert one-click headers, got %+v", message.Headers)
	}
}

func TestAccountQuotaAlertEventRespectsAdminUnsubscribePreference(t *testing.T) {
	users := usermemory.New()
	admin, err := users.Create(context.Background(), userscontract.CreateStoredUser{
		Email:        "unsubscribed-admin@srapi.local",
		Name:         "No Quota Mail",
		PasswordHash: "hash",
		Status:       userscontract.StatusActive,
		Roles:        []userscontract.Role{userscontract.RoleAdmin},
		Balance:      "0.00000000",
		Currency:     "USD",
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	preferences, err := notificationsservice.NewPreferenceService(newFakePreferenceStore(), testMasterKey, "https://console.srapi.local")
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	if _, err := preferences.SetPreference(context.Background(), admin.Email, notificationscontract.TemplateAccountQuotaAlert, false, "test", nil); err != nil {
		t.Fatalf("unsubscribe quota alert: %v", err)
	}
	sender := &fakeSender{}
	svc, err := notificationsservice.NewWithPreferences(users, sender, notificationscontract.EmailConfig{
		PublicBaseURL: "https://console.srapi.local",
		SMTPHost:      "smtp.srapi.local",
		SMTPPort:      587,
		SMTPFrom:      "noreply@srapi.local",
	}, testMasterKey, nil, preferences)
	if err != nil {
		t.Fatalf("new notifications service: %v", err)
	}

	err = svc.HandleOutboxEvent(context.Background(), eventscontract.OutboxEvent{
		EventType: notificationscontract.EventAccountQuotaAlertTriggered,
		Payload: map[string]any{
			"account_id":            12,
			"account_name":          "codex-primary",
			"quota_type":            "codex_7d_percent",
			"quota_remaining_ratio": "0.10000000",
			"quota_threshold":       "0.20000000",
		},
	})
	if err != nil {
		t.Fatalf("handle account quota event: %v", err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("unsubscribed admin should be suppressed, got %+v", sender.messages)
	}
}

func TestEmailTemplatePreviewEscapesVariablesAndBlanksUnsafeURLs(t *testing.T) {
	preview, err := notificationsservice.PreviewEmailTemplate(nil, notificationsservice.EmailTemplatePreviewInput{
		Event:   notificationscontract.TemplateBalanceLow,
		Subject: "Balance alert for {{ recipient_name }}",
		HTML:    `<p>{{recipient_name}}</p><a href="{{recharge_url}}">Recharge</a><a href="{{unsubscribe_url}}">Manage</a>`,
		Variables: map[string]string{
			"recipient_name":  "<Admin>",
			"recharge_url":    "javascript:alert(1)",
			"unsubscribe_url": "https://console.srapi.local/api/v1/notifications/unsubscribe?token=preview",
		},
	})
	if err != nil {
		t.Fatalf("preview template: %v", err)
	}
	if preview.Subject != "Balance alert for &lt;Admin&gt;" {
		t.Fatalf("expected escaped subject, got %q", preview.Subject)
	}
	if !strings.Contains(preview.HTML, "&lt;Admin&gt;") {
		t.Fatalf("expected escaped HTML variable, got %s", preview.HTML)
	}
	if strings.Contains(preview.HTML, "javascript:") || !strings.Contains(preview.HTML, `href=""`) {
		t.Fatalf("expected unsafe URL placeholder to be blanked, got %s", preview.HTML)
	}
}

func TestEmailTemplateUpdateRejectsUnsupportedPlaceholder(t *testing.T) {
	_, _, err := notificationsservice.UpdateEmailTemplate(nil, notificationscontract.TemplateAuthPasswordReset, "Reset {{evil}}", "<p>Reset</p>")
	if !errors.Is(err, notificationsservice.ErrInvalidInput) {
		t.Fatalf("expected invalid input for unsupported placeholder, got %v", err)
	}
}

func TestEmailTemplateRestoreRemovesOnlySelectedOverride(t *testing.T) {
	overrides, detail, err := notificationsservice.UpdateEmailTemplate(nil, notificationscontract.TemplateAuthPasswordReset, "Custom reset", "<p>Reset {{action_url}}</p>")
	if err != nil {
		t.Fatalf("update reset template: %v", err)
	}
	if !detail.IsCustom {
		t.Fatalf("expected updated template to be custom: %+v", detail)
	}
	overrides["auth.email_verification.subject"] = "Keep this"

	restored, detail, err := notificationsservice.RestoreEmailTemplate(overrides, notificationscontract.TemplateAuthPasswordReset)
	if err != nil {
		t.Fatalf("restore reset template: %v", err)
	}
	if detail.IsCustom || detail.Subject != "Reset your SRapi password" {
		t.Fatalf("expected default reset template after restore, got %+v", detail)
	}
	if _, ok := restored["auth.password_reset.subject"]; ok {
		t.Fatalf("expected reset subject override to be removed: %+v", restored)
	}
	if restored["auth.email_verification.subject"] != "Keep this" {
		t.Fatalf("restore should preserve other template overrides: %+v", restored)
	}
}

func TestPreferenceServiceUnsubscribeSuppressesOptionalEventByHash(t *testing.T) {
	store := newFakePreferenceStore()
	svc, err := notificationsservice.NewPreferenceService(store, testMasterKey, "https://console.srapi.local")
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	ctx := context.Background()
	email := "User@Example.COM"
	unsubscribed, err := svc.IsUnsubscribed(ctx, email, notificationscontract.TemplateBalanceLow)
	if err != nil {
		t.Fatalf("check initial preference: %v", err)
	}
	if unsubscribed {
		t.Fatal("new recipient should not start unsubscribed")
	}

	token, err := svc.CreateUnsubscribeToken(email, notificationscontract.TemplateBalanceLow)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	if strings.Contains(strings.ToLower(token), "user@example.com") {
		t.Fatalf("unsubscribe token leaked plaintext email: %s", token)
	}
	preview, err := svc.PreviewUnsubscribe(token)
	if err != nil {
		t.Fatalf("preview token: %v", err)
	}
	if preview.Event != notificationscontract.TemplateBalanceLow || preview.Done {
		t.Fatalf("unexpected preview: %+v", preview)
	}

	result, err := svc.Unsubscribe(ctx, token)
	if err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if result.Event != notificationscontract.TemplateBalanceLow || !result.Done {
		t.Fatalf("unexpected unsubscribe result: %+v", result)
	}
	unsubscribed, err = svc.IsUnsubscribed(ctx, "user@example.com", notificationscontract.TemplateBalanceLow)
	if err != nil {
		t.Fatalf("check stored preference: %v", err)
	}
	if !unsubscribed {
		t.Fatal("expected normalized email hash to be unsubscribed")
	}
	otherEvent, err := svc.IsUnsubscribed(ctx, "user@example.com", notificationscontract.TemplateAccountQuotaAlert)
	if err != nil {
		t.Fatalf("check separate event preference: %v", err)
	}
	if otherEvent {
		t.Fatal("preference should be event scoped")
	}
}

func TestPreferenceServiceRejectsTransactionalUnsubscribe(t *testing.T) {
	svc, err := notificationsservice.NewPreferenceService(newFakePreferenceStore(), testMasterKey, "https://console.srapi.local")
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	if _, err := svc.CreateUnsubscribeToken("user@example.com", notificationscontract.TemplateAuthPasswordReset); !errors.Is(err, notificationsservice.ErrUnsupportedNotificationEvent) {
		t.Fatalf("expected transactional event to be rejected, got %v", err)
	}
	unsubscribed, err := svc.IsUnsubscribed(context.Background(), "user@example.com", notificationscontract.TemplateAuthPasswordReset)
	if err != nil {
		t.Fatalf("transactional preference check should not fail: %v", err)
	}
	if unsubscribed {
		t.Fatal("transactional auth email must never be suppressed by optional preferences")
	}
}

func TestPreferenceServiceListsAndUpdatesCurrentUserPreferences(t *testing.T) {
	store := newFakePreferenceStore()
	svc, err := notificationsservice.NewPreferenceService(store, testMasterKey, "https://console.srapi.local")
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	ctx := context.Background()
	email := "User@Example.COM"

	initial, err := svc.ListPreferences(ctx, email)
	if err != nil {
		t.Fatalf("list initial preferences: %v", err)
	}
	if len(initial) != 3 {
		t.Fatalf("expected three optional preference events, got %+v", initial)
	}
	for _, item := range initial {
		if !item.Subscribed {
			t.Fatalf("new preferences should default to subscribed, got %+v", initial)
		}
	}

	actorID := 42
	updated, err := svc.SetPreference(ctx, email, notificationscontract.TemplateBalanceLow, false, "current_user", &actorID)
	if err != nil {
		t.Fatalf("unsubscribe current user preference: %v", err)
	}
	if updated.Event != notificationscontract.TemplateBalanceLow || updated.Subscribed || updated.UpdatedAt == nil {
		t.Fatalf("unexpected updated preference: %+v", updated)
	}
	unsubscribed, err := svc.IsUnsubscribed(ctx, strings.ToLower(email), notificationscontract.TemplateBalanceLow)
	if err != nil {
		t.Fatalf("check unsubscribed preference: %v", err)
	}
	if !unsubscribed {
		t.Fatal("expected explicit current-user unsubscribe to suppress optional event")
	}
	for key, value := range store.values {
		if strings.Contains(strings.ToLower(key), "user@example.com") || strings.Contains(strings.ToLower(fmt.Sprint(value)), "user@example.com") {
			t.Fatalf("stored preference leaked plaintext email: key=%s value=%+v", key, value)
		}
	}

	resubscribed, err := svc.SetPreference(ctx, email, notificationscontract.TemplateBalanceLow, true, "current_user", &actorID)
	if err != nil {
		t.Fatalf("resubscribe current user preference: %v", err)
	}
	if !resubscribed.Subscribed {
		t.Fatalf("expected preference to resubscribe: %+v", resubscribed)
	}
	unsubscribed, err = svc.IsUnsubscribed(ctx, email, notificationscontract.TemplateBalanceLow)
	if err != nil {
		t.Fatalf("check resubscribed preference: %v", err)
	}
	if unsubscribed {
		t.Fatal("resubscribed preference should not suppress optional event")
	}

	if _, err := svc.SetPreference(ctx, email, notificationscontract.TemplateAuthPasswordReset, false, "current_user", &actorID); !errors.Is(err, notificationsservice.ErrUnsupportedNotificationEvent) {
		t.Fatalf("expected transactional event update to be rejected, got %v", err)
	}
}

func TestPreferenceServiceBuildsOneClickHeadersWithoutPlainEmail(t *testing.T) {
	svc, err := notificationsservice.NewPreferenceService(newFakePreferenceStore(), testMasterKey, "https://console.srapi.local/app")
	if err != nil {
		t.Fatalf("new preference service: %v", err)
	}
	headers, err := svc.OneClickHeaders("person@example.com", notificationscontract.TemplateAccountQuotaAlert)
	if err != nil {
		t.Fatalf("one-click headers: %v", err)
	}
	if headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
		t.Fatalf("unexpected one-click header: %+v", headers)
	}
	listHeader := headers["List-Unsubscribe"]
	if !strings.HasPrefix(listHeader, "<https://console.srapi.local/app/api/v1/notifications/unsubscribe?token=") || !strings.HasSuffix(listHeader, ">") {
		t.Fatalf("unexpected List-Unsubscribe header: %s", listHeader)
	}
	if strings.Contains(strings.ToLower(listHeader), "person@example.com") {
		t.Fatalf("List-Unsubscribe header leaked plaintext email: %s", listHeader)
	}
	transactional, err := svc.OneClickHeaders("person@example.com", notificationscontract.TemplateAuthEmailVerification)
	if err != nil {
		t.Fatalf("transactional one-click headers: %v", err)
	}
	if len(transactional) != 0 {
		t.Fatalf("transactional email should not include unsubscribe headers: %+v", transactional)
	}
}

type fakeSender struct {
	messages []notificationscontract.EmailMessage
}

func (s *fakeSender) Send(_ context.Context, message notificationscontract.EmailMessage) error {
	s.messages = append(s.messages, message)
	return nil
}

type fakePreferenceStore struct {
	values map[string]map[string]any
}

func newFakePreferenceStore() *fakePreferenceStore {
	return &fakePreferenceStore{values: map[string]map[string]any{}}
}

func (s *fakePreferenceStore) Get(_ context.Context, key string) (map[string]any, bool, error) {
	value, ok := s.values[key]
	if !ok {
		return nil, false, nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out, true, nil
}

func (s *fakePreferenceStore) Set(_ context.Context, key string, value map[string]any, _ *int) error {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	s.values[key] = out
	return nil
}

type fakeEventEnqueuer struct {
	events []eventscontract.OutboxEvent
}

func newFakeEventEnqueuer() *fakeEventEnqueuer {
	return &fakeEventEnqueuer{}
}

func (e *fakeEventEnqueuer) Enqueue(_ context.Context, req eventscontract.EnqueueRequest) (eventscontract.OutboxEvent, error) {
	event := eventscontract.OutboxEvent{
		ID:             len(e.events) + 1,
		EventType:      req.EventType,
		EventVersion:   req.EventVersion,
		ProducerModule: req.ProducerModule,
		AggregateType:  req.AggregateType,
		AggregateID:    req.AggregateID,
		IdempotencyKey: req.IdempotencyKey,
		Payload:        req.Payload,
		Metadata:       req.Metadata,
		CreatedAt:      time.Now().UTC(),
	}
	e.events = append(e.events, event)
	return event, nil
}

func verifyContactForTest(t *testing.T, svc *notificationsservice.ContactService, events *fakeEventEnqueuer, userID int, userName, userEmail, contactEmail string) notificationsservice.NotificationContact {
	t.Helper()
	result, err := svc.RequestVerification(context.Background(), notificationsservice.ContactVerificationRequest{
		UserID:       userID,
		UserName:     userName,
		UserEmail:    userEmail,
		ContactEmail: contactEmail,
	}, &userID)
	if err != nil {
		t.Fatalf("request contact verification: %v", err)
	}
	if len(events.events) == 0 {
		t.Fatal("expected contact verification event")
	}
	token := contactVerificationTokenFromEvent(t, events.events[len(events.events)-1])
	contact, err := svc.ConfirmVerification(context.Background(), userID, token, &userID)
	if err != nil {
		t.Fatalf("confirm contact verification: %v", err)
	}
	if !contact.Verified {
		t.Fatalf("expected verified contact: %+v", contact)
	}
	if result.Contact.ID != contact.ID {
		t.Fatalf("verification result contact changed: before=%+v after=%+v", result.Contact, contact)
	}
	return contact
}

func contactVerificationTokenFromEvent(t *testing.T, event eventscontract.OutboxEvent) string {
	t.Helper()
	token, err := notificationsservice.DecryptNotificationContactSecret(testMasterKey, fmt.Sprint(event.Payload["verification_token_ciphertext"]), notificationsservice.NotificationContactTokenAAD())
	if err != nil {
		t.Fatalf("decrypt contact verification token: %v", err)
	}
	return token
}

func encryptToken(t *testing.T, token, aad string) string {
	t.Helper()
	key, err := platformcrypto.DeriveAESKey(testMasterKey)
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new gcm: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("random nonce: %v", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(token), []byte(aad))
	return strings.Join([]string{
		"v1",
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(ciphertext),
	}, ":")
}

func emailHash(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}
