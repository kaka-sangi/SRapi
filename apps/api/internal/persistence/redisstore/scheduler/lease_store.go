package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

var (
	ErrInvalidStore    = errors.New("invalid scheduler redis lease store")
	ErrInvalidLease    = errors.New("invalid lease")
	ErrConcurrencyFull = errors.New("concurrency full")
	ErrLeaseNotFound   = errors.New("lease not found")
)

const (
	defaultLeaseRetention = 5 * time.Minute
	leaseKeyPrefix        = "scheduler:lease:"
	requestKeyPrefix      = "scheduler:lease_request:"
	accountKeyPrefix      = "scheduler:account:"
)

type Store struct {
	client    *redis.Client
	retention time.Duration
}

func New(client *redis.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client, retention: defaultLeaseRetention}, nil
}

func (s *Store) AcquireLease(ctx context.Context, input contract.Lease, maxConcurrency *int) (contract.Lease, error) {
	if input.ID == "" || input.RequestID == "" || input.AccountID <= 0 {
		return contract.Lease{}, ErrInvalidLease
	}
	if input.AttemptNo <= 0 {
		input.AttemptNo = 1
	}
	now := time.Now().UTC()
	lease := input
	lease.Status = contract.LeaseStatusPending
	if lease.CreatedAt.IsZero() {
		lease.CreatedAt = now
	}
	if lease.UpdatedAt.IsZero() {
		lease.UpdatedAt = lease.CreatedAt
	}
	if lease.ExpiresAt.IsZero() {
		lease.ExpiresAt = now.Add(30 * time.Second)
	}
	max := -1
	if maxConcurrency != nil {
		max = *maxConcurrency
	}
	retentionMS := ttlMillis(time.Until(lease.ExpiresAt) + s.retention)
	result, err := acquireLeaseScript.Run(ctx, s.client, []string{
		s.leaseKey(lease.ID),
		s.accountLeasesKey(lease.AccountID),
		s.accountConcurrencyKey(lease.AccountID),
		s.requestKey(lease.RequestID, lease.AttemptNo),
	},
		now.UnixNano(),
		lease.ID,
		max,
		lease.RequestID,
		lease.AccountID,
		lease.ExpiresAt.UnixNano(),
		lease.CreatedAt.UnixNano(),
		lease.UpdatedAt.UnixNano(),
		leaseKeyPrefix,
		retentionMS,
		lease.AttemptNo,
	).Slice()
	if err != nil {
		return contract.Lease{}, err
	}
	code, value := scriptResult(result)
	switch code {
	case "ok", "exists":
		// Mark this account least-recently-used "now" so the scheduler rotates
		// load across equally-scored accounts. Best-effort; never fails acquire.
		_ = s.client.Set(ctx, s.accountLastUsedKey(lease.AccountID), now.UnixMilli(), lastUsedRetention).Err()
		return s.findLeaseByID(ctx, value)
	case "full":
		return contract.Lease{}, ErrConcurrencyFull
	default:
		return contract.Lease{}, fmt.Errorf("unexpected scheduler lease result: %s", code)
	}
}

func (s *Store) UpdateLeaseStatus(ctx context.Context, requestID string, attemptNo int, status contract.LeaseStatus) (contract.Lease, error) {
	if requestID == "" {
		return contract.Lease{}, ErrLeaseNotFound
	}
	if attemptNo <= 0 {
		attemptNo = 1
	}
	now := time.Now().UTC()
	result, err := updateLeaseScript.Run(ctx, s.client, []string{
		s.requestKey(requestID, attemptNo),
		"",
		"",
	},
		now.UnixNano(),
		string(status),
		leaseKeyPrefix,
		accountKeyPrefix,
		ttlMillis(s.retention),
	).Slice()
	if err != nil {
		return contract.Lease{}, err
	}
	code, leaseID := scriptResult(result)
	if code != "ok" || leaseID == "" {
		return contract.Lease{}, ErrLeaseNotFound
	}
	return s.findLeaseByID(ctx, leaseID)
}

func (s *Store) ListLeases(ctx context.Context) ([]contract.Lease, error) {
	var (
		cursor uint64
		out    []contract.Lease
	)
	for {
		keys, next, err := s.client.Scan(ctx, cursor, leaseKeyPrefix+"*", 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			row, err := s.client.HGetAll(ctx, key).Result()
			if err != nil {
				return nil, err
			}
			if len(row) == 0 {
				continue
			}
			lease := leaseFromHash(row)
			if lease.ID == "" {
				continue
			}
			if lease.Status == contract.LeaseStatusPending && !lease.ExpiresAt.IsZero() && !lease.ExpiresAt.After(time.Now().UTC()) {
				lease.Status = contract.LeaseStatusExpired
				lease.UpdatedAt = time.Now().UTC()
				if err := s.markExpired(ctx, lease); err != nil {
					return nil, err
				}
			}
			out = append(out, lease)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) CountActiveLeases(ctx context.Context) (int, error) {
	var (
		cursor uint64
		total  int
	)
	now := time.Now().UTC()
	for {
		keys, next, err := s.client.Scan(ctx, cursor, leaseKeyPrefix+"*", 100).Result()
		if err != nil {
			return 0, err
		}
		for _, key := range keys {
			row, err := s.client.HGetAll(ctx, key).Result()
			if err != nil {
				return 0, err
			}
			if len(row) == 0 {
				continue
			}
			lease := leaseFromHash(row)
			if lease.ID == "" {
				continue
			}
			if lease.Status == contract.LeaseStatusPending && !lease.ExpiresAt.IsZero() && !lease.ExpiresAt.After(now) {
				lease.Status = contract.LeaseStatusExpired
				lease.UpdatedAt = now
				if err := s.markExpired(ctx, lease); err != nil {
					return 0, err
				}
				continue
			}
			if lease.Status == contract.LeaseStatusPending {
				total++
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return total, nil
}

func (s *Store) findLeaseByID(ctx context.Context, leaseID string) (contract.Lease, error) {
	row, err := s.client.HGetAll(ctx, s.leaseKey(leaseID)).Result()
	if err != nil {
		return contract.Lease{}, err
	}
	if len(row) == 0 {
		return contract.Lease{}, ErrLeaseNotFound
	}
	lease := leaseFromHash(row)
	if lease.ID == "" {
		return contract.Lease{}, ErrLeaseNotFound
	}
	return lease, nil
}

func (s *Store) markExpired(ctx context.Context, lease contract.Lease) error {
	pipe := s.client.TxPipeline()
	pipe.HSet(ctx, s.leaseKey(lease.ID),
		"status", string(contract.LeaseStatusExpired),
		"updated_at_unix_nano", lease.UpdatedAt.UnixNano(),
	)
	pipe.ZRem(ctx, s.accountLeasesKey(lease.AccountID), lease.ID)
	pipe.Set(ctx, s.accountConcurrencyKey(lease.AccountID), 0, s.retention)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) leaseKey(id string) string {
	return leaseKeyPrefix + id
}

func (s *Store) requestKey(requestID string, attemptNo int) string {
	if attemptNo <= 0 {
		attemptNo = 1
	}
	return requestKeyPrefix + requestID + ":" + strconv.Itoa(attemptNo)
}

func (s *Store) accountLeasesKey(accountID int) string {
	return accountKeyPrefix + strconv.Itoa(accountID) + ":leases"
}

func (s *Store) accountConcurrencyKey(accountID int) string {
	return accountKeyPrefix + strconv.Itoa(accountID) + ":concurrency"
}

func (s *Store) accountLastUsedKey(accountID int) string {
	return accountKeyPrefix + strconv.Itoa(accountID) + ":last_used"
}

// lastUsedRetention keeps the least-recently-used marker alive well beyond a
// single lease so LRU ordering survives idle gaps between requests.
const lastUsedRetention = time.Hour

// AccountLastUsed returns when an account was last selected (epoch ms), or 0 if
// no marker exists. Implements contract.AccountLastUsedReporter.
func (s *Store) AccountLastUsed(ctx context.Context, accountID int) (int64, error) {
	if s == nil || s.client == nil || accountID <= 0 {
		return 0, nil
	}
	raw, err := s.client.Get(ctx, s.accountLastUsedKey(accountID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, nil
	}
	return value, nil
}

// CountAccountConcurrency returns the live in-flight lease count for an account,
// maintained by AcquireLease as the ZCARD of the account's active lease set. A
// missing key means no active leases (0). Implements
// contract.AccountConcurrencyCounter.
func (s *Store) CountAccountConcurrency(ctx context.Context, accountID int) (int, error) {
	if s == nil || s.client == nil || accountID <= 0 {
		return 0, nil
	}
	raw, err := s.client.Get(ctx, s.accountConcurrencyKey(accountID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, nil
	}
	return value, nil
}

func ttlMillis(value time.Duration) int64 {
	if value <= 0 {
		return int64(time.Millisecond)
	}
	return int64(value / time.Millisecond)
}

func scriptResult(values []any) (string, string) {
	if len(values) == 0 {
		return "", ""
	}
	code := scriptString(values[0])
	value := ""
	if len(values) > 1 {
		value = scriptString(values[1])
	}
	return code, value
}

func scriptString(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

func leaseFromHash(row map[string]string) contract.Lease {
	return contract.Lease{
		ID:        row["id"],
		RequestID: row["request_id"],
		AttemptNo: parseInt(row["attempt_no"]),
		AccountID: parseInt(row["account_id"]),
		Status:    contract.LeaseStatus(row["status"]),
		ExpiresAt: parseUnixNano(row["expires_at_unix_nano"]),
		CreatedAt: parseUnixNano(row["created_at_unix_nano"]),
		UpdatedAt: parseUnixNano(row["updated_at_unix_nano"]),
	}
}

func parseInt(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func parseUnixNano(value string) time.Time {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed == 0 {
		return time.Time{}
	}
	return time.Unix(0, parsed).UTC()
}

var acquireLeaseScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local lease_id = ARGV[2]
local max_concurrency = tonumber(ARGV[3])
local request_id = ARGV[4]
local account_id = ARGV[5]
local expires_at = tonumber(ARGV[6])
local created_at = ARGV[7]
local updated_at = ARGV[8]
local lease_prefix = ARGV[9]
local retention_ms = tonumber(ARGV[10])
local attempt_no = ARGV[11]

local expired = redis.call("ZRANGEBYSCORE", KEYS[2], "-inf", now)
for _, expired_lease_id in ipairs(expired) do
	local expired_key = lease_prefix .. expired_lease_id
	if redis.call("HGET", expired_key, "status") == "pending" then
		redis.call("HSET", expired_key, "status", "expired", "updated_at_unix_nano", now)
	end
end
if #expired > 0 then
	redis.call("ZREM", KEYS[2], unpack(expired))
end

local existing = redis.call("GET", KEYS[4])
if existing then
	return {"exists", existing}
end
if redis.call("EXISTS", KEYS[1]) == 1 then
	return {"exists", lease_id}
end

local active = redis.call("ZCARD", KEYS[2])
if max_concurrency >= 0 and active >= max_concurrency then
	redis.call("SET", KEYS[3], active, "PX", retention_ms)
	return {"full", tostring(active)}
end

redis.call("HSET", KEYS[1],
	"id", lease_id,
	"request_id", request_id,
	"attempt_no", attempt_no,
	"account_id", account_id,
	"status", "pending",
	"expires_at_unix_nano", expires_at,
	"created_at_unix_nano", created_at,
	"updated_at_unix_nano", updated_at
)
redis.call("PEXPIRE", KEYS[1], retention_ms)
redis.call("SET", KEYS[4], lease_id, "PX", retention_ms)
redis.call("ZADD", KEYS[2], expires_at, lease_id)
redis.call("PEXPIRE", KEYS[2], retention_ms)
redis.call("SET", KEYS[3], active + 1, "PX", retention_ms)
return {"ok", lease_id}
`)

var updateLeaseScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local next_status = ARGV[2]
local lease_prefix = ARGV[3]
local account_prefix = ARGV[4]
local retention_ms = tonumber(ARGV[5])

local lease_id = redis.call("GET", KEYS[1])
if not lease_id then
	return {"missing", ""}
end

local lease_key = lease_prefix .. lease_id
if redis.call("EXISTS", lease_key) == 0 then
	return {"missing", lease_id}
end

local current_status = redis.call("HGET", lease_key, "status")
local account_id = redis.call("HGET", lease_key, "account_id")
local expires_at = tonumber(redis.call("HGET", lease_key, "expires_at_unix_nano") or "0")
local account_leases_key = account_prefix .. account_id .. ":leases"
local concurrency_key = account_prefix .. account_id .. ":concurrency"

if current_status == "pending" and expires_at > 0 and expires_at <= now then
	current_status = "expired"
	redis.call("HSET", lease_key, "status", "expired", "updated_at_unix_nano", now)
	redis.call("ZREM", account_leases_key, lease_id)
end

if current_status == "pending" then
	redis.call("HSET", lease_key, "status", next_status, "updated_at_unix_nano", now)
	redis.call("ZREM", account_leases_key, lease_id)
end

redis.call("SET", concurrency_key, redis.call("ZCARD", account_leases_key), "PX", retention_ms)
return {"ok", lease_id}
`)
