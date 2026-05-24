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
