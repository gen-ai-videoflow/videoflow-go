# Request capacity and retry policy

## Implemented safeguards

The API admits at most `MAX_CONCURRENT_REQUESTS` simultaneous requests per process (default128, allowed1–4096). Excess work receives503 and Retry-After2. Liveness/readiness probes bypass this gate. Ordinary handlers receive a30-second context; job event streams keep their streaming lifetime. Database/provider operations must continue honoring context cancellation; the gate is not a claim that arbitrary CPU work is forcibly preempted.

Public/read API traffic has an additional600 requests per minute per trusted peer limit. Existing account, mutation, playback, quote and generation limits remain. Real client attribution requires the documented restricted proxy network and trusted CloudFront viewer address. The local fallback limiter is capped at10,000 keys and rejects new keys at capacity. Strict production security/money limits fail closed when Redis is unavailable.

Redis pool size16 and connection/read/write/pool deadlines are bounded. Hidden Redis command retries are disabled (`MaxRetries=-1` before library normalization); request contexts impose a one-second limiter budget. Invalid Redis URLs fail environment configuration. `/live` is process liveness; `/ready` checks PostgreSQL and configured Redis, requiring Redis outside development; `/health` preserves its earlier PostgreSQL contract. Do not point every high-frequency load-balancer probe at dependency readiness.

The customer Node gateway admits128 concurrent forwards, bounds input to5MiB and covers body reading plus upstream headers with a30-second deadline. Cancellation closes a stalled body reader and releases the permit. A forwarding-hop header rejects recursive proxying; direct same-origin targets are rejected too. Configure the internal URL to the actual API `/v1`, never the public web gateway. Streaming response cancellation propagates after headers; the deadline is not an arbitrary30-second cap on an established event stream.

Frontend JSON calls have a30-second deadline and optional AbortSignal. They never automatically retry mutations. Generation, pending payment and upload polling use completion scheduling and single-flight guards; background tabs pause polls. Backoff is capped with jitter where task failures propagate. Explicit mutation retry must preserve the existing durable idempotency key. No shared cache mixes private responses between accounts.

`GET /v1/admin/capacity` exposes process-local admitted/completed/active/overload counters and PostgreSQL pool statistics to authorized administrators. Counts are HTTP work, not qualified views or revenue. Each API process has20 PostgreSQL connections maximum; account for every replica and worker when sizing PostgreSQL.

## Evidence and remaining capacity gates

Behavioral tests cover slow polls, cancellation, deadline, no mutation retry, proxy loop rejection, body bounds, gateway overload and a never-ending body. API race tests cover admission/panic release, limiter memory bounds and Redis options. A synthetic200-attempt burst with capacity8 admitted at most8, rejected192 predictably and released every permit. This is an isolation test, not a production throughput/SLA result.

Before public launch measure the declared deployment under realistic browse/Shorts/generation/upload mixtures, concurrent tabs, slow providers and Redis/PostgreSQL outages. Set target throughput, p95/p99, memory and queue-age budgets from that baseline; review DB query plans and total connection budget. Worker outbox dispatch still sends to SQS inside a database transaction: measure contention and design durable leasing before moving that boundary. External SDK and queue retries must be included in total attempt/cost budgets. No production load test or AWS rollout is claimed.
