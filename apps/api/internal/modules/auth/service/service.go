package service

import (
	"context"
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
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
)

const (
	defaultSessionTTL = 12 * time.Hour
	sessionIDBytes    = 32
	csrfTokenBytes    = 32
	challengeTTL      = 5 * time.Minute
	challengeBytes    = 18
	challengeVersion  = "v1"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

type UserService interface {
	AuthenticatePassword(ctx context.Context, email, password string) (userscontract.StoredUser, error)
	FindByID(ctx context.Context, id int) (userscontract.StoredUser, error)
	TouchLastLogin(ctx context.Context, id int) error
}

type PasswordResetUserService interface {
	FindByEmail(ctx context.Context, email string) (userscontract.StoredUser, error)
	ResetPassword(ctx context.Context, id int, newPassword string) (userscontract.StoredUser, error)
}

type EmailVerificationUserService interface {
	FindByEmail(ctx context.Context, email string) (userscontract.StoredUser, error)
	VerifyEmail(ctx context.Context, id int, verifiedAt time.Time) (userscontract.StoredUser, error)
}

type SecondFactorVerifier interface {
	IsEnabled(ctx context.Context, userID int) (bool, error)
	VerifyLogin(ctx context.Context, userID int, code string) error
}

type EventEnqueuer interface {
	Enqueue(ctx context.Context, req eventscontract.EnqueueRequest) (eventscontract.OutboxEvent, error)
}

type Service struct {
	users         UserService
	sessions      authcontract.Store
	secondFactor  SecondFactorVerifier
	challengeKey  []byte
	resetTokenKey []byte
	events        EventEnqueuer
	clock         Clock
	ttl           time.Duration
}

func New(users UserService, sessions authcontract.Store, ttl time.Duration, clock Clock) (*Service, error) {
	if users == nil || sessions == nil {
		return nil, ErrInvalidInput
	}
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{users: users, sessions: sessions, clock: clock, ttl: ttl}, nil
}

func NewWithSecondFactor(users UserService, sessions authcontract.Store, ttl time.Duration, clock Clock, secondFactor SecondFactorVerifier, challengeSecret string) (*Service, error) {
	svc, err := New(users, sessions, ttl, clock)
	if err != nil {
		return nil, err
	}
	var challengeKey []byte
	if strings.TrimSpace(challengeSecret) != "" {
		challengeKey, err = platformcrypto.DeriveAESKey(challengeSecret)
		if err != nil {
			return nil, ErrInvalidInput
		}
	}
	if secondFactor != nil && len(challengeKey) == 0 {
		return nil, ErrInvalidInput
	}
	svc.secondFactor = secondFactor
	svc.challengeKey = challengeKey
	svc.resetTokenKey = challengeKey
	return svc, nil
}

func (s *Service) SetEventEnqueuer(events EventEnqueuer) {
	s.events = events
}

func (s *Service) Login(ctx context.Context, email, password string) (authcontract.LoginResult, error) {
	storedUser, err := s.users.AuthenticatePassword(ctx, email, password)
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	if s.secondFactor != nil {
		enabled, err := s.secondFactor.IsEnabled(ctx, storedUser.ID)
		if err != nil {
			return authcontract.LoginResult{}, err
		}
		if enabled {
			challengeID, expiresAt, err := s.issueSecondFactorChallenge(storedUser.ID)
			if err != nil {
				return authcontract.LoginResult{}, err
			}
			return authcontract.LoginResult{
				User:                       storedUser.User,
				RequiresSecondFactor:       true,
				SecondFactorChallengeID:    challengeID,
				SecondFactorChallengeUntil: &expiresAt,
			}, nil
		}
	}
	return s.CreateSessionForUser(ctx, storedUser)
}

func (s *Service) CompleteSecondFactorLogin(ctx context.Context, challengeID string, code string) (authcontract.LoginResult, error) {
	if s.secondFactor == nil {
		return authcontract.LoginResult{}, ErrSecondFactorInvalid
	}
	userID, err := s.verifySecondFactorChallenge(challengeID)
	if err != nil {
		return authcontract.LoginResult{}, ErrSecondFactorInvalid
	}
	if err := s.secondFactor.VerifyLogin(ctx, userID, code); err != nil {
		return authcontract.LoginResult{}, ErrSecondFactorInvalid
	}
	storedUser, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return authcontract.LoginResult{}, ErrSessionUserUnavailable
	}
	if storedUser.Status != userscontract.StatusActive {
		return authcontract.LoginResult{}, ErrSessionUserUnavailable
	}
	return s.CreateSessionForUser(ctx, storedUser)
}

// CreateSessionForUser creates a console session for a previously authenticated or newly registered active user.
func (s *Service) CreateSessionForUser(ctx context.Context, storedUser userscontract.StoredUser) (authcontract.LoginResult, error) {
	if storedUser.ID <= 0 || storedUser.Status != userscontract.StatusActive {
		return authcontract.LoginResult{}, ErrSessionUserUnavailable
	}
	now := s.clock.Now()
	sessionID, err := randomToken("sess", sessionIDBytes)
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	csrfToken, err := randomToken("csrf", csrfTokenBytes)
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	session, err := s.sessions.Create(ctx, authcontract.CreateSession{
		ID:        sessionID,
		UserID:    storedUser.ID,
		CSRFToken: csrfToken,
		ExpiresAt: now.Add(s.ttl),
		CreatedAt: now,
	})
	if err != nil {
		return authcontract.LoginResult{}, err
	}
	if err := s.users.TouchLastLogin(ctx, storedUser.ID); err != nil {
		_ = s.sessions.Delete(ctx, session.ID)
		return authcontract.LoginResult{}, err
	}
	storedUser.LastLoginAt = &now
	return authcontract.LoginResult{
		User:    storedUser.User,
		Session: session,
	}, nil
}

func (s *Service) issueSecondFactorChallenge(userID int) (string, time.Time, error) {
	if userID <= 0 || len(s.challengeKey) == 0 {
		return "", time.Time{}, ErrInvalidInput
	}
	nonce, err := randomRawToken(challengeBytes)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := s.clock.Now().Add(challengeTTL)
	payload := strings.Join([]string{
		challengeVersion,
		strconv.Itoa(userID),
		strconv.FormatInt(expiresAt.Unix(), 10),
		nonce,
	}, ":")
	mac := hmac.New(sha256.New, s.challengeKey)
	mac.Write([]byte(payload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + ":" + signature, expiresAt, nil
}

func (s *Service) verifySecondFactorChallenge(challengeID string) (int, error) {
	parts := strings.Split(strings.TrimSpace(challengeID), ":")
	if len(parts) != 5 || parts[0] != challengeVersion {
		return 0, ErrSecondFactorInvalid
	}
	payload := strings.Join(parts[:4], ":")
	mac := hmac.New(sha256.New, s.challengeKey)
	mac.Write([]byte(payload))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[4]), []byte(want)) {
		return 0, ErrSecondFactorInvalid
	}
	expiresUnix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, ErrSecondFactorInvalid
	}
	if !time.Unix(expiresUnix, 0).After(s.clock.Now()) {
		return 0, ErrSecondFactorInvalid
	}
	userID, err := strconv.Atoi(parts[1])
	if err != nil || userID <= 0 {
		return 0, ErrSecondFactorInvalid
	}
	return userID, nil
}

func (s *Service) AuthenticateSession(ctx context.Context, sessionID string) (authcontract.LoginResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return authcontract.LoginResult{}, ErrSessionNotFound
	}
	session, err := s.sessions.FindByID(ctx, sessionID)
	if err != nil {
		return authcontract.LoginResult{}, ErrSessionNotFound
	}
	now := s.clock.Now()
	if !session.ExpiresAt.After(now) {
		_ = s.sessions.Delete(ctx, session.ID)
		return authcontract.LoginResult{}, ErrSessionExpired
	}
	storedUser, err := s.users.FindByID(ctx, session.UserID)
	if err != nil {
		return authcontract.LoginResult{}, ErrSessionUserUnavailable
	}
	if storedUser.Status != userscontract.StatusActive {
		return authcontract.LoginResult{}, ErrSessionUserUnavailable
	}
	if err := s.sessions.Touch(ctx, session.ID, now); err != nil {
		return authcontract.LoginResult{}, err
	}
	session.LastSeenAt = &now
	return authcontract.LoginResult{
		User:    storedUser.User,
		Session: session,
	}, nil
}

func (s *Service) CurrentUser(ctx context.Context, sessionID string) (userscontract.User, error) {
	result, err := s.AuthenticateSession(ctx, sessionID)
	if err != nil {
		return userscontract.User{}, err
	}
	return result.User, nil
}

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ErrSessionNotFound
	}
	return s.sessions.Delete(ctx, sessionID)
}

// LogoutUser revokes all active console sessions for a user.
func (s *Service) LogoutUser(ctx context.Context, userID int) error {
	if userID <= 0 {
		return ErrInvalidInput
	}
	return s.sessions.DeleteByUserID(ctx, userID)
}

func ValidateCSRF(session authcontract.Session, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrCSRFTokenInvalid
	}
	expected := strings.TrimSpace(session.CSRFToken)
	if strings.HasPrefix(expected, "sha256:") {
		sum := sha256.Sum256([]byte(token))
		token = "sha256:" + hex.EncodeToString(sum[:])
	}
	if !hmac.Equal([]byte(expected), []byte(token)) {
		return ErrCSRFTokenInvalid
	}
	return nil
}

// GenerateCSRFToken creates a new cryptographically random CSRF token.
func GenerateCSRFToken() (string, error) {
	return randomToken("csrf", csrfTokenBytes)
}

func randomToken(prefix string, size int) (string, error) {
	token, err := randomRawToken(size)
	if err != nil {
		return "", err
	}
	return prefix + "_" + token, nil
}

func randomRawToken(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
