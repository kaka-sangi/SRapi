package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entauthsession "github.com/srapi/srapi/apps/api/ent/authsession"
	entemailverificationtoken "github.com/srapi/srapi/apps/api/ent/emailverificationtoken"
	entpasswordresettoken "github.com/srapi/srapi/apps/api/ent/passwordresettoken"
	entpendingoauthsession "github.com/srapi/srapi/apps/api/ent/pendingoauthsession"
	"github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

var (
	ErrInvalidStore    = errors.New("invalid auth ent store")
	ErrSessionNotFound = errors.New("auth session not found")
)

const (
	sessionStatusActive  = "active"
	sessionStatusExpired = "expired"
	sessionStatusRevoked = "revoked"
)

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) Create(ctx context.Context, input contract.CreateSession) (contract.Session, error) {
	sessionID := strings.TrimSpace(input.ID)
	csrfToken := strings.TrimSpace(input.CSRFToken)
	if sessionID == "" || csrfToken == "" || input.UserID <= 0 || input.ExpiresAt.IsZero() {
		return contract.Session{}, ErrInvalidStore
	}

	create := s.client.AuthSession.Create().
		SetSessionIDHash(tokenHash(sessionID)).
		SetCsrfTokenHash(tokenHash(csrfToken)).
		SetUserID(input.UserID).
		SetExpiresAt(input.ExpiresAt).
		SetStatus(sessionStatusActive)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		return contract.Session{}, err
	}
	return contract.Session{
		ID:        sessionID,
		UserID:    created.UserID,
		CSRFToken: csrfToken,
		ExpiresAt: created.ExpiresAt,
		CreatedAt: created.CreatedAt,
	}, nil
}

func (s *Store) FindByID(ctx context.Context, id string) (contract.Session, error) {
	sessionID := strings.TrimSpace(id)
	if sessionID == "" {
		return contract.Session{}, ErrInvalidStore
	}
	found, err := s.client.AuthSession.Query().
		Where(
			entauthsession.SessionIDHashEQ(tokenHash(sessionID)),
			entauthsession.StatusEQ(sessionStatusActive),
			entauthsession.DeletedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		return contract.Session{}, err
	}
	return toSession(sessionID, found), nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	sessionID := strings.TrimSpace(id)
	if sessionID == "" {
		return nil
	}
	now := time.Now().UTC()
	_, err := s.client.AuthSession.Update().
		Where(
			entauthsession.SessionIDHashEQ(tokenHash(sessionID)),
			entauthsession.DeletedAtIsNil(),
		).
		SetStatus(sessionStatusRevoked).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (s *Store) DeleteByUserID(ctx context.Context, userID int) error {
	if userID <= 0 {
		return ErrInvalidStore
	}
	now := time.Now().UTC()
	_, err := s.client.AuthSession.Update().
		Where(
			entauthsession.UserIDEQ(userID),
			entauthsession.StatusEQ(sessionStatusActive),
			entauthsession.DeletedAtIsNil(),
		).
		SetStatus(sessionStatusRevoked).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (s *Store) UpdateCSRFToken(ctx context.Context, sessionID string, newToken string) error {
	sessionID = strings.TrimSpace(sessionID)
	newToken = strings.TrimSpace(newToken)
	if sessionID == "" || newToken == "" {
		return ErrInvalidStore
	}
	now := time.Now().UTC()
	affected, err := s.client.AuthSession.Update().
		Where(
			entauthsession.SessionIDHashEQ(tokenHash(sessionID)),
			entauthsession.StatusEQ(sessionStatusActive),
			entauthsession.DeletedAtIsNil(),
		).
		SetCsrfTokenHash(tokenHash(newToken)).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (s *Store) Touch(ctx context.Context, id string, at time.Time) error {
	sessionID := strings.TrimSpace(id)
	if sessionID == "" || at.IsZero() {
		return ErrInvalidStore
	}
	affected, err := s.client.AuthSession.Update().
		Where(
			entauthsession.SessionIDHashEQ(tokenHash(sessionID)),
			entauthsession.StatusEQ(sessionStatusActive),
			entauthsession.DeletedAtIsNil(),
		).
		SetLastActiveAt(at).
		SetUpdatedAt(at).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrSessionNotFound
	}
	return err
}

func (s *Store) CreatePasswordResetToken(ctx context.Context, input contract.CreatePasswordResetToken) (contract.PasswordResetToken, error) {
	tokenHash := strings.TrimSpace(input.TokenHash)
	if input.UserID <= 0 || tokenHash == "" || input.ExpiresAt.IsZero() {
		return contract.PasswordResetToken{}, ErrInvalidStore
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	created, err := s.client.PasswordResetToken.Create().
		SetUserID(input.UserID).
		SetTokenHash(tokenHash).
		SetTokenVersion("v1").
		SetExpiresAt(input.ExpiresAt).
		SetCreatedAt(createdAt).
		SetUpdatedAt(createdAt).
		Save(ctx)
	if err != nil {
		return contract.PasswordResetToken{}, err
	}
	return toPasswordResetToken(created), nil
}

func (s *Store) ConsumePasswordResetToken(ctx context.Context, tokenHash string, now time.Time) (contract.PasswordResetToken, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return contract.PasswordResetToken{}, ErrInvalidStore
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	updated, err := s.client.PasswordResetToken.Update().
		Where(
			entpasswordresettoken.TokenHashEQ(tokenHash),
			entpasswordresettoken.UsedAtIsNil(),
			entpasswordresettoken.ExpiresAtGT(now),
		).
		SetUsedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.PasswordResetToken{}, err
	}
	if updated == 0 {
		return contract.PasswordResetToken{}, ErrSessionNotFound
	}
	found, err := s.client.PasswordResetToken.Query().
		Where(entpasswordresettoken.TokenHashEQ(tokenHash)).
		Only(ctx)
	if err != nil {
		return contract.PasswordResetToken{}, err
	}
	return toPasswordResetToken(found), nil
}

func (s *Store) CreateEmailVerificationToken(ctx context.Context, input contract.CreateEmailVerificationToken) (contract.EmailVerificationToken, error) {
	tokenHash := strings.TrimSpace(input.TokenHash)
	if input.UserID <= 0 || tokenHash == "" || input.ExpiresAt.IsZero() {
		return contract.EmailVerificationToken{}, ErrInvalidStore
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	created, err := s.client.EmailVerificationToken.Create().
		SetUserID(input.UserID).
		SetTokenHash(tokenHash).
		SetTokenVersion("v1").
		SetExpiresAt(input.ExpiresAt).
		SetCreatedAt(createdAt).
		SetUpdatedAt(createdAt).
		Save(ctx)
	if err != nil {
		return contract.EmailVerificationToken{}, err
	}
	return toEmailVerificationToken(created), nil
}

func (s *Store) ConsumeEmailVerificationToken(ctx context.Context, tokenHash string, now time.Time) (contract.EmailVerificationToken, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return contract.EmailVerificationToken{}, ErrInvalidStore
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	updated, err := s.client.EmailVerificationToken.Update().
		Where(
			entemailverificationtoken.TokenHashEQ(tokenHash),
			entemailverificationtoken.UsedAtIsNil(),
			entemailverificationtoken.ExpiresAtGT(now),
		).
		SetUsedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.EmailVerificationToken{}, err
	}
	if updated == 0 {
		return contract.EmailVerificationToken{}, ErrSessionNotFound
	}
	found, err := s.client.EmailVerificationToken.Query().
		Where(entemailverificationtoken.TokenHashEQ(tokenHash)).
		Only(ctx)
	if err != nil {
		return contract.EmailVerificationToken{}, err
	}
	return toEmailVerificationToken(found), nil
}

func (s *Store) CreatePendingOAuthSession(ctx context.Context, input contract.CreatePendingOAuthSession) (contract.PendingOAuthSession, error) {
	sessionTokenHash := strings.TrimSpace(input.SessionTokenHash)
	provider := strings.TrimSpace(string(input.Provider))
	providerKey := strings.TrimSpace(input.ProviderKey)
	subjectHash := strings.TrimSpace(input.ProviderSubjectHash)
	if sessionTokenHash == "" || input.Intent == "" || provider == "" || providerKey == "" || subjectHash == "" || input.ExpiresAt.IsZero() {
		return contract.PendingOAuthSession{}, ErrInvalidStore
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	created, err := s.client.PendingOAuthSession.Create().
		SetSessionTokenHash(sessionTokenHash).
		SetIntent(string(input.Intent)).
		SetProvider(provider).
		SetProviderKey(providerKey).
		SetProviderSubjectHash(subjectHash).
		SetSubjectHint(strings.TrimSpace(input.SubjectHint)).
		SetNillableTargetUserID(input.TargetUserID).
		SetRedirectTo(strings.TrimSpace(input.RedirectTo)).
		SetResolvedEmail(strings.ToLower(strings.TrimSpace(input.ResolvedEmail))).
		SetDisplayName(strings.TrimSpace(input.DisplayName)).
		SetEmailVerified(input.EmailVerified).
		SetAvatarURL(strings.TrimSpace(input.AvatarURL)).
		SetExpiresAt(input.ExpiresAt).
		SetCreatedAt(createdAt).
		SetUpdatedAt(createdAt).
		Save(ctx)
	if err != nil {
		return contract.PendingOAuthSession{}, err
	}
	return toPendingOAuthSession(created), nil
}

func (s *Store) ConsumePendingOAuthSession(ctx context.Context, sessionTokenHash string, now time.Time) (contract.PendingOAuthSession, error) {
	sessionTokenHash = strings.TrimSpace(sessionTokenHash)
	if sessionTokenHash == "" {
		return contract.PendingOAuthSession{}, ErrInvalidStore
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	updated, err := s.client.PendingOAuthSession.Update().
		Where(
			entpendingoauthsession.SessionTokenHashEQ(sessionTokenHash),
			entpendingoauthsession.ConsumedAtIsNil(),
			entpendingoauthsession.ExpiresAtGT(now),
		).
		SetConsumedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.PendingOAuthSession{}, err
	}
	if updated == 0 {
		return contract.PendingOAuthSession{}, ErrSessionNotFound
	}
	found, err := s.client.PendingOAuthSession.Query().
		Where(entpendingoauthsession.SessionTokenHashEQ(sessionTokenHash)).
		Only(ctx)
	if err != nil {
		return contract.PendingOAuthSession{}, err
	}
	return toPendingOAuthSession(found), nil
}

func (s *Store) FindPendingOAuthSession(ctx context.Context, sessionTokenHash string, now time.Time) (contract.PendingOAuthSession, error) {
	sessionTokenHash = strings.TrimSpace(sessionTokenHash)
	if sessionTokenHash == "" {
		return contract.PendingOAuthSession{}, ErrInvalidStore
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	found, err := s.client.PendingOAuthSession.Query().
		Where(
			entpendingoauthsession.SessionTokenHashEQ(sessionTokenHash),
			entpendingoauthsession.ConsumedAtIsNil(),
			entpendingoauthsession.ExpiresAtGT(now),
		).
		Only(ctx)
	if err != nil {
		return contract.PendingOAuthSession{}, err
	}
	return toPendingOAuthSession(found), nil
}

func (s *Store) CompletePendingOAuthEmail(ctx context.Context, sessionTokenHash string, email string, now time.Time) (contract.PendingOAuthSession, error) {
	sessionTokenHash = strings.TrimSpace(sessionTokenHash)
	email = strings.ToLower(strings.TrimSpace(email))
	if sessionTokenHash == "" || email == "" {
		return contract.PendingOAuthSession{}, ErrInvalidStore
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	updated, err := s.client.PendingOAuthSession.Update().
		Where(
			entpendingoauthsession.SessionTokenHashEQ(sessionTokenHash),
			entpendingoauthsession.ResolvedEmailEQ(""),
			entpendingoauthsession.ConsumedAtIsNil(),
			entpendingoauthsession.ExpiresAtGT(now),
		).
		SetResolvedEmail(email).
		SetEmailVerified(true).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.PendingOAuthSession{}, err
	}
	if updated == 0 {
		return contract.PendingOAuthSession{}, ErrSessionNotFound
	}
	found, err := s.client.PendingOAuthSession.Query().
		Where(entpendingoauthsession.SessionTokenHashEQ(sessionTokenHash)).
		Only(ctx)
	if err != nil {
		return contract.PendingOAuthSession{}, err
	}
	return toPendingOAuthSession(found), nil
}

func (s *Store) CleanupExpiredSessions(ctx context.Context, now time.Time) (contract.CleanupExpiredSessionsResult, error) {
	if now.IsZero() {
		return contract.CleanupExpiredSessionsResult{}, ErrInvalidStore
	}
	now = now.UTC()
	query := s.client.AuthSession.Query().
		Where(
			entauthsession.StatusEQ(sessionStatusActive),
			entauthsession.ExpiresAtLTE(now),
			entauthsession.DeletedAtIsNil(),
		)
	selected, err := query.Count(ctx)
	if err != nil {
		return contract.CleanupExpiredSessionsResult{}, err
	}
	if selected == 0 {
		return contract.CleanupExpiredSessionsResult{}, nil
	}
	expired, err := s.client.AuthSession.Update().
		Where(
			entauthsession.StatusEQ(sessionStatusActive),
			entauthsession.ExpiresAtLTE(now),
			entauthsession.DeletedAtIsNil(),
		).
		SetStatus(sessionStatusExpired).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.CleanupExpiredSessionsResult{}, err
	}
	return contract.CleanupExpiredSessionsResult{Selected: selected, Expired: expired}, nil
}

func toSession(rawSessionID string, row *ent.AuthSession) contract.Session {
	return contract.Session{
		ID:         rawSessionID,
		UserID:     row.UserID,
		CSRFToken:  hashedTokenMarker(row.CsrfTokenHash),
		ExpiresAt:  row.ExpiresAt,
		CreatedAt:  row.CreatedAt,
		LastSeenAt: row.LastActiveAt,
	}
}

func toPasswordResetToken(row *ent.PasswordResetToken) contract.PasswordResetToken {
	return contract.PasswordResetToken{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
		UsedAt:    row.UsedAt,
		CreatedAt: row.CreatedAt,
	}
}

func toEmailVerificationToken(row *ent.EmailVerificationToken) contract.EmailVerificationToken {
	return contract.EmailVerificationToken{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
		UsedAt:    row.UsedAt,
		CreatedAt: row.CreatedAt,
	}
}

func toPendingOAuthSession(row *ent.PendingOAuthSession) contract.PendingOAuthSession {
	return contract.PendingOAuthSession{
		ID:                  row.ID,
		SessionTokenHash:    row.SessionTokenHash,
		Intent:              contract.PendingOAuthIntent(row.Intent),
		Provider:            userscontract.AuthIdentityProvider(row.Provider),
		ProviderKey:         row.ProviderKey,
		ProviderSubjectHash: row.ProviderSubjectHash,
		SubjectHint:         row.SubjectHint,
		TargetUserID:        cloneInt(row.TargetUserID),
		RedirectTo:          row.RedirectTo,
		ResolvedEmail:       row.ResolvedEmail,
		DisplayName:         row.DisplayName,
		EmailVerified:       row.EmailVerified,
		AvatarURL:           row.AvatarURL,
		ExpiresAt:           row.ExpiresAt,
		ConsumedAt:          row.ConsumedAt,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func tokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func hashedTokenMarker(hash string) string {
	return "sha256:" + strings.TrimSpace(hash)
}
