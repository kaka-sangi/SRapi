package service

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisHitCounter implements HitCounter using Redis INCR with TTL. Each key
// is a simple counter that auto-expires after the sliding window. Multiple
// nodes incrementing the same key converge to the cluster-wide total.
type RedisHitCounter struct {
	client redis.Cmdable
}

// NewRedisHitCounter wraps a go-redis client (or the .Raw() of the platform
// redis.Client) into a HitCounter.
func NewRedisHitCounter(client redis.Cmdable) *RedisHitCounter {
	if client == nil {
		return nil
	}
	return &RedisHitCounter{client: client}
}

func (r *RedisHitCounter) IncrHit(key string, window time.Duration) (int64, error) {
	ctx := context.Background()
	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}
