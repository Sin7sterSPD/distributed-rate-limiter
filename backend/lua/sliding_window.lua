-- Sliding Window Counter (weighted previous + current fixed-window counters)
--
-- KEYS[1]   : rate limit key
-- ARGV[1]   : window size in milliseconds
-- ARGV[2]   : current timestamp (unix ms)
-- ARGV[3]   : limit (max requests per window)
-- ARGV[4]   : cost (requests to consume)
-- ARGV[5]   : TTL in seconds
--
-- Returns {allowed(0|1), remaining, retry_after_seconds}

local key    = KEYS[1]
local window = tonumber(ARGV[1])
local now    = tonumber(ARGV[2])
local limit  = tonumber(ARGV[3])
local cost   = tonumber(ARGV[4])
local ttl    = tonumber(ARGV[5])

-- Current fixed-window start, aligned to the window boundary.
local curr_start = now - (now % window)

local state = redis.call("HMGET", key, "prev", "curr", "start")
local prev  = tonumber(state[1]) or 0
local curr  = tonumber(state[2]) or 0
local start = tonumber(state[3])

if start == nil then
    prev, curr, start = 0, 0, curr_start
elseif curr_start > start then
    -- Advance the fixed windows.
    local elapsed_windows = math.floor((curr_start - start) / window)
    if elapsed_windows == 1 then
        prev = curr
    else
        prev = 0 -- more than a full window passed: both counters are stale
    end
    curr  = 0
    start = curr_start
end

-- Weighted estimate of usage in the sliding window.
local weight = 1 - ((now - start) / window)
if weight < 0 then weight = 0 end
local used = curr + prev * weight

if used + cost <= limit then
    curr = curr + cost
    redis.call("HMSET", key, "prev", prev, "curr", curr, "start", start)
    if ttl > 0 then
        redis.call("EXPIRE", key, ttl)
    end
    local remaining = limit - math.ceil(used) - cost
    if remaining < 0 then remaining = 0 end
    return {1, remaining, 0}
end

-- Estimate when enough of the previous window ages out for this request to pass.
local retry_after = window / 1000.0
if prev > 0 then
    local need       = used + cost - limit
    local shed_per_s = prev / (window / 1000.0)
    if shed_per_s > 0 then
        retry_after = math.ceil(need / shed_per_s)
    end
end
if retry_after < 1 then retry_after = 1 end
if retry_after > window / 1000.0 then retry_after = math.ceil(window / 1000.0) end

return {0, 0, retry_after}
