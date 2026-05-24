package ratelimit

import "github.com/redis/go-redis/v9"

var multiLimitScript = redis.NewScript(`
local count = #KEYS
local max_limit = 0
local max_used = 0

for i = 1, count do
	local name = ARGV[((i - 1) * 4) + 1]
	local limit = tonumber(ARGV[((i - 1) * 4) + 2])
	local cost = tonumber(ARGV[((i - 1) * 4) + 3])
	local window_ms = tonumber(ARGV[((i - 1) * 4) + 4])
	local used = tonumber(redis.call("GET", KEYS[i]) or "0")
	if used + cost > limit then
		local ttl = redis.call("PTTL", KEYS[i])
		if ttl < 0 then
			ttl = window_ms
		end
		return {"limited", name, tostring(limit), tostring(used), tostring(cost), tostring(ttl)}
	end
end

for i = 1, count do
	local cost = tonumber(ARGV[((i - 1) * 4) + 3])
	local window_ms = tonumber(ARGV[((i - 1) * 4) + 4])
	local used = redis.call("INCRBY", KEYS[i], cost)
	if used == cost then
		redis.call("PEXPIRE", KEYS[i], window_ms)
	elseif redis.call("PTTL", KEYS[i]) < 0 then
		redis.call("PEXPIRE", KEYS[i], window_ms)
	end
	local limit = tonumber(ARGV[((i - 1) * 4) + 2])
	if limit > max_limit then
		max_limit = limit
	end
	if used > max_used then
		max_used = used
	end
end

return {"ok", tostring(max_limit), tostring(max_used)}
`)

var acquireConcurrencyScript = redis.NewScript(`
local name = ARGV[1]
local limit = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local ttl_ms = tonumber(ARGV[4])
local token = ARGV[5]
local expire_at = now_ms + ttl_ms

redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", now_ms)
local used = tonumber(redis.call("ZCARD", KEYS[1]))
if used >= limit then
	local oldest = redis.call("ZRANGE", KEYS[1], 0, 0, "WITHSCORES")
	local retry_ms = ttl_ms
	if oldest[2] ~= nil then
		retry_ms = tonumber(oldest[2]) - now_ms
		if retry_ms < 1 then
			retry_ms = 1
		end
	end
	return {"limited", name, tostring(limit), tostring(used), "1", tostring(retry_ms)}
end

redis.call("ZADD", KEYS[1], expire_at, token)
redis.call("PEXPIRE", KEYS[1], ttl_ms)
return {"ok", name, tostring(limit), tostring(used + 1)}
`)

var releaseConcurrencyScript = redis.NewScript(`
redis.call("ZREM", KEYS[1], ARGV[1])
if redis.call("ZCARD", KEYS[1]) == 0 then
	redis.call("DEL", KEYS[1])
end
return {"ok"}
`)
