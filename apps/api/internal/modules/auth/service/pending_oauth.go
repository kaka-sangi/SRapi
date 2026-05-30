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
	"strconv"
	"strings"
	"time"

	authcontract "github.com/srapi/srapi/apps/api/internal/modules/auth/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

const (
	pendingOAuthTokenBytes              = 32
	pendingOAuthEmailCompletionTTL      = 15 * time.Minute
	pendingOAuthTTL                     = 15 * time.Minute
	pendingOAuthChallengeVersion        = "oauth-bind-v2"
	pendingOAuthActionVersion           = "oauth-action-v1"
	pendingOAuthEmailCompletionTokenV1  = "v1"
	pendingOAuthEmailCompletionURLPath  = "/oauth/pending/email-completion"
	pendingOAuthEmailCompletionTemplate = "auth.oauth_pending_email_completion"
)

type PendingOAuthProfile struct {
	SubjectHint   string
	ResolvedEmail string
	DisplayName   string
	EmailVerified bool
	AvatarURL     string
}

type CreatePendingOAuthSessionRequest struct {
	Intent              authcontract.PendingOAuthIntent
	Provider            userscontract.AuthIdentityProvider
	ProviderKey         string
	ProviderSubjectHash string
	TargetUserID        *int
	RedirectTo          string
	Profile             PendingOAuthProfile
}

type CreatePendingOAuthSessionResult struct {
	SessionToken string
	Session      authcontract.PendingOAuthSession
}

type PendingOAuthBindLoginResult struct {
	User                       userscontract.StoredUser
	RequiresSecondFactor       bool
	SecondFactorChallengeID    string
	SecondFactorChallengeUntil *time.Time
}

type CompletePendingOAuthBindLoginSecondFactorResult struct {
	User             userscontract.StoredUser
	Session          authcontract.PendingOAuthSession
	AdoptDisplayName bool
}

type PendingOAuthActionToken struct {
	Token     string
	ExpiresAt time.Time
}

type PendingOAuthEmailCompletionRequestResult struct {
	Accepted  bool
	ExpiresAt time.Time
}

func (s *Service) CreatePendingOAuthSession(ctx context.Context, req CreatePendingOAuthSessionRequest) (CreatePendingOAuthSessionResult, error) {
	store, ok := s.sessions.(authcontract.PendingOAuthStore)
	if !ok {
		return CreatePendingOAuthSessionResult{}, ErrPendingOAuthUnavailable
	}
	intent := normalizePendingOAuthIntent(req.Intent)
	provider := normalizeAuthIdentityProvider(req.Provider)
	providerKey := strings.TrimSpace(req.ProviderKey)
	subjectHash := strings.TrimSpace(req.ProviderSubjectHash)
	if intent == "" || provider == "" || providerKey == "" || subjectHash == "" {
		return CreatePendingOAuthSessionResult{}, ErrInvalidInput
	}
	if len(s.resetTokenKey) == 0 {
		return CreatePendingOAuthSessionResult{}, ErrPendingOAuthUnavailable
	}
	if req.TargetUserID != nil && *req.TargetUserID <= 0 {
		return CreatePendingOAuthSessionResult{}, ErrInvalidInput
	}
	sessionToken, err := randomToken("oauth_pending", pendingOAuthTokenBytes)
	if err != nil {
		return CreatePendingOAuthSessionResult{}, err
	}
	now := s.clock.Now()
	session, err := store.CreatePendingOAuthSession(ctx, authcontract.CreatePendingOAuthSession{
		SessionTokenHash:    s.pendingOAuthSessionTokenHash(sessionToken),
		Intent:              intent,
		Provider:            provider,
		ProviderKey:         providerKey,
		ProviderSubjectHash: subjectHash,
		SubjectHint:         strings.TrimSpace(req.Profile.SubjectHint),
		TargetUserID:        cloneInt(req.TargetUserID),
		RedirectTo:          normalizePendingOAuthRedirect(req.RedirectTo),
		ResolvedEmail:       strings.ToLower(strings.TrimSpace(req.Profile.ResolvedEmail)),
		DisplayName:         strings.TrimSpace(req.Profile.DisplayName),
		EmailVerified:       req.Profile.EmailVerified,
		AvatarURL:           strings.TrimSpace(req.Profile.AvatarURL),
		ExpiresAt:           now.Add(pendingOAuthTTL),
		CreatedAt:           now,
	})
	if err != nil {
		return CreatePendingOAuthSessionResult{}, err
	}
	return CreatePendingOAuthSessionResult{SessionToken: sessionToken, Session: session}, nil
}

func (s *Service) ConsumePendingOAuthSession(ctx context.Context, sessionToken string) (authcontract.PendingOAuthSession, error) {
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return authcontract.PendingOAuthSession{}, ErrInvalidInput
	}
	if len(s.resetTokenKey) == 0 {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthUnavailable
	}
	store, ok := s.sessions.(authcontract.PendingOAuthStore)
	if !ok {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthUnavailable
	}
	session, err := store.ConsumePendingOAuthSession(ctx, s.pendingOAuthSessionTokenHash(sessionToken), s.clock.Now())
	if err != nil {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthInvalid
	}
	return session, nil
}

func (s *Service) FindPendingOAuthSession(ctx context.Context, sessionToken string) (authcontract.PendingOAuthSession, error) {
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return authcontract.PendingOAuthSession{}, ErrInvalidInput
	}
	if len(s.resetTokenKey) == 0 {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthUnavailable
	}
	store, ok := s.sessions.(authcontract.PendingOAuthStore)
	if !ok {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthUnavailable
	}
	session, err := store.FindPendingOAuthSession(ctx, s.pendingOAuthSessionTokenHash(sessionToken), s.clock.Now())
	if err != nil {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthInvalid
	}
	return session, nil
}

func (s *Service) RequestPendingOAuthEmailCompletion(ctx context.Context, sessionToken string, email string) (PendingOAuthEmailCompletionRequestResult, error) {
	session, err := s.FindPendingOAuthSession(ctx, sessionToken)
	if err != nil {
		return PendingOAuthEmailCompletionRequestResult{}, err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if session.Intent != authcontract.PendingOAuthIntentLogin || session.TargetUserID != nil || session.ResolvedEmail != "" || !validEmailAddress(email) {
		return PendingOAuthEmailCompletionRequestResult{}, ErrInvalidInput
	}
	if len(s.resetTokenKey) == 0 || s.events == nil {
		return PendingOAuthEmailCompletionRequestResult{}, ErrPendingOAuthUnavailable
	}
	rawToken, err := randomToken("oauth_email", emailVerificationTokenBytes)
	if err != nil {
		return PendingOAuthEmailCompletionRequestResult{}, err
	}
	now := s.clock.Now()
	expiresAt := now.Add(pendingOAuthEmailCompletionTTL)
	tokenPayload := strings.Join([]string{
		strconv.Itoa(session.ID),
		email,
		strconv.FormatInt(expiresAt.Unix(), 10),
		rawToken,
	}, ":")
	tokenCiphertext, err := s.encryptPendingOAuthEmailCompletionToken(tokenPayload)
	if err != nil {
		return PendingOAuthEmailCompletionRequestResult{}, err
	}
	emailCiphertext, err := s.encryptPendingOAuthEmailCompletionEmail(email)
	if err != nil {
		return PendingOAuthEmailCompletionRequestResult{}, err
	}
	if _, err := s.events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "PendingOAuthEmailCompletionRequested",
		EventVersion:   pendingOAuthEmailCompletionTokenV1,
		ProducerModule: "auth",
		AggregateType:  "pending_oauth_session",
		AggregateID:    strconv.Itoa(session.ID),
		IdempotencyKey: "auth.pending_oauth_email:" + strconv.Itoa(session.ID) + ":" + s.pendingOAuthEmailCompletionTokenHash(rawToken)[:16],
		Payload: map[string]any{
			"template":                      pendingOAuthEmailCompletionTemplate,
			"recipient_email_hash":          emailHash(email),
			"recipient_email_ciphertext":    emailCiphertext,
			"verification_token_ciphertext": tokenCiphertext,
			"verification_token_version":    pendingOAuthEmailCompletionTokenV1,
			"verification_url_path":         pendingOAuthEmailCompletionURLPath,
			"expires_at":                    expiresAt.Format(time.RFC3339Nano),
		},
		Metadata: map[string]any{
			"token_delivery": "encrypted_outbox",
			"scope":          "pending_oauth_email_completion",
		},
	}); err != nil {
		return PendingOAuthEmailCompletionRequestResult{}, err
	}
	return PendingOAuthEmailCompletionRequestResult{Accepted: true, ExpiresAt: expiresAt}, nil
}

func (s *Service) ConfirmPendingOAuthEmailCompletion(ctx context.Context, sessionToken string, token string) (authcontract.PendingOAuthSession, error) {
	session, err := s.FindPendingOAuthSession(ctx, sessionToken)
	if err != nil {
		return authcontract.PendingOAuthSession{}, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return authcontract.PendingOAuthSession{}, ErrInvalidInput
	}
	if len(s.resetTokenKey) == 0 {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthUnavailable
	}
	payload, err := s.decryptPendingOAuthEmailCompletionToken(token)
	if err != nil {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthEmailInvalid
	}
	parts := strings.Split(payload, ":")
	if len(parts) != 4 {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthEmailInvalid
	}
	sessionID, err := strconv.Atoi(parts[0])
	email := strings.ToLower(strings.TrimSpace(parts[1]))
	expiresUnix, expiresErr := strconv.ParseInt(parts[2], 10, 64)
	rawToken := strings.TrimSpace(parts[3])
	now := s.clock.Now()
	if err != nil || expiresErr != nil || sessionID != session.ID || !time.Unix(expiresUnix, 0).After(now) || rawToken == "" || !validEmailAddress(email) {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthEmailInvalid
	}
	if session.Intent != authcontract.PendingOAuthIntentLogin || session.TargetUserID != nil || session.ResolvedEmail != "" {
		return authcontract.PendingOAuthSession{}, ErrInvalidInput
	}
	store, ok := s.sessions.(authcontract.PendingOAuthStore)
	if !ok {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthUnavailable
	}
	updated, err := store.CompletePendingOAuthEmail(ctx, s.pendingOAuthSessionTokenHash(sessionToken), email, now)
	if err != nil {
		return authcontract.PendingOAuthSession{}, ErrPendingOAuthInvalid
	}
	return updated, nil
}

func (s *Service) PreparePendingOAuthBindLogin(ctx context.Context, sessionToken, email, password string, adoptDisplayName bool) (PendingOAuthBindLoginResult, error) {
	session, err := s.FindPendingOAuthSession(ctx, sessionToken)
	if err != nil {
		return PendingOAuthBindLoginResult{}, err
	}
	if session.Intent != authcontract.PendingOAuthIntentLogin {
		return PendingOAuthBindLoginResult{}, ErrInvalidInput
	}
	storedUser, err := s.users.AuthenticatePassword(ctx, email, password)
	if err != nil {
		return PendingOAuthBindLoginResult{}, err
	}
	if session.TargetUserID != nil && *session.TargetUserID != storedUser.ID {
		return PendingOAuthBindLoginResult{}, ErrPendingOAuthTargetMismatch
	}
	if s.secondFactor != nil {
		enabled, err := s.secondFactor.IsEnabled(ctx, storedUser.ID)
		if err != nil {
			return PendingOAuthBindLoginResult{}, err
		}
		if enabled {
			challengeID, expiresAt, err := s.issuePendingOAuthBindLoginChallenge(storedUser.ID, sessionToken, adoptDisplayName)
			if err != nil {
				return PendingOAuthBindLoginResult{}, err
			}
			return PendingOAuthBindLoginResult{
				User:                       storedUser,
				RequiresSecondFactor:       true,
				SecondFactorChallengeID:    challengeID,
				SecondFactorChallengeUntil: &expiresAt,
			}, nil
		}
	}
	return PendingOAuthBindLoginResult{User: storedUser}, nil
}

func (s *Service) CompletePendingOAuthBindLoginSecondFactor(ctx context.Context, sessionToken, challengeID, code string) (CompletePendingOAuthBindLoginSecondFactorResult, error) {
	if s.secondFactor == nil {
		return CompletePendingOAuthBindLoginSecondFactorResult{}, ErrSecondFactorInvalid
	}
	userID, adoptDisplayName, err := s.verifyPendingOAuthBindLoginChallenge(sessionToken, challengeID)
	if err != nil {
		return CompletePendingOAuthBindLoginSecondFactorResult{}, ErrSecondFactorInvalid
	}
	if err := s.secondFactor.VerifyLogin(ctx, userID, code); err != nil {
		return CompletePendingOAuthBindLoginSecondFactorResult{}, ErrSecondFactorInvalid
	}
	session, err := s.FindPendingOAuthSession(ctx, sessionToken)
	if err != nil {
		return CompletePendingOAuthBindLoginSecondFactorResult{}, err
	}
	if session.Intent != authcontract.PendingOAuthIntentLogin {
		return CompletePendingOAuthBindLoginSecondFactorResult{}, ErrInvalidInput
	}
	if session.TargetUserID != nil && *session.TargetUserID != userID {
		return CompletePendingOAuthBindLoginSecondFactorResult{}, ErrPendingOAuthTargetMismatch
	}
	storedUser, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return CompletePendingOAuthBindLoginSecondFactorResult{}, ErrSessionUserUnavailable
	}
	if storedUser.Status != userscontract.StatusActive {
		return CompletePendingOAuthBindLoginSecondFactorResult{}, ErrSessionUserUnavailable
	}
	return CompletePendingOAuthBindLoginSecondFactorResult{User: storedUser, Session: session, AdoptDisplayName: adoptDisplayName}, nil
}

func (s *Service) IssuePendingOAuthActionToken(ctx context.Context, sessionToken string, action string) (PendingOAuthActionToken, error) {
	session, err := s.FindPendingOAuthSession(ctx, sessionToken)
	if err != nil {
		return PendingOAuthActionToken{}, err
	}
	action = normalizePendingOAuthAction(action)
	if action == "" || session.ID <= 0 || len(s.challengeKey) == 0 || len(s.resetTokenKey) == 0 {
		return PendingOAuthActionToken{}, ErrInvalidInput
	}
	nonce, err := randomRawToken(challengeBytes)
	if err != nil {
		return PendingOAuthActionToken{}, err
	}
	expiresAt := s.clock.Now().Add(challengeTTL)
	payload := strings.Join([]string{
		pendingOAuthActionVersion,
		action,
		strconv.Itoa(session.ID),
		strconv.FormatInt(expiresAt.Unix(), 10),
		nonce,
	}, ":")
	mac := hmac.New(sha256.New, s.challengeKey)
	mac.Write([]byte(payload))
	mac.Write([]byte(":"))
	mac.Write([]byte(s.pendingOAuthSessionTokenHash(sessionToken)))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return PendingOAuthActionToken{
		Token:     payload + ":" + signature,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *Service) VerifyPendingOAuthActionToken(ctx context.Context, sessionToken string, action string, token string) (authcontract.PendingOAuthSession, error) {
	session, err := s.FindPendingOAuthSession(ctx, sessionToken)
	if err != nil {
		return authcontract.PendingOAuthSession{}, err
	}
	action = normalizePendingOAuthAction(action)
	parts := strings.Split(strings.TrimSpace(token), ":")
	if action == "" || len(parts) != 6 || parts[0] != pendingOAuthActionVersion || parts[1] != action || len(s.challengeKey) == 0 || len(s.resetTokenKey) == 0 {
		return authcontract.PendingOAuthSession{}, ErrCSRFTokenInvalid
	}
	sessionID, err := strconv.Atoi(parts[2])
	if err != nil || sessionID != session.ID {
		return authcontract.PendingOAuthSession{}, ErrCSRFTokenInvalid
	}
	expiresUnix, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || !time.Unix(expiresUnix, 0).After(s.clock.Now()) {
		return authcontract.PendingOAuthSession{}, ErrCSRFTokenInvalid
	}
	payload := strings.Join(parts[:5], ":")
	mac := hmac.New(sha256.New, s.challengeKey)
	mac.Write([]byte(payload))
	mac.Write([]byte(":"))
	mac.Write([]byte(s.pendingOAuthSessionTokenHash(sessionToken)))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[5]), []byte(want)) {
		return authcontract.PendingOAuthSession{}, ErrCSRFTokenInvalid
	}
	return session, nil
}

func (s *Service) issuePendingOAuthBindLoginChallenge(userID int, sessionToken string, adoptDisplayName bool) (string, time.Time, error) {
	if userID <= 0 || strings.TrimSpace(sessionToken) == "" || len(s.challengeKey) == 0 || len(s.resetTokenKey) == 0 {
		return "", time.Time{}, ErrInvalidInput
	}
	nonce, err := randomRawToken(challengeBytes)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := s.clock.Now().Add(challengeTTL)
	payload := strings.Join([]string{
		pendingOAuthChallengeVersion,
		strconv.Itoa(userID),
		pendingOAuthBoolFlag(adoptDisplayName),
		strconv.FormatInt(expiresAt.Unix(), 10),
		nonce,
	}, ":")
	mac := hmac.New(sha256.New, s.challengeKey)
	mac.Write([]byte(payload))
	mac.Write([]byte(":"))
	mac.Write([]byte(s.pendingOAuthSessionTokenHash(sessionToken)))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + ":" + signature, expiresAt, nil
}

func (s *Service) verifyPendingOAuthBindLoginChallenge(sessionToken string, challengeID string) (int, bool, error) {
	parts := strings.Split(strings.TrimSpace(challengeID), ":")
	if len(parts) != 6 || parts[0] != pendingOAuthChallengeVersion || strings.TrimSpace(sessionToken) == "" || len(s.challengeKey) == 0 || len(s.resetTokenKey) == 0 {
		return 0, false, ErrSecondFactorInvalid
	}
	payload := strings.Join(parts[:5], ":")
	mac := hmac.New(sha256.New, s.challengeKey)
	mac.Write([]byte(payload))
	mac.Write([]byte(":"))
	mac.Write([]byte(s.pendingOAuthSessionTokenHash(sessionToken)))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[5]), []byte(want)) {
		return 0, false, ErrSecondFactorInvalid
	}
	expiresUnix, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || !time.Unix(expiresUnix, 0).After(s.clock.Now()) {
		return 0, false, ErrSecondFactorInvalid
	}
	userID, err := strconv.Atoi(parts[1])
	if err != nil || userID <= 0 {
		return 0, false, ErrSecondFactorInvalid
	}
	return userID, parts[2] == "1", nil
}

func pendingOAuthBoolFlag(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func (s *Service) HashOAuthProviderSubject(provider userscontract.AuthIdentityProvider, providerKey, subject string) (string, error) {
	provider = normalizeAuthIdentityProvider(provider)
	providerKey = strings.TrimSpace(providerKey)
	subject = strings.TrimSpace(subject)
	if provider == "" || providerKey == "" || subject == "" {
		return "", ErrInvalidInput
	}
	if len(s.resetTokenKey) == 0 {
		return "", ErrPendingOAuthUnavailable
	}
	mac := hmac.New(sha256.New, s.resetTokenKey)
	mac.Write([]byte("oauth_subject:"))
	mac.Write([]byte(provider))
	mac.Write([]byte(":"))
	mac.Write([]byte(providerKey))
	mac.Write([]byte(":"))
	mac.Write([]byte(subject))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (s *Service) pendingOAuthSessionTokenHash(token string) string {
	mac := hmac.New(sha256.New, s.resetTokenKey)
	mac.Write([]byte("pending_oauth:"))
	mac.Write([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) encryptPendingOAuthEmailCompletionToken(payload string) (string, error) {
	return s.encryptPendingOAuthEmailCompletionSecret(payload, "auth.pending_oauth_email_completion:"+pendingOAuthEmailCompletionTokenV1)
}

func (s *Service) encryptPendingOAuthEmailCompletionEmail(email string) (string, error) {
	return s.encryptPendingOAuthEmailCompletionSecret(email, "auth.pending_oauth_email_completion.email:"+pendingOAuthEmailCompletionTokenV1)
}

func (s *Service) encryptPendingOAuthEmailCompletionSecret(value string, aad string) (string, error) {
	block, err := aes.NewCipher(s.resetTokenKey)
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
		pendingOAuthEmailCompletionTokenV1,
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(ciphertext),
	}, ":"), nil
}

func (s *Service) decryptPendingOAuthEmailCompletionToken(token string) (string, error) {
	parts := strings.Split(strings.TrimSpace(token), ":")
	if len(parts) != 3 || parts[0] != pendingOAuthEmailCompletionTokenV1 {
		return "", ErrPendingOAuthEmailInvalid
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrPendingOAuthEmailInvalid
	}
	rawCiphertext, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", ErrPendingOAuthEmailInvalid
	}
	block, err := aes.NewCipher(s.resetTokenKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, nonce, rawCiphertext, []byte("auth.pending_oauth_email_completion:"+pendingOAuthEmailCompletionTokenV1))
	if err != nil {
		return "", ErrPendingOAuthEmailInvalid
	}
	return string(plaintext), nil
}

func (s *Service) pendingOAuthEmailCompletionTokenHash(token string) string {
	mac := hmac.New(sha256.New, s.resetTokenKey)
	mac.Write([]byte("pending_oauth_email:"))
	mac.Write([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(mac.Sum(nil))
}

func validEmailAddress(email string) bool {
	return email != "" && len(email) <= 320 && strings.Contains(email, "@") && !strings.ContainsAny(email, "\r\n\t ")
}

func normalizePendingOAuthIntent(intent authcontract.PendingOAuthIntent) authcontract.PendingOAuthIntent {
	switch authcontract.PendingOAuthIntent(strings.ToLower(strings.TrimSpace(string(intent)))) {
	case authcontract.PendingOAuthIntentLogin:
		return authcontract.PendingOAuthIntentLogin
	case authcontract.PendingOAuthIntentBindCurrentUser:
		return authcontract.PendingOAuthIntentBindCurrentUser
	default:
		return ""
	}
}

func normalizeAuthIdentityProvider(provider userscontract.AuthIdentityProvider) userscontract.AuthIdentityProvider {
	switch userscontract.AuthIdentityProvider(strings.ToLower(strings.TrimSpace(string(provider)))) {
	case userscontract.AuthIdentityProviderOIDC:
		return userscontract.AuthIdentityProviderOIDC
	case userscontract.AuthIdentityProviderGitHub:
		return userscontract.AuthIdentityProviderGitHub
	case userscontract.AuthIdentityProviderGoogle:
		return userscontract.AuthIdentityProviderGoogle
	case userscontract.AuthIdentityProviderLinuxDo:
		return userscontract.AuthIdentityProviderLinuxDo
	case userscontract.AuthIdentityProviderWeChat:
		return userscontract.AuthIdentityProviderWeChat
	case userscontract.AuthIdentityProviderDingTalk:
		return userscontract.AuthIdentityProviderDingTalk
	default:
		return ""
	}
}

func normalizePendingOAuthRedirect(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "/"
	}
	return value
}

func normalizePendingOAuthAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "create_account":
		return "create_account"
	default:
		return ""
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
