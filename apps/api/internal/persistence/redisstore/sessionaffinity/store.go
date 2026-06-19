// Package sessionaffinity is the Redis-backed session→account affinity store.
//
// Bindings are shared across all gateway nodes so a multi-turn conversation
// stays pinned to one upstream account regardless of which node serves each
// turn. Keys are namespaced sticky_session:{scope}:{sessionKey} and carry a TTL
// so idle sessions release their account automatically.
package sessionaffinity

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/srapi/srapi/apps/api/internal/modules/sessionaffinity/contract"
)

// ErrInvalidStore is returned by New when given a nil client.
var ErrInvalidStore = errors.New("invalid session affinity redis store")

const keyPrefix = "sticky_session:"

// Store is a Redis-backed session affinity store.
type Store struct {
	client *redis.Client
}

var _ contract.Store = (*Store)(nil)

// New returns a Redis-backed session affinity store.
func New(client *redis.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func redisKey(scope, key string) string {
	return keyPrefix + scope + ":" + key
}

// Lookup resolves the longest-prefix binding for sessionKey, refreshing its TTL
// on a hit.
func (s *Store) Lookup(ctx context.Context, scope, sessionKey string, ttl time.Duration) (contract.Binding, error) {
	candidates := contract.CandidateKeys(sessionKey)
	if len(candidates) == 0 {
		return contract.Binding{}, nil
	}
	redisKeys := make([]string, len(candidates))
	for i, candidate := range candidates {
		redisKeys[i] = redisKey(scope, candidate)
	}
	values, err := s.client.MGet(ctx, redisKeys...).Result()
	if err != nil {
		return contract.Binding{}, err
	}
	for i, value := range values {
		raw, ok := value.(string)
		if !ok {
			continue
		}
		accountID, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || accountID <= 0 {
			continue
		}
		if ttl > 0 {
			// Best-effort TTL refresh on the matched key.
			_ = s.client.PExpire(ctx, redisKeys[i], ttl).Err()
		}
		return contract.Binding{AccountID: accountID, MatchedKey: candidates[i]}, nil
	}
	return contract.Binding{}, nil
}

// Bind stores sessionKey→accountID with the given TTL.
func (s *Store) Bind(ctx context.Context, scope, sessionKey string, accountID int, ttl time.Duration) error {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || accountID <= 0 {
		return contract.ErrInvalidInput
	}
	return s.client.Set(ctx, redisKey(scope, sessionKey), strconv.Itoa(accountID), ttl).Err()
}

// Release removes the binding for sessionKey.
func (s *Store) Release(ctx context.Context, scope, sessionKey string) error {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return nil
	}
	return s.client.Del(ctx, redisKey(scope, sessionKey)).Err()
}

const accountSessionsKeyPrefix = "sticky_account_sessions:"

func accountSessionsKey(accountID int) string {
	return accountSessionsKeyPrefix + strconv.Itoa(accountID)
}

var addAccountSessionScript = redis.NewScript(`
local key = KEYS[1]
local session_id = ARGV[1]
local now_ms = ARGV[2]
local expires_at_ms = ARGV[3]
local ttl_ms = tonumber(ARGV[4])

redis.call("ZREMRANGEBYSCORE", key, "-inf", now_ms)
redis.call("ZADD", key, expires_at_ms, session_id)

local current_ttl = redis.call("PTTL", key)
if current_ttl < ttl_ms then
  redis.call("PEXPIRE", key, ttl_ms)
end
return 1
`)

// AddAccountSession records sessionID as active on accountID (ZSET scored by
// expiry; re-adding refreshes the score so one conversation never double-counts).
func (s *Store) AddAccountSession(ctx context.Context, accountID int, sessionID string, ttl time.Duration) error {
	sessionID = strings.TrimSpace(sessionID)
	if accountID <= 0 || sessionID == "" {
		return contract.ErrInvalidInput
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	key := accountSessionsKey(accountID)
	nowMs := time.Now().UnixMilli()
	ttlMs := ttl.Milliseconds()
	if ttlMs <= 0 {
		ttlMs = 1
	}
	expiresAtMs := nowMs + ttlMs
	_, err := addAccountSessionScript.Run(ctx, s.client, []string{key}, sessionID, nowMs, expiresAtMs, ttlMs).Result()
	return err
}

// CountAccountSessionsExcluding counts live sessions on accountID other than
// sessionID.
func (s *Store) CountAccountSessionsExcluding(ctx context.Context, accountID int, sessionID string) (int, error) {
	key := accountSessionsKey(accountID)
	if err := s.client.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(time.Now().UnixMilli(), 10)).Err(); err != nil {
		return 0, err
	}
	total, err := s.client.ZCard(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	count := int(total)
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		if _, err := s.client.ZScore(ctx, key, sessionID).Result(); err == nil {
			count-- // exclude the current conversation's own slot
		} else if !errors.Is(err, redis.Nil) {
			return 0, err
		}
	}
	if count < 0 {
		count = 0
	}
	return count, nil
}
