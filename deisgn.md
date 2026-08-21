

# Distributed Rate Limiter
---

# What is Rate Limiting?

A **rate limiter** is a system that controls **how many requests** a client is allowed to make during a specified period of time.

Instead of allowing unlimited requests, it enforces a predefined limit.

For example

```json
100 requests / minute

OR

10 requests / second

OR

1000 requests / hour
```

If the client exceeds this limit, the request is rejected (or delayed depending on the algorithm).

Usually the server returns

```
HTTP 429 Too Many Requests
```

---

# Simple Example

Suppose an API allows

```
5 requests / minute
```

Timeline

```json
12:00:00   Request 1 ✅
12:00:05   Request 2 ✅
12:00:10   Request 3 ✅
12:00:20   Request 4 ✅
12:00:40   Request 5 ✅
12:00:45   Request 6 ❌
```

The sixth request exceeds the configured limit.

Server returns

```
429 Too Many Requests
```

---

# Real World Examples

Almost every large company uses rate limiting.

## GitHub

```
5000 API requests/hour
```

per authenticated user.

---

## Stripe

Uses Token Bucket to throttle API requests.

Allows bursts while preventing abuse.

---

## AWS API Gateway

Supports

- Token Bucket
    
- Burst limits
    
- Steady-state limits
    

---

## Cloudflare

Uses highly distributed rate limiters running across hundreds of edge locations.

---

# Why Do We Need Rate Limiting?

This is one of the first interview questions.

Most people answer

> "To prevent too many requests."

This is incomplete.

Let's understand every reason.

---

# 1. Protect Servers

Imagine

```
1 Million Users

↓

API
```

Suddenly

```json
100,000 users refresh simultaneously
```

Without rate limiting

```
CPU

100%

↓

Memory

100%

↓

DB overloaded

↓

Crash
```

Rate limiter blocks excess requests before they reach servers.

---

# 2. Prevent DoS Attacks

Imagine an attacker sends

```
500,000 requests/second
```

Without rate limiting

```
Attacker

↓

API

↓

Database

↓

Crash
```

Rate limiter drops requests early.

---

# 3. Prevent Accidental Abuse

Sometimes users accidentally create infinite loops.

Example

```javascript
while(true){
   fetch("/api")
}
```

Without rate limiting

```
Millions of requests
```

Even though the user isn't malicious.

---

# 4. Fair Resource Sharing

Suppose

```
Server Capacity

1000 Requests/sec
```

User A

```
900 requests/sec
```

User B

```
50 requests/sec
```

User C

```
50 requests/sec
```

Without rate limiting

User A monopolizes resources.

With rate limiting

```
Each user gets fair usage.
```

---

# 5. Reduce Infrastructure Cost

Every request consumes

- CPU
    
- Memory
    
- Network
    
- Database
    
- Cache
    

If your application calls

```
Stripe

OR

Twilio

OR

OpenAI API
```

every request costs money.

Blocking unnecessary requests directly saves infrastructure cost.

---

# 6. Protect Databases and Prevent spam and more 

---

# Types of Rate Limiting

Many beginners think rate limiting only applies to users.

Actually we can rate limit almost anything.

---

## Per User

```
userId

↓

100 requests/min
```

---

## Per IP

```
192.168.10.1

↓

10 requests/sec
```

Useful for unauthenticated endpoints.

---

## Per API Key

```
Developer API Key

↓

1000/day
```

Common in SaaS products.

---

## Per Endpoint

Example

```
Login

↓

5/min

Payments

↓

20/min

Upload

↓

3/min
```

Different endpoints have different limits.

---

## Per Organization

```
Company A

↓

1 Million/day
```

---

## Per Device

```
Device ID

↓

50/day
```

Useful in gaming applications.

---

## Global Rate Limit

Entire system

```
100,000 requests/sec
```

Protects infrastructure.

---

# Functional Requirements

Functional requirements describe **what the system should do**.

Based on the uploaded design documents, the key functional requirements are:

![[Pasted image 20260802103443.png]]
## 1. Limit Requests

The system must accurately enforce configured request limits.

Example:

```
100 requests/minute
```

If request number 101 arrives before the window resets, it must be blocked.

---

## 2. Support Multiple Types of Limits

The limiter should support different identifiers, such as:

- User ID
    
- IP address
    
- API key
    
- Device ID
    
- Endpoint
    
- Organization
    
- Global system limits
    

This flexibility allows the same rate limiter to be reused across many APIs.

---

## 3. Configurable Rules

Limits should not be hardcoded.

Instead, rules should be configurable, for example:

```
Login:
5 requests/minute

Payments:
20 requests/minute

Comments:
100 requests/hour
```

Production systems usually load these rules from configuration and cache them in memory.

---

## 4. Return Standard Responses

When a limit is exceeded, the client should receive:

- HTTP `429 Too Many Requests`
    
- `Retry-After` header
    
- Remaining quota information (if exposed)
    

---

## 5. Work in Distributed Systems

If a user sends requests to different API servers behind a load balancer, all servers must enforce the same global limit.

---

## 6. Support Different Algorithms

The implementation should be flexible enough to use different algorithms depending on the use case, such as:

- Fixed Window
    
- Sliding Window Log
    
- Sliding Window Counter
    
- Token Bucket
    
- Leaky Bucket
    

---

# Non-Functional Requirements

Non-functional requirements describe **how well the system should behave**.

The uploaded sources emphasize the following goals.

![[Pasted image 20260802103527.png]]

## 1. Low Latency

Every request passes through the rate limiter, so it must add only a few milliseconds of overhead.

A slow limiter makes every API slower.

---

## 2. High Availability

If the rate limiter fails completely, it should not bring down the entire application.

Many production systems prefer a **fail-open** strategy, allowing traffic temporarily instead of causing a total outage.

---

## 3. Scalability

The system must continue working as traffic grows from thousands to millions (or billions) of requests.

This generally requires:

- Stateless gateway instances
    
- Shared distributed storage (such as Redis)
    
- Horizontal scaling
    

---

## 4. Accuracy

The limiter should enforce limits as precisely as possible while balancing performance and memory usage.

Different algorithms trade off between precision and efficiency.

---

## 5. Memory Efficiency

Since counters are maintained for many active clients, memory consumption must remain bounded.

Using expiration (TTL) ensures stale counters are removed automatically.

---

## 6. Fault Tolerance

Failures of individual gateway instances or cache nodes should not stop the entire system from serving requests.

---

## 7. Consistency

Multiple servers should observe approximately the same usage for a client.

Perfect consistency is often unnecessary; many systems accept eventual consistency if it provides better scalability.

---

## 8. Configurability

Operators should be able to adjust limits without redeploying the application.

---

# Capacity Estimation (Interview Style)

A common interview exercise is estimating the scale.

Suppose the service receives:

- 100 million daily active users
    
- 1 billion requests per day
    

Approximate calculations:

- Seconds per day ≈ 86,400 (often rounded to 100,000 for mental math)
    
- Average QPS ≈ 10,000
    
- Peak QPS (assuming 5× traffic spikes) ≈ 50,000
    

Counters for active users can typically fit comfortably in memory using Redis, making an in-memory data store a practical choice for rate limiting.

---

## Where Should We Place the Rate Limiter? 

>There are three ways to implement Ratelimiter 
> 1. Client side - bad approach
> 2. server side - good for smalll applications 
> 3. Api Gateway best way to build 


### High level architecture 

 We will place the Rate Limiter logic inside the **API Gateway**. This ensures traffic is rejected at the edge of our infrastructure, saving resources for backend services.

1. **Client:** Sends an HTTP request.
    
2. **Load Balancer:** Distributes traffic to API Gateway instances.
    
3. **API Gateway:**
    
    - Serves as the entry point.
    - Runs the **Rate Limiter Middleware**.
    - The middleware calculates the user's key and asks **Redis**: "Can this user proceed?"
    - **If Yes:** Forwards request to Backend Service.
    - **If No:** Returns HTTP 429.
4. **Redis Cluster:** Stores the request counters in memory. We use Redis for its speed and atomic operations (Lua scripts).
    
5. **Backend Services:** The actual application servers (e.g., Order Service).
![[Pasted image 20260802104032.png]]

We will use **Redis** (Key-Value Store) because we need extremely fast writes and atomic counters.

## **Core flows end to end**

We will focus on the most critical path: **The Request Evaluation Flow**. This flow determines whether a user’s API request is accepted or rejected.

Since this is a distributed system, we must ensure the check is atomic (happens all at once) to prevent race conditions where two users read the same counter simultaneously.

### **Flow 1: The "Check-and-Act" Cycle (Happy Path)**

This process happens for every single incoming request. It must complete in under 10 milliseconds.

**1. Request Interception**

- **Action:** A client sends a GET /api/orders request.
- **Gateway:** The API Gateway (middleware) intercepts the request before it reaches the backend Order Service.
- **Identification:** The middleware extracts the User_ID (e.g., user_55) from the JWT token or API Key.

**2. Constructing the Keys**

- The middleware identifies the current time window and the previous window.
- **Example:**
    - **Current Time:** 10:01:15 (15 seconds into the minute).
    - **Current Key:** limiter:user_55:10:01
    - **Previous Key:** limiter:user_55:10:00

**3. Atomic Check (The Lua Script)**

- The Gateway sends a single Lua script to the Redis Cluster.
- **Why Lua?** Redis executes Lua scripts atomically. No other request can interrupt the script while it runs. This performs a "Get, Calculate, and Set" operation in one go, preventing race conditions.
- **The Script Logic:**
    - **Fetch:** It retrieves the values for Current Key and Previous Key.
    - **Calculate:** It runs the "Weighted Average" formula (detailed below).
    - **Decision:**
        - If result  Limit: It increments the Current Key and returns ALLOW.
        - If result  Limit: It returns BLOCK without incrementing.

**4. Forwarding the Request**

- **Gateway:** Since Redis returned ALLOW, the Gateway forwards the HTTP request to the backend **Order Service**.
- **Response:** The Order Service processes the data and returns it to the client. The user perceives no delay.
![[Pasted image 20260802110852.png]]

# Rate Limiting algorithms 

### Fixed window counter algorithm

This algorithm divides the time into fixed intervals called **windows** and assigns a counter to each window. When a specific window receives a request, the counter is incremented by one. Once the counter reaches its limit, new requests are discarded in that window.

As shown in the below figure, a dotted line represents the limit in each window. If the counter is lower than the limit, forward the request; otherwise, discard the request.
![[Pasted image 20260802111536.png]]

A major problem with this algorithm is that a burst of traffic at the edges of time windows could cause more requests than allowed quota to go through. Consider the following case:

Lets say 100 req per minute and user sends 100 req at 2.00 and 100 in 2.01 thts 200 req in 1 min of timeline this is burst problem 
#### Essential parameters

The fixed window counter algorithm requires the following parameters:

- **Window size (W):** It represents the size of the time window. It can be a minute, an hour, or any other suitable time slice.
- **Rate limit (R):** It shows the number of requests allowed per time window.
- **Requests count (N):** This parameter shows the number of incoming requests per window. The incoming requests are allowed if NN is less than or equal to RR.

#### Advantages

- It is also space efficient due to constraints on the rate of requests.
- As compared to token bucket-style algorithms (that discard the new requests if there aren’t enough tokens), this algorithm services the new requests.

#### Disadvantages

- A consistent burst of traffic (twice the number of allowed requests per window) at the window edges could cause a potential decrease in performance.

# Sliding window Log Algorithm

- The algorithm keeps track of request timestamps. Timestamp data is usually kept in cache, such as sorted sets of Redis 
    
- When a new request comes in, remove all the outdated timestamps. Outdated timestamps are defined as those older than the start of the current time window.
    
- Add timestamp of the new request to the log.
    
- If the log size is the same or lower than the allowed count, a request is accepted. Otherwise, it is rejected.
-
Remove old timestamps that fall **outside the current window**.
    
- Check how many timestamps remain.
    
- If count < limit → accept request.
    
- If count ≥ limit → reject request

We explain the algorithm with an example as revealed in


![[Pasted image 20260802112753.png]]
- Formula: **Keep only requests with**  
- 
- ```
  timestamp ≥ (current_time – window_size)
  ```

    
- Example: window size = 60 sec, current Time = 02:25 → keep requests ≥ 01:25.
    
- This way, the window **slides dynamically** with each request, not fixed at round minute
#### Disadvantages

- It consumes extra memory for storing additional information, the time stamps of incoming requests. It keeps the time stamps to provide a dynamic window, even if the request is rejected.

# leaky Bucket algorithm
The leaking bucket algorithm is similar to the token bucket except that requests are processed at a fixed rate. It is usually implemented with a first-in-first-out (FIFO) queue. The algorithm works as follows:

- When a request arrives, the system checks if the queue is full. If it is not full, the request is added to the queue.
    
- Otherwise, the request is dropped.
    
- Requests are pulled from the queue and processed at regular intervals.
    

Figure 7 explains how the algorithm works.


Leaking bucket algorithm takes the following two parameters:

- Bucket size: it is equal to the queue size. The queue holds the requests to be processed at a fixed rate.
    
- Outflow rate: it defines how many requests can be processed at a fixed rate, usually in seconds.
    

Shopify, an ecommerce company, uses leaky buckets for rate-limiting 

### Pros:

- Memory efficient given the limited queue size.
    
- Requests are processed at a fixed rate therefore it is suitable for use cases that a stable outflow rate is needed.
    

### Cons:

- A burst of traffic fills up the queue with old requests, and if they are not processed in time, recent requests will be rate limited.
    
- There are two parameters in the algorithm. It might not be easy to tune them properly.

# Sliding winodow Counter
## Definition

The **Sliding Window Counter** is a **hybrid rate limiting algorithm** that combines the **Fixed Window Counter** and the **Sliding Window Log** algorithms.

It stores only the **current window counter** and the **previous window counter**, then estimates the number of requests in the current sliding window by weighting the previous window based on how much of it overlaps with the current window.

This provides **better accuracy than Fixed Window** while using **much less memory than Sliding Window Log**
![[Pasted image 20260802115220.png]]
# Core Idea

Instead of storing every request timestamp, the algorithm stores only:

- Current window request count
- Previous window request count

When a new request arrives, it estimates how many requests exist in the current sliding window using these two counters.


# Token Bucket Algorithm

 - A token bucket is a container that has pre-defined capacity. Tokens are put in the bucket at preset rates periodically. Once the bucket is full, no more tokens are added. As shown in Figure 4, the token bucket capacity is 4. The refiller puts 2 tokens into the bucket every second. Once the bucket is full, extra tokens will overflow.
 - ![[Pasted image 20260802115817.png]]

   - Each request consumes one token. When a request arrives, we check if there are enough tokens in the bucket. Figure 5 explains how it works.
    
- If there are enough tokens, we take one token out for each request, and the request goes through.
    
- If there are not enough tokens, the request is dropped.
![[Pasted image 20260802120428.png]]

We require the following essential parameters to implement the token bucket algorithm:

- **Bucket capacity (C:** The maximum number of tokens that can reside in the bucket.
- **Rate limit (R):** The number of requests we want to limit per unit time.
- **Refill rate (1/R) :** The duration after which a token is added to the bucket.
- **Requests count (N:** This parameter tracks the number of incoming requests and compares them with the bucket’s capacity.

#### Advantages

- This algorithm can cause a burst of traffic as long as there are enough tokens in the bucket.
- It is space efficient. The memory needed for the algorithm is nominal due to limited states.

#### Disadvantages

- Choosing an optimal value for the essential parameters is a difficult task.

# High level Design of Distributed Rate Limiter

###  Rate Limiter - Core Flows

## Flow 1: Check-and-Act Cycle (Happy Path)
>  Must complete in < 10ms

1. **Request Interception** — Gateway intercepts `GET /api/orders`, extracts `User_ID` from JWT/API key
2. **Constructing Keys** — e.g. at `10:01:15`:
   - Current: `limiter:user_55:10:01`
   - Previous: `limiter:user_55:10:00`
3. **Atomic Check (Lua Script)**
   - Redis executes Lua atomically → no race conditions
   - Fetch → Calculate → Decide (increment + ALLOW, or BLOCK without incrementing)
4. **Forwarding** — ALLOW → request goes to backend, user sees no delay

## Flow 2: Weighted Average Calculation
**Scenario:** limit = 10 req/min, time = `10:01:15`, previous window = 20 reqs, current window = 2 reqs so far.

- Overlap weight: \( \text{Weight} = 1 - \frac{15}{60} = 0.75 \) (1)
- Estimated rate: \( 2 + (20 \times 0.75) = 17 \) (2)
- Since \(17 > 10\) → **BLOCKED**

## Flow 3: Handling Rejection
1. **Short-circuit** — request never reaches backend
2. **Local caching optimization** — Gateway caches `user_55:BLOCKED` in local RAM for ~1s so a 1000 req/s bot only hits Redis once
3. **429 Response headers:**
   - `X-Ratelimit-Limit: 10`
   - `X-Ratelimit-Remaining: 0`
   - `Retry-After: 45`

## Flow 4: Failure Handling (Fail-Open)
1. Gateway sends Lua script, no reply within 10ms → timeout
2. Default decision = **ALLOW** (never block due to infra failure)
3. Log "Redis Timeout" for DevOps, let traffic pass

> the contradiction in your notes
> Section 2 ("How do we ensure high availability") argues for **fail-closed** on a social platform to prevent cascading failure during viral spikes, but Flow 4 and Section 12 default to **fail-open**. Pick one per endpoint criticality — see [[Rate Limiter - Reliability#Fail Open vs Fail Closed]].


###  Rate Limiter - Scaling

## Pattern: Scaling Writes
Rate limiters are a classic **scaling writes** problem — millions of atomic read-modify-write counter updates/sec.

## Sharding Strategy
- A single Redis node handles ~50k-100k ops/sec (HMGET + HSET per check)
- Target: 1M req/s → need ~10 shards at ~100k ops/sec each
- **Shard key:** hash(User_ID) / hash(IP) / hash(API_Key) depending on client type
- Must be **consistent** — a client's state must always land on the same shard, or rate limiting breaks

## Consistent Hashing / Redis Cluster
- Production: use **Redis Cluster** — auto-shards across 16,384 hash slots
- No need to hand-roll consistent hashing in the gateway; Redis Cluster routes automatically

## Stateless Gateways
- API Gateway layer is stateless → auto-scale on CPU load

## Latency Optimizations
- **Connection pooling** — avoids TCP handshake (20-50ms) per request
- **Geographic distribution** — regional Redis clusters near users; accept eventual consistency across regions
- Lower priority: Lua/pipelining, request batching — mention briefly, don't over-engineer

## Hot Keys (Viral Content)
| Scenario | Mitigation |
|---|---|
| Legitimate high-volume client | Client-side rate limiting, request batching, premium tiers |
| Abusive traffic | Auto-block after repeated violations, DDoS protection (Cloudflare/AWS Shield) |
| Shared IP (NAT/public WiFi) | Set higher IP limits upfront, prefer authenticated user limits |

###  Rate Limiter - Reliability

## Fail Open vs Fail Closed

| | Fail-Closed | Fail-Open |
|---|---|---|
| Behavior | Reject all (503/429) when Redis unreachable | Allow all when Redis unreachable |
| Risk | Takes API offline during outage; retry storms | Loses protection exactly when traffic is spiking |
| Best for | Payments, high-security systems | General APIs, social platforms avoiding total collapse |

## High Availability
- **Master-replica replication** per shard
- Automatic failover via Redis Cluster (promotes replica on master failure)
- Trade-off: infra cost + replication lag

## Circuit Breakers & Backpressure
- Circuit breaker opens on Redis error spikes → bypass rate check for ~30s, default ALLOW
- **Jitter** on `Retry-After` (e.g. 29s/30s/31s) to avoid synchronized retry storms

## Data Durability
- **TTL** = Window_Size × 2 on Redis keys → auto-cleanup, low memory
- Disable/reduce RDB/AOF persistence — losing counters on crash is an acceptable trade-off for write speed

## Monitoring
- Redis health: CPU, memory, network
- App-level: rate limit success rate, latency, alerts on fail-open triggers
