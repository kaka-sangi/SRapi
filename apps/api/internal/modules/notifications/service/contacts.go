package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
)

const (
	notificationContactKeyPrefix       = "notifications.email.contacts:v1:user"
	notificationContactMaxPerUser      = 3
	notificationContactIDBytes         = 16
	notificationContactTokenNonceBytes = 16
	notificationContactVerificationTTL = 24 * time.Hour
	notificationContactTokenVersionV1  = "v1"

	notificationContactEmailAAD = "notification.contact_verification.email:v1"
	notificationContactTokenAAD = "notification.contact_verification.token:v1"
)

var (
	ErrNotificationContactNotFound = errors.New("notification contact not found")
	ErrNotificationContactLimit    = errors.New("notification contact limit reached")
	ErrNotificationContactConflict = errors.New("notification contact conflict")
)

// ContactStore persists current-user notification contact state.
type ContactStore interface {
	Get(ctx context.Context, key string) (map[string]any, bool, error)
	Set(ctx context.Context, key string, value map[string]any, updatedBy *int) error
}

// ContactEventEnqueuer is the outbox subset needed for verification mail.
type ContactEventEnqueuer interface {
	Enqueue(ctx context.Context, req eventscontract.EnqueueRequest) (eventscontract.OutboxEvent, error)
}

// NotificationContact is a user-owned secondary notification email address.
type NotificationContact struct {
	ID         string
	Email      string
	EmailHash  string
	Verified   bool
	Disabled   bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
	VerifiedAt *time.Time
}

// ContactVerificationRequest starts ownership verification for a contact email.
type ContactVerificationRequest struct {
	UserID       int
	UserName     string
	UserEmail    string
	ContactEmail string
}

// ContactVerificationResult describes the pending or existing contact.
type ContactVerificationResult struct {
	Contact          NotificationContact
	VerificationSent bool
	ExpiresAt        *time.Time
}

// ContactService manages verified secondary notification recipients.
type ContactService struct {
	store         ContactStore
	events        ContactEventEnqueuer
	tokenKey      []byte
	publicBaseURL string
	now           func() time.Time
}

func NewContactService(store ContactStore, masterKey, publicBaseURL string, events ContactEventEnqueuer) (*ContactService, error) {
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
	return &ContactService{
		store:         store,
		events:        events,
		tokenKey:      key,
		publicBaseURL: publicBaseURL,
		now:           func() time.Time { return time.Now().UTC() },
	}, nil
}

func DecryptNotificationContactSecret(masterKey, ciphertextValue, aad string) (string, error) {
	key, err := platformcrypto.DeriveAESKey(masterKey)
	if err != nil {
		return "", ErrInvalidInput
	}
	parts := strings.Split(strings.TrimSpace(ciphertextValue), ":")
	if len(parts) != 3 || parts[0] != notificationContactTokenVersionV1 {
		return "", ErrInvalidInput
	}
	return decryptSecretWithKey(key, ciphertextValue, notificationContactTokenVersionV1, aad)
}

func NotificationContactEmailAAD() string {
	return notificationContactEmailAAD
}

func NotificationContactTokenAAD() string {
	return notificationContactTokenAAD
}

func (s *ContactService) ListContacts(ctx context.Context, userID int) ([]NotificationContact, error) {
	collection, err := s.loadCollection(ctx, userID)
	if err != nil {
		return nil, err
	}
	return cloneContacts(collection.Contacts), nil
}

func (s *ContactService) ListVerifiedContacts(ctx context.Context, userID int) ([]NotificationContact, error) {
	contacts, err := s.ListContacts(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]NotificationContact, 0, len(contacts))
	for _, contact := range contacts {
		if contact.Verified && !contact.Disabled {
			out = append(out, contact)
		}
	}
	return out, nil
}

func (s *ContactService) GetContact(ctx context.Context, userID int, contactID string) (NotificationContact, bool, error) {
	collection, err := s.loadCollection(ctx, userID)
	if err != nil {
		return NotificationContact{}, false, err
	}
	contactID = strings.TrimSpace(contactID)
	for _, contact := range collection.Contacts {
		if contact.ID == contactID {
			return contact.toPublic(), true, nil
		}
	}
	return NotificationContact{}, false, nil
}

func (s *ContactService) RequestVerification(ctx context.Context, req ContactVerificationRequest, updatedBy *int) (ContactVerificationResult, error) {
	if req.UserID <= 0 {
		return ContactVerificationResult{}, ErrInvalidInput
	}
	if s.events == nil || s.publicBaseURL == "" {
		return ContactVerificationResult{}, ErrNotConfigured
	}
	contactEmail, contactHash, ok := normalizeContactEmail(req.ContactEmail)
	if !ok {
		return ContactVerificationResult{}, ErrInvalidInput
	}
	userEmail, _, userEmailOK := normalizeContactEmail(req.UserEmail)
	if userEmailOK && contactEmail == userEmail {
		return ContactVerificationResult{}, ErrNotificationContactConflict
	}
	collection, err := s.loadCollection(ctx, req.UserID)
	if err != nil {
		return ContactVerificationResult{}, err
	}
	now := s.now().UTC()
	contactIdx := -1
	for idx, contact := range collection.Contacts {
		if contact.EmailHash == contactHash {
			contactIdx = idx
			break
		}
	}
	if contactIdx < 0 && len(collection.Contacts) >= notificationContactMaxPerUser {
		return ContactVerificationResult{}, ErrNotificationContactLimit
	}
	if contactIdx < 0 {
		contactID, err := randomContactID()
		if err != nil {
			return ContactVerificationResult{}, err
		}
		collection.Contacts = append(collection.Contacts, storedNotificationContact{
			ID:        contactID,
			Email:     contactEmail,
			EmailHash: contactHash,
			Verified:  false,
			Disabled:  false,
			CreatedAt: now,
			UpdatedAt: now,
		})
		contactIdx = len(collection.Contacts) - 1
	}
	contact := collection.Contacts[contactIdx]
	if contact.Verified {
		return ContactVerificationResult{Contact: contact.toPublic()}, nil
	}
	token, tokenHash, expiresAt, err := s.createVerificationToken(req.UserID, contact.ID, contactHash)
	if err != nil {
		return ContactVerificationResult{}, err
	}
	contact.Email = contactEmail
	contact.EmailHash = contactHash
	contact.Disabled = false
	contact.UpdatedAt = now
	contact.VerificationRequestedAt = &now
	contact.VerificationTokenHash = tokenHash
	contact.VerificationTokenExpiresAt = &expiresAt
	collection.Contacts[contactIdx] = contact
	if err := s.storeCollection(ctx, collection, updatedBy); err != nil {
		return ContactVerificationResult{}, err
	}
	emailCiphertext, err := s.encryptSecret(contactEmail, notificationContactEmailAAD)
	if err != nil {
		return ContactVerificationResult{}, err
	}
	tokenCiphertext, err := s.encryptSecret(token, notificationContactTokenAAD)
	if err != nil {
		return ContactVerificationResult{}, err
	}
	if _, err := s.events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      notificationscontract.EventNotificationContactVerificationRequested,
		EventVersion:   notificationContactTokenVersionV1,
		ProducerModule: "notifications",
		AggregateType:  "user",
		AggregateID:    strconv.Itoa(req.UserID),
		IdempotencyKey: "notifications.contact_verification:" + strconv.Itoa(req.UserID) + ":" + contact.ID + ":" + tokenHash[:16],
		Payload: map[string]any{
			"template":                      notificationscontract.TemplateNotificationContactVerification,
			"recipient_user_id":             req.UserID,
			"recipient_email_hash":          contactHash,
			"contact_id":                    contact.ID,
			"contact_email_ciphertext":      emailCiphertext,
			"verification_token_ciphertext": tokenCiphertext,
			"verification_token_version":    notificationContactTokenVersionV1,
			"verification_url_path":         "/notification-contacts/verify",
			"expires_at":                    expiresAt.Format(time.RFC3339Nano),
		},
		Metadata: map[string]any{
			"token_delivery": "encrypted_outbox",
		},
	}); err != nil {
		return ContactVerificationResult{}, err
	}
	return ContactVerificationResult{Contact: contact.toPublic(), VerificationSent: true, ExpiresAt: &expiresAt}, nil
}

func (s *ContactService) ConfirmVerification(ctx context.Context, userID int, token string, updatedBy *int) (NotificationContact, error) {
	if userID <= 0 {
		return NotificationContact{}, ErrInvalidInput
	}
	claims, tokenHash, err := s.parseVerificationToken(token)
	if err != nil {
		return NotificationContact{}, err
	}
	if claims.UserID != userID {
		return NotificationContact{}, ErrInvalidInput
	}
	collection, err := s.loadCollection(ctx, userID)
	if err != nil {
		return NotificationContact{}, err
	}
	now := s.now().UTC()
	for idx, contact := range collection.Contacts {
		if contact.ID != claims.ContactID {
			continue
		}
		if contact.Disabled || contact.EmailHash != claims.EmailHash || contact.VerificationTokenHash != tokenHash || contact.VerificationTokenExpiresAt == nil || !contact.VerificationTokenExpiresAt.After(now) {
			return NotificationContact{}, ErrInvalidInput
		}
		contact.Verified = true
		contact.VerifiedAt = &now
		contact.UpdatedAt = now
		contact.VerificationTokenHash = ""
		contact.VerificationTokenExpiresAt = nil
		contact.VerificationRequestedAt = nil
		collection.Contacts[idx] = contact
		if err := s.storeCollection(ctx, collection, updatedBy); err != nil {
			return NotificationContact{}, err
		}
		return contact.toPublic(), nil
	}
	return NotificationContact{}, ErrNotificationContactNotFound
}

func (s *ContactService) SetContactDisabled(ctx context.Context, userID int, contactID string, disabled bool, updatedBy *int) (NotificationContact, error) {
	collection, err := s.loadCollection(ctx, userID)
	if err != nil {
		return NotificationContact{}, err
	}
	now := s.now().UTC()
	for idx, contact := range collection.Contacts {
		if contact.ID != strings.TrimSpace(contactID) {
			continue
		}
		contact.Disabled = disabled
		contact.UpdatedAt = now
		collection.Contacts[idx] = contact
		if err := s.storeCollection(ctx, collection, updatedBy); err != nil {
			return NotificationContact{}, err
		}
		return contact.toPublic(), nil
	}
	return NotificationContact{}, ErrNotificationContactNotFound
}

func (s *ContactService) DeleteContact(ctx context.Context, userID int, contactID string, updatedBy *int) error {
	collection, err := s.loadCollection(ctx, userID)
	if err != nil {
		return err
	}
	contactID = strings.TrimSpace(contactID)
	out := make([]storedNotificationContact, 0, len(collection.Contacts))
	found := false
	for _, contact := range collection.Contacts {
		if contact.ID == contactID {
			found = true
			continue
		}
		out = append(out, contact)
	}
	if !found {
		return ErrNotificationContactNotFound
	}
	collection.Contacts = out
	return s.storeCollection(ctx, collection, updatedBy)
}

func (s *ContactService) VerificationURL(token string) (string, error) {
	if s.publicBaseURL == "" {
		return "", ErrNotConfigured
	}
	base, err := url.Parse(s.publicBaseURL)
	if err != nil {
		return "", ErrInvalidInput
	}
	action := *base
	action.Path = strings.TrimRight(base.Path, "/") + "/notification-contacts/verify"
	query := action.Query()
	query.Set("token", strings.TrimSpace(token))
	action.RawQuery = query.Encode()
	return action.String(), nil
}

func (s *ContactService) loadCollection(ctx context.Context, userID int) (notificationContactCollection, error) {
	if userID <= 0 {
		return notificationContactCollection{}, ErrInvalidInput
	}
	value, found, err := s.store.Get(ctx, notificationContactKey(userID))
	if err != nil {
		return notificationContactCollection{}, err
	}
	if !found {
		return notificationContactCollection{Version: "v1", UserID: userID}, nil
	}
	return contactCollectionFromMap(userID, value)
}

func (s *ContactService) storeCollection(ctx context.Context, collection notificationContactCollection, updatedBy *int) error {
	if collection.UserID <= 0 {
		return ErrInvalidInput
	}
	collection.Version = "v1"
	return s.store.Set(ctx, notificationContactKey(collection.UserID), collection.toMap(), updatedBy)
}

func (s *ContactService) createVerificationToken(userID int, contactID, emailHash string) (string, string, time.Time, error) {
	nonce := make([]byte, notificationContactTokenNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return "", "", time.Time{}, err
	}
	expiresAt := s.now().UTC().Add(notificationContactVerificationTTL)
	claims := notificationContactVerificationClaims{
		UserID:    userID,
		ContactID: strings.TrimSpace(contactID),
		EmailHash: emailHash,
		Nonce:     base64.RawURLEncoding.EncodeToString(nonce),
		ExpiresAt: expiresAt.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", "", time.Time{}, err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	token := encodedPayload + "." + s.sign(encodedPayload)
	return token, s.verificationTokenHash(token), expiresAt, nil
}

func (s *ContactService) parseVerificationToken(token string) (notificationContactVerificationClaims, string, error) {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return notificationContactVerificationClaims{}, "", ErrInvalidInput
	}
	if !hmac.Equal([]byte(parts[1]), []byte(s.sign(parts[0]))) {
		return notificationContactVerificationClaims{}, "", ErrInvalidInput
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return notificationContactVerificationClaims{}, "", ErrInvalidInput
	}
	var claims notificationContactVerificationClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return notificationContactVerificationClaims{}, "", ErrInvalidInput
	}
	if claims.UserID <= 0 || strings.TrimSpace(claims.ContactID) == "" || !validEmailHash(claims.EmailHash) || strings.TrimSpace(claims.Nonce) == "" {
		return notificationContactVerificationClaims{}, "", ErrInvalidInput
	}
	if claims.ExpiresAt <= s.now().UTC().Unix() {
		return notificationContactVerificationClaims{}, "", ErrInvalidInput
	}
	return claims, s.verificationTokenHash(token), nil
}

func (s *ContactService) verificationTokenHash(token string) string {
	mac := hmac.New(sha256.New, s.tokenKey)
	_, _ = mac.Write([]byte("notification_contact_verification:"))
	_, _ = mac.Write([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *ContactService) sign(payload string) string {
	mac := hmac.New(sha256.New, s.tokenKey)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *ContactService) encryptSecret(value, aad string) (string, error) {
	block, err := aes.NewCipher(s.tokenKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(value), []byte(aad))
	return strings.Join([]string{
		notificationContactTokenVersionV1,
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(ciphertext),
	}, ":"), nil
}

type notificationContactVerificationClaims struct {
	UserID    int    `json:"user_id"`
	ContactID string `json:"contact_id"`
	EmailHash string `json:"email_hash"`
	Nonce     string `json:"nonce"`
	ExpiresAt int64  `json:"exp"`
}

type notificationContactCollection struct {
	Version  string
	UserID   int
	Contacts []storedNotificationContact
}

func (c notificationContactCollection) toMap() map[string]any {
	contacts := make([]any, 0, len(c.Contacts))
	for _, contact := range c.Contacts {
		contacts = append(contacts, contact.toMap())
	}
	return map[string]any{
		"version":  "v1",
		"user_id":  c.UserID,
		"contacts": contacts,
	}
}

type storedNotificationContact struct {
	ID                         string
	Email                      string
	EmailHash                  string
	Verified                   bool
	Disabled                   bool
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	VerifiedAt                 *time.Time
	VerificationRequestedAt    *time.Time
	VerificationTokenHash      string
	VerificationTokenExpiresAt *time.Time
}

func (c storedNotificationContact) toPublic() NotificationContact {
	return NotificationContact{
		ID:         c.ID,
		Email:      c.Email,
		EmailHash:  c.EmailHash,
		Verified:   c.Verified,
		Disabled:   c.Disabled,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
		VerifiedAt: cloneContactTime(c.VerifiedAt),
	}
}

func (c storedNotificationContact) toMap() map[string]any {
	value := map[string]any{
		"id":         c.ID,
		"email":      c.Email,
		"email_hash": c.EmailHash,
		"verified":   c.Verified,
		"disabled":   c.Disabled,
		"created_at": c.CreatedAt.Format(time.RFC3339Nano),
		"updated_at": c.UpdatedAt.Format(time.RFC3339Nano),
		"token_hash": c.VerificationTokenHash,
	}
	if c.VerifiedAt != nil {
		value["verified_at"] = c.VerifiedAt.Format(time.RFC3339Nano)
	}
	if c.VerificationRequestedAt != nil {
		value["verification_requested_at"] = c.VerificationRequestedAt.Format(time.RFC3339Nano)
	}
	if c.VerificationTokenExpiresAt != nil {
		value["token_expires_at"] = c.VerificationTokenExpiresAt.Format(time.RFC3339Nano)
	}
	return value
}

func contactCollectionFromMap(userID int, value map[string]any) (notificationContactCollection, error) {
	collection := notificationContactCollection{Version: "v1", UserID: userID}
	if storedUserID := mapInt(value["user_id"]); storedUserID > 0 && storedUserID != userID {
		return notificationContactCollection{}, ErrInvalidInput
	}
	rawContacts, ok := value["contacts"].([]any)
	if !ok {
		return collection, nil
	}
	for _, raw := range rawContacts {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		contact := storedNotificationContact{
			ID:                    strings.TrimSpace(fmt.Sprint(item["id"])),
			Email:                 strings.TrimSpace(fmt.Sprint(item["email"])),
			EmailHash:             strings.TrimSpace(fmt.Sprint(item["email_hash"])),
			Verified:              mapBool(item["verified"]),
			Disabled:              mapBool(item["disabled"]),
			CreatedAt:             mapTime(item["created_at"]),
			UpdatedAt:             mapTime(item["updated_at"]),
			VerificationTokenHash: strings.TrimSpace(fmt.Sprint(item["token_hash"])),
		}
		if contact.ID == "" || !validEmailHash(contact.EmailHash) {
			continue
		}
		contact.VerifiedAt = mapOptionalTime(item["verified_at"])
		contact.VerificationRequestedAt = mapOptionalTime(item["verification_requested_at"])
		contact.VerificationTokenExpiresAt = mapOptionalTime(item["token_expires_at"])
		collection.Contacts = append(collection.Contacts, contact)
	}
	return collection, nil
}

func notificationContactKey(userID int) string {
	return notificationContactKeyPrefix + ":" + strconv.Itoa(userID)
}

func normalizeContactEmail(email string) (string, string, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > 254 || strings.ContainsAny(email, "\r\n\t ") || strings.Count(email, "@") != 1 {
		return "", "", false
	}
	local, domain, ok := strings.Cut(email, "@")
	if !ok || strings.TrimSpace(local) == "" || strings.TrimSpace(domain) == "" || strings.Contains(domain, "..") {
		return "", "", false
	}
	sum := sha256.Sum256([]byte(email))
	return email, hex.EncodeToString(sum[:]), true
}

func randomContactID() (string, error) {
	buf := make([]byte, notificationContactIDBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "nct_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func cloneContacts(items []storedNotificationContact) []NotificationContact {
	out := make([]NotificationContact, 0, len(items))
	for _, item := range items {
		out = append(out, item.toPublic())
	}
	return out
}

func mapBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(value)), "true")
	}
}

func mapInt(value any) int {
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

func mapTime(value any) time.Time {
	if parsed := mapOptionalTime(value); parsed != nil {
		return *parsed
	}
	return time.Time{}
}

func mapOptionalTime(value any) *time.Time {
	raw := strings.TrimSpace(fmt.Sprint(value))
	if raw == "" || raw == "<nil>" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, raw)
	}
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func cloneContactTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := value.UTC()
	return &clone
}
