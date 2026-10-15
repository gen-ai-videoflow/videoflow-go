# Standard Watch and Shorts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver familiar standard watching and accessible vertical Shorts backed by real eligibility, progress and watch accounting.

**Architecture:** One playback controller with explicit active/visible/muted state serves WatchPage and ShortsFeed. Data APIs return trusted metadata, access reasons and cursor-based results; Postgres remains authoritative and Redis is optional bounded caching.

**Tech Stack:** Svelte 5/shadcn, native video/HLS.js, Go/PostgreSQL, existing signed-media grants and watch events.

**Spec:** `docs/ux/2026-10-15-viewing-publishing-spec.md` sections 4–5, 8–11; master plan constraints/review focus apply. Depends on Phase B's measured source metadata and stable source/visibility model.

## Task C1: Viewing API, eligibility and feed cursors

**Files:** publishing.go, engagement.go, profile.go, server.go; create viewing.go/viewing_test.go and next metadata/index migration; frontend generated contract and +layout.server.ts.

**Interface:** public VideoSummary adds trusted width/height/duration/origin/format, safe thumbnail, publication date, creator and explicit access reason; no private storage key. GET /feed?format=standard|short&cursor=… returns items,next_cursor. GET /videos/:id/related returns eligible items excluding current. Cursor is bounded, validated and tied to filter/order; personalised response never uses anonymous shared cache.

- [ ] Write failing tests for private/unlisted/blocked exclusion, hidden creators, expired Premium entitlement, malformed cursor, stable page ordering/dedup and eligibility based on probed metadata.
- [ ] Implement deterministic cursor pagination with an ID tiebreaker; backfill unknown metadata conservatively. Keep existing feed callers compatible or migrate them together.
- [ ] Return differentiated public access gates without leaking inaccessible private metadata; recheck authorisation at grant issuance, not only metadata fetch.
- [ ] Add simple related ordering and reason labels grounded in actual signals; no ML claims. Test visibility/entitlement changes while cached metadata exists.
- [ ] Add public-profile Videos/Shorts/About tabs using the same eligibility rules; existing feature selection/count privacy remains correct. Targeted race tests and contract checks before commit.

## Task C2: Reusable player and standard watch

**Files:** refactor VideoPlayer.svelte; create frontend/src/lib/cloud/viewing/playback-controller.ts and frontend/src/lib/cloud/viewing/{WatchPage,AccessGate,RelatedVideos}.svelte; update CloudApp.svelte; create frontend/scripts/check-playback.mjs for controller lifecycle tests plus real browser fixtures.

**Interface:** player input videoId, active, muted, captions preference and optional resumeAt; callbacks ready/error/progress/ended. Changing video identity destroys the previous HLS instance and aborts old grants. UI preferences are separate from entitlement and accounting.

- [ ] Add failing lifecycle tests: stale grant after navigation, rapid ID switches, hidden/offscreen playback, expired resume grants, teardown/no leaked timers and autoplay rejection.
- [ ] Preserve existing watch-clock protections and auth renewal; one initial grant request per mounted identity, safe retries and progress preservation. Never retry unauthorised media indefinitely.
- [ ] Build player-led desktop two-column/mobile stacked WatchPage with creator/follow/share, accurate metadata, description, related videos and contextual gates. Keep native accessible controls as baseline.
- [ ] Add durable progress/history endpoint (PUT /videos/:id/progress) and bounded coalesced writes obeying history_enabled. Progress is not reward evidence. Clear history clears progress; a new source version invalidates stale positions.
- [ ] Verify real local portrait/landscape/no-audio/captioned fixtures, lock/unavailable states, >5-minute pause/grant renewal, keyboard controls, autoplay-next off and recovery. Run relevant frontend/backend tests and commit.

## Task C3: Shorts feed and lifecycle

**Files:** create frontend/src/lib/cloud/viewing/{ShortsFeed,ShortsItem}.svelte and shorts-state.ts; extend ViewerShell/CloudApp routes; reuse player/controller and preference APIs.

**Interface:** /shorts and /shorts/:id share a cursor feed state with stable activeVideoId, seen IDs and explicit next/previous. Only current player actively streams; maximum current+one nearby mounted instance. Direct-link inaccessible items receive a useful gate/error, not a broken scrolling feed.

- [ ] Write failing state/lifecycle tests for fast swipes, duplicate pages, end-of-feed, fetch failure, deep link/back restoration, hidden tab and user-paused state.
- [ ] Implement scroll snapping and visible navigation alternatives; single owner for active player; pause on offscreen/report/settings, stop prefetch for data saving and locked content.
- [ ] Handle muted autoplay refusal with Play; remember user mute/caption choices, provide sound toggle, and honour reduced motion. Ignore navigation keys in forms and dialogs.
- [ ] Build mobile viewport and contained desktop portrait layouts with readable creator/actions/captions, touch targets and no duplicate navigation. Do not rely on swipe alone.
- [ ] Verify 3/5/10/180-second, Premium, unavailable and empty fixtures; continuous swiping must not grow media connections/HLS instances without bound. Batched visual/keyboard review, then commit.

## Task C4: Accurate metrics and durable viewer actions

**Files:** engagement.go/engagement_test.go and related migration; new reactions.go/reactions_test.go, progress APIs; frontend viewing actions and library/history components; API contract.

- [ ] Propose/confirm the duration-aware public-view threshold independently of rewards. Add failing tests for short clips, loops, seek jumps, repeated IDs, parallel tabs, owner views, background time and entitlement loss.
- [ ] Implement the approved public count policy with server bounds/dedup; distinct starts/views/watch/completion/reward eligibility. No change to capped financial ledger activation.
- [ ] Implement PUT/DELETE like/save with unique(user,video), ownership/access checks and rollback-safe optimistic UI. Add saved/history pagination and “Not interested” backed by actual preferences.
- [ ] Expose only implemented actions; guest actions lead to safe return-target authentication. Watch-next respects privacy/hidden creators. Comments remain outside this phase until moderation is implemented.
- [ ] Run race/concurrency tests, route/controller tests and local populated media review. Sync both repos and require CI before declaring this phase complete.
