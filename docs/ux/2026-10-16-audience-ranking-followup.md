# Audience analytics and browsing ranking

## User journeys

Creators can open Earnings → Audience overview, select7/28/90days, inspect qualified views, accepted watch minutes, watch sessions, privacy-protected unique viewers, daily UTC activity and up to50 videos ordered by qualified views. Per-video watch-through and watch-time-reach estimates are shown only for measured source durations and a cohort of at least5 unique viewers. Small cohorts show an em dash, not fabricated zeroes. The interface explains authenticated tracking coverage, replays, thresholds and refresh time. These metrics never calculate payout amounts.

Discover and Shorts now expose Recent, Popular, For you and Following ordering. Recent retains existing chronological behavior. Popular uses accepted watch seconds and qualified-view counters among the newest2000 matching public videos. For you additionally favors followed creators, creators of up to300 recently liked videos, and creators in up to300 recent history entries from the past30days. Disabling viewing history removes history as a ranking input; deliberate likes/follows continue to apply. The Discover copy explains the current inputs and links to history settings. No raw viewing history is returned by feed ranking.

## Backend boundaries

- New authenticated endpoint `/creator/analytics/detail?days=7|28|90`; channel ownership derives from the authenticated account, never a requested channel ID.
- Analytics use accepted server watch events and deduplicated daily qualified-view rows. Client resume/playhead values are not earnings or analytics watch totals. Existing elapsed-time/idempotency heartbeat guards remain unchanged.
- All analytics queries use a read-only repeatable-read snapshot. Aggregation covers a bounded date range, up to50 per-video rows and up to90 daily rows. Existing event/video/time and recent-history indexes are reused. Migration039 adds daily-view/day and recent-like indexes.
- Unique viewers and engagement rates are suppressed below5 viewers. No raw viewer identifiers, event payloads or viewer hashes are returned.
- “Watch-through” means accepted watch time at least90% of measured duration; watch-time reach shows25/50/75/100% duration thresholds. Replays may contribute. These are engagement estimates, not frame-by-frame retention curves.
- Feed candidate pool and interest lists are bounded. Rank, format, query, following filter, user and history-consent state bind each cursor; a cursor from another ranking/consent state is rejected.
- Ranking preserves published/public gates, hidden-channel preferences and suspended-creator exclusion. Premium remains a discoverable locked preview; existing playback authorization remains authoritative. No Premium manifests or private object keys are exposed by ranking.
- Live ranking scores can change between pagination requests; client deduplication prevents repeated cards. It is not a frozen recommendation snapshot or ML recommender.

## Verification

Svelte check returned0errors0warnings after the first implementation pass. Real PostgreSQL race-enabled tests cover creator-only aggregates, synthetic counter inflation independence, privacy suppression, date-range limits, ranking/consent changes, explicit likes, private/hidden content and incompatible cursors; final results recorded in the parent release report. Existing qualified-heartbeat tests remain required in the combined backend test run.

No financial formulas, reward activation, live provider calls or AWS resources were changed.
