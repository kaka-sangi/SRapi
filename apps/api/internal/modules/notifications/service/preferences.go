package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
)

const (
	emailPreferencePrefix = "notifications.email.preference:v1"
	unsubscribeTokenTTL   = 30 * 24 * time.Hour
)

var ErrUnsupportedNotificationEvent = errors.New("unsupported notification event")

// PreferenceStore persists public notification preference state.
type PreferenceStore interface {
	Get(ctx context.Context, key string) (map[string]any, bool, error)
	Set(ctx context.Context, key string, value map[string]any, updatedBy *int) error
}

// PreferenceService manages optional notification preferences and unsubscribe tokens.
type PreferenceService struct {
	store         PreferenceStore
	tokenKey      []byte
	publicBaseURL string
	now           func() time.Time
}

type UnsubscribePreview struct {
	Event string
	Done  bool
}

// EmailPreference is the current subscription state for one optional event.
type EmailPreference struct {
	Event       string
	Label       string
	Description string
	Category    string
	Subscribed  bool
	UpdatedAt   *time.Time
}

func NewPreferenceService(store PreferenceStore, masterKey, publicBaseURL string) (*PreferenceService, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	key, err := platformcrypto.DeriveAESKey(masterKey)
	if err != nil {
		return nil, ErrInvalidInput
	}
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if publicBaseURL != "" && !validNotificationBaseURL(publicBaseURL) {
		return nil, ErrInvalidInput
	}
	return &PreferenceService{
		store:         store,
		tokenKey:      key,
		publicBaseURL: publicBaseURL,
		now:           func() time.Time { return time.Now().UTC() },
	}, nil
}

func (s *PreferenceService) PreviewUnsubscribe(token string) (UnsubscribePreview, error) {
	claims, err := s.parseUnsubscribeToken(token)
	if err != nil {
		return UnsubscribePreview{}, err
	}
	return UnsubscribePreview{Event: claims.Event, Done: false}, nil
}

func (s *PreferenceService) Unsubscribe(ctx context.Context, token string) (UnsubscribePreview, error) {
	claims, err := s.parseUnsubscribeToken(token)
	if err != nil {
		return UnsubscribePreview{}, err
	}
	if err := s.setPreferenceByHash(ctx, claims.EmailHash, claims.Event, false, "one_click", nil); err != nil {
		return UnsubscribePreview{}, err
	}
	return UnsubscribePreview{Event: claims.Event, Done: true}, nil
}

func (s *PreferenceService) ListPreferences(ctx context.Context, email string) ([]EmailPreference, error) {
	hash, ok := emailPreferenceHash(email)
	if !ok {
		return nil, ErrInvalidInput
	}
	events := optionalEmailPreferenceEvents()
	items := make([]EmailPreference, 0, len(events))
	for _, event := range events {
		value, found, err := s.store.Get(ctx, emailPreferenceKey(event.Event, hash))
		if err != nil {
			return nil, err
		}
		items = append(items, preferenceFromStoredValue(event, value, found))
	}
	return items, nil
}

func (s *PreferenceService) SetPreference(ctx context.Context, email, event string, subscribed bool, source string, updatedBy *int) (EmailPreference, error) {
	hash, ok := emailPreferenceHash(email)
	if !ok {
		return EmailPreference{}, ErrInvalidInput
	}
	if err := s.setPreferenceByHash(ctx, hash, event, subscribed, source, updatedBy); err != nil {
		return EmailPreference{}, err
	}
	info, ok := optionalEmailPreferenceEventByName(event)
	if !ok {
		return EmailPreference{}, ErrUnsupportedNotificationEvent
	}
	value, found, err := s.store.Get(ctx, emailPreferenceKey(info.Event, hash))
	if err != nil {
		return EmailPreference{}, err
	}
	return preferenceFromStoredValue(info, value, found), nil
}

func (s *PreferenceService) IsUnsubscribed(ctx context.Context, email, event string) (bool, error) {
	event = strings.TrimSpace(event)
	if !IsOptionalEmailEvent(event) {
		return false, nil
	}
	hash, ok := emailPreferenceHash(email)
	if !ok {
		return false, ErrInvalidInput
	}
	value, found, err := s.store.Get(ctx, emailPreferenceKey(event, hash))
	if err != nil || !found {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(value["status"])), "unsubscribed"), nil
}

func (s *PreferenceService) UnsubscribeURL(email, event string) (string, error) {
	token, err := s.CreateUnsubscribeToken(email, event)
	if err != nil {
		return "", err
	}
	if s.publicBaseURL == "" {
		return "", ErrNotConfigured
	}
	base, err := url.Parse(s.publicBaseURL)
	if err != nil {
		return "", ErrInvalidInput
	}
	action := *base
	path := "/api/v1/notifications/unsubscribe"
	action.Path = strings.TrimRight(base.Path, "/") + path
	query := action.Query()
	query.Set("token", token)
	action.RawQuery = query.Encode()
	return action.String(), nil
}

func (s *PreferenceService) OneClickHeaders(email, event string) (map[string]string, error) {
	if !IsOptionalEmailEvent(event) {
		return map[string]string{}, nil
	}
	unsubscribeURL, err := s.UnsubscribeURL(email, event)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"List-Unsubscribe":      "<" + unsubscribeURL + ">",
		"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
	}, nil
}

func (s *PreferenceService) CreateUnsubscribeToken(email, event string) (string, error) {
	event = strings.TrimSpace(event)
	if !IsOptionalEmailEvent(event) {
		return "", ErrUnsupportedNotificationEvent
	}
	hash, ok := emailPreferenceHash(email)
	if !ok {
		return "", ErrInvalidInput
	}
	claims := unsubscribeClaims{
		EmailHash: hash,
		Event:     event,
		ExpiresAt: s.now().UTC().Add(unsubscribeTokenTTL).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := s.sign(encodedPayload)
	return encodedPayload + "." + signature, nil
}

func (s *PreferenceService) setPreferenceByHash(ctx context.Context, emailHash, event string, subscribed bool, source string, updatedBy *int) error {
	event = strings.TrimSpace(event)
	if !IsOptionalEmailEvent(event) || !validEmailHash(emailHash) {
		return ErrUnsupportedNotificationEvent
	}
	status := "subscribed"
	if !subscribed {
		status = "unsubscribed"
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "current_user"
	}
	value := map[string]any{
		"status":     status,
		"event":      event,
		"email_hash": emailHash,
		"updated_at": s.now().UTC().Format(time.RFC3339Nano),
		"source":     source,
	}
	return s.store.Set(ctx, emailPreferenceKey(event, emailHash), value, updatedBy)
}

func IsOptionalEmailEvent(event string) bool {
	switch strings.TrimSpace(event) {
	case notificationscontract.TemplateBalanceLow, notificationscontract.TemplateSubscriptionExpiry, notificationscontract.TemplateAccountQuotaAlert:
		return true
	default:
		return false
	}
}

func IsTransactionalEmailEvent(event string) bool {
	switch strings.TrimSpace(event) {
	case notificationscontract.TemplateAuthPasswordReset, notificationscontract.TemplateAuthEmailVerification, notificationscontract.TemplateNotificationContactVerification:
		return true
	default:
		return false
	}
}

func optionalEmailPreferenceEvents() []EmailTemplateEventInfo {
	events := make([]EmailTemplateEventInfo, 0, len(emailTemplateDefinitions))
	for _, def := range emailTemplateDefinitions {
		if def.Optional {
			events = append(events, eventInfoFromDefinition(def))
		}
	}
	return events
}

func optionalEmailPreferenceEventByName(event string) (EmailTemplateEventInfo, bool) {
	event = strings.TrimSpace(event)
	for _, info := range optionalEmailPreferenceEvents() {
		if info.Event == event {
			return info, true
		}
	}
	return EmailTemplateEventInfo{}, false
}

func preferenceFromStoredValue(info EmailTemplateEventInfo, value map[string]any, found bool) EmailPreference {
	item := EmailPreference{
		Event:       info.Event,
		Label:       info.Label,
		Description: info.Description,
		Category:    info.Category,
		Subscribed:  true,
	}
	if !found {
		return item
	}
	item.Subscribed = !strings.EqualFold(strings.TrimSpace(fmt.Sprint(value["status"])), "unsubscribed")
	if updatedAt, ok := parsePreferenceUpdatedAt(value["updated_at"]); ok {
		item.UpdatedAt = &updatedAt
	}
	return item
}

func parsePreferenceUpdatedAt(value any) (time.Time, bool) {
	raw := strings.TrimSpace(fmt.Sprint(value))
	if raw == "" || raw == "<nil>" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

type unsubscribeClaims struct {
	EmailHash string `json:"email_hash"`
	Event     string `json:"event"`
	ExpiresAt int64  `json:"exp"`
}

func (s *PreferenceService) parseUnsubscribeToken(token string) (unsubscribeClaims, error) {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return unsubscribeClaims{}, ErrInvalidInput
	}
	if !hmac.Equal([]byte(parts[1]), []byte(s.sign(parts[0]))) {
		return unsubscribeClaims{}, ErrInvalidInput
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return unsubscribeClaims{}, ErrInvalidInput
	}
	var claims unsubscribeClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return unsubscribeClaims{}, ErrInvalidInput
	}
	if !IsOptionalEmailEvent(claims.Event) || !validEmailHash(claims.EmailHash) {
		return unsubscribeClaims{}, ErrUnsupportedNotificationEvent
	}
	if claims.ExpiresAt <= s.now().UTC().Unix() {
		return unsubscribeClaims{}, ErrInvalidInput
	}
	return claims, nil
}

func (s *PreferenceService) sign(payload string) string {
	mac := hmac.New(sha256.New, s.tokenKey)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func emailPreferenceHash(email string) (string, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return "", false
	}
	sum := sha256.Sum256([]byte(email))
	return hex.EncodeToString(sum[:]), true
}

func emailPreferenceKey(event, emailHash string) string {
	return emailPreferencePrefix + ":" + strings.TrimSpace(event) + ":" + strings.TrimSpace(emailHash)
}

func validEmailHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func validNotificationBaseURL(value string) bool {
	base, err := url.Parse(value)
	return err == nil && (base.Scheme == "http" || base.Scheme == "https") && base.Host != "" && base.RawQuery == "" && base.Fragment == ""
}
