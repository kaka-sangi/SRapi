package realtime

import "github.com/redis/go-redis/v9"

var acquireSlotScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local slot_id = ARGV[2]
local kind = ARGV[3]
local request_id = ARGV[4]
local user_id = ARGV[5]
local api_key_id = ARGV[6]
local source_endpoint = ARGV[7]
local affinity_source = ARGV[8]
local affinity_hash = ARGV[9]
local sticky_account_id = ARGV[10]
local sticky_strength = ARGV[11]
local expires_at = tonumber(ARGV[12])
local max_slots = tonumber(ARGV[13])
local max_slots_per_key = tonumber(ARGV[14])
local slot_prefix = ARGV[15]
local retention_ms = tonumber(ARGV[16])

local expired = redis.call("ZRANGEBYSCORE", KEYS[2], "-inf", now)
for _, expired_slot_id in ipairs(expired) do
	local expired_key = slot_prefix .. expired_slot_id
	if redis.call("EXISTS", expired_key) == 1 and redis.call("HGET", expired_key, "released_at_unix_nano") == false then
		redis.call("HSET", expired_key, "released_at_unix_nano", now)
		redis.call("HINCRBY", KEYS[3], "released", 1)
		redis.call("PEXPIRE", expired_key, retention_ms)
	end
end
if #expired > 0 then
	redis.call("ZREM", KEYS[2], unpack(expired))
end

if redis.call("EXISTS", KEYS[1]) == 1 then
	return {"exists", slot_id}
end

local active = redis.call("ZCARD", KEYS[2])
if max_slots > 0 and active >= max_slots then
	redis.call("HINCRBY", KEYS[3], "rejected", 1)
	return {"full", tostring(active)}
end

if max_slots_per_key > 0 then
	local active_for_key = 0
	local active_ids = redis.call("ZRANGE", KEYS[2], 0, -1)
	for _, active_id in ipairs(active_ids) do
		local active_key = slot_prefix .. active_id
		if redis.call("HGET", active_key, "api_key_id") == api_key_id then
			active_for_key = active_for_key + 1
		end
	end
	if active_for_key >= max_slots_per_key then
		redis.call("HINCRBY", KEYS[3], "rejected", 1)
		return {"full", tostring(active_for_key)}
	end
end

redis.call("HSET", KEYS[1],
	"id", slot_id,
	"kind", kind,
	"request_id", request_id,
	"user_id", user_id,
	"api_key_id", api_key_id,
	"source_endpoint", source_endpoint,
	"session_affinity_source", affinity_source,
	"session_affinity_key_hash", affinity_hash,
	"sticky_account_id", sticky_account_id,
	"sticky_strength", sticky_strength,
	"acquired_at_unix_nano", now
)
redis.call("PEXPIRE", KEYS[1], retention_ms)
redis.call("ZADD", KEYS[2], expires_at, slot_id)
redis.call("PEXPIRE", KEYS[2], retention_ms)
redis.call("HINCRBY", KEYS[3], "acquired", 1)
redis.call("PEXPIRE", KEYS[3], retention_ms)
return {"ok", slot_id}
`)

var releaseSlotScript = redis.NewScript(`
local released_at = tonumber(ARGV[1])
local slot_id = ARGV[2]
local retention_ms = tonumber(ARGV[3])

if redis.call("EXISTS", KEYS[1]) == 0 then
	return {"missing", slot_id}
end
local already_released = redis.call("HGET", KEYS[1], "released_at_unix_nano")
if already_released == false or already_released == "" then
	redis.call("HSET", KEYS[1], "released_at_unix_nano", released_at)
	redis.call("HINCRBY", KEYS[3], "released", 1)
end
redis.call("ZREM", KEYS[2], slot_id)
redis.call("PEXPIRE", KEYS[1], retention_ms)
redis.call("PEXPIRE", KEYS[3], retention_ms)
return {"ok", slot_id}
`)

var expireSlotsScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local slot_prefix = ARGV[2]
local retention_ms = tonumber(ARGV[3])

local expired = redis.call("ZRANGEBYSCORE", KEYS[1], "-inf", now)
for _, expired_slot_id in ipairs(expired) do
	local expired_key = slot_prefix .. expired_slot_id
	if redis.call("EXISTS", expired_key) == 1 and redis.call("HGET", expired_key, "released_at_unix_nano") == false then
		redis.call("HSET", expired_key, "released_at_unix_nano", now)
		redis.call("HINCRBY", KEYS[2], "released", 1)
		redis.call("PEXPIRE", expired_key, retention_ms)
	end
end
if #expired > 0 then
	redis.call("ZREM", KEYS[1], unpack(expired))
end
redis.call("PEXPIRE", KEYS[2], retention_ms)
return {"ok", tostring(#expired)}
`)
