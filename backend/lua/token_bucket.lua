-- KEYS[1]   : rate limit key
-- ARGV[1]   : capacity (max tokens)
-- ARGV[2]   : refill rate (tokens per second)
-- ARGV[3]   : current timestamp (unix ms)
-- ARGV[4]   : cost (tokens to consume)
-- ARGV[5]   : TTL in seconds

local key         = KEYS[1]
local capacity    = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now         = tonumber(ARGV[3])
local cost        = tonumber(ARGV[4])
local ttl         = tonumber(ARGV[5])

local state       = redis.call("HMGET", key, "tokens", "last_update")
local tokens      = tonumber(state[1])
local last_update = tonumber(state[2])

if tokens == nil then
    tokens      = capacity
    last_update = now
end

local elapsed    = math.max(0, now - last_update) / 1000.0
local new_tokens = math.min(capacity, tokens + elapsed * refill_rate)

redis.call("HMSET", key, "tokens", new_tokens, "last_update", now)
redis.call("EXPIRE", key, ttl)

if new_tokens >= cost then
    new_tokens = new_tokens - cost
    redis.call("HMSET", key, "tokens", new_tokens, "last_update", now)
    return {1, math.floor(new_tokens), 0}
else
    local deficit      = cost - new_tokens
    local retry_after  = math.ceil(deficit / refill_rate)
    return {0, math.floor(new_tokens), retry_after}
end
