-- KEYS[1]   : rate limit key
-- ARGV[1]   : window size in milliseconds
-- ARGV[2]   : current timestamp (unix ms)
-- ARGV[3]   : limit (max requests in window)
-- ARGV[4]   : unique member (request nonce for ZADD)
-- ARGV[5]   : TTL in seconds

local key       = KEYS[1]
local window    = tonumber(ARGV[1])
local now       = tonumber(ARGV[2])
local limit     = tonumber(ARGV[3])
local member    = ARGV[4]
local ttl       = tonumber(ARGV[5])

local min_score = now - window
redis.call("ZREMRANGEBYSCORE", key, 0, min_score)

local current = redis.call("ZCARD", key)

if current < limit then
    redis.call("ZADD", key, now, member)
    redis.call("EXPIRE", key, ttl)
    local remaining = limit - current - 1
    return {1, remaining, 0}
else
    local oldest    = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
    local oldest_ts = tonumber(oldest[2])
    local retry_after = math.ceil((oldest_ts + window - now) / 1000)
    if retry_after < 1 then retry_after = 1 end
    return {0, 0, retry_after}
end
