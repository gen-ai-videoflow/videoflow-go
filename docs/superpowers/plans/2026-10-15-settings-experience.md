# Settings Experience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give viewers and creators one coherent settings experience with real persisted preferences.

**Architecture:** A reusable settings shell hosts existing profile/security/payment modules and focused account/privacy/playback forms. Keep CloudApp as route composition rather than another growing inline form.

**Tech Stack:** Existing Svelte 5/shadcn-svelte, Go/PostgreSQL, existing authentication middleware.

**Spec:** `docs/ux/2026-10-15-viewing-publishing-spec.md` sections 2–3, 11; global constraints/review focus in `2026-10-15-experience-master-plan.md` apply.

## Task A1: Settings routes and task hierarchy

**Files:** create frontend/src/lib/cloud/settings/{SettingsShell,AccountSettings,PrivacySettings}.svelte; modify frontend/src/lib/cloud/{CloudApp,ViewerShell}.svelte and cloud.css; reuse ProfileEditor/SecurityPanel/SessionManager/Payments unchanged where possible; test frontend/scripts/check-settings.mjs (new route/compatibility checks, not CSS snapshots).

**Interface:** canonical /settings/account|profile|security|privacy|playback|billing. Existing /settings, /profile, /security and /payments entry points remain compatible. Billing links to credits and Premium without moving creator payouts into settings. Unknown section yields useful not-found/back navigation.

- [ ] Add failing route/return-target checks for guest redirects, old URLs, query preservation and direct section refresh.
- [ ] Build persistent desktop settings rail and mobile section navigation using existing primitives. Remove irrelevant eyebrow and move verification controls into a properly spaced action group.
- [ ] Compose existing security/profile/payment modules; preserve one-time secrets, token return destinations, invoice recovery and MFA reauthentication.
- [ ] Make deletion/clear-history confirmations accessible and explicit; preserve current server semantics and dirty form state on errors. Do not add email/username edit controls without backend support.
- [ ] Verify guest/signed-in/unverified states, long account text, keyboard focus, browser Back and 390px/200% zoom. Existing security and payment regression checks must pass before commit.

## Task A2: Durable playback/privacy preferences

**Files:** create internal/cloud/preferences.go and preferences_test.go; add next migration with cloud_video_preferences; modify security/account route group in server.go and history clearing in maintenance.go; create frontend/src/lib/cloud/settings/PlaybackSettings.svelte and preferences.ts; update generated contract script and cmd/cloud-migrate/main_test.go.

**Interface:** GET /settings/preferences returns schema_version, captions_enabled, caption_language, autoplay_next=false, shorts_autoplay=true (proposed), reduce_motion='system', data_saving=false, history_enabled=true. PATCH accepts only known fields with bounded language codes/enums. Server checks ownership from session; clients cannot supply user IDs. Profile display names and security factors are not part of this resource.

- [ ] Add failing HTTP tests for guest access, partial updates, invalid/unknown fields, per-account isolation and defaults for existing accounts.
- [ ] Persist preferences with checked constraints and a schema version; add restricted-role tests. Reuse existing JSON/origin/mutation limits.
- [ ] Wire playback/privacy forms with save/loading/error feedback and dirty-navigation guard. Browser-only guest preferences must not leak into another signed-in account.
- [ ] Ensure clear history removes newly added progress records when introduced in Phase C and future writes obey history_enabled; financial/watch aggregate evidence stays intact.
- [ ] Test account switch, offline save, slow response after navigation and reset-to-default behaviour. Run targeted Go race tests, Svelte check, both builds, then commit.

## Task A3: Viewer shell quality

**Files:** ViewerShell.svelte, CloudApp.svelte, cloud.css; shared new RouteLoading.svelte/EmptyState.svelte only where actual reuse exists.

- [ ] Introduce Discover/Shorts/Following viewer destinations and clear Studio entry; hide unavailable routes until the corresponding feature is enabled.
- [ ] Replace generic creator loading copy with route-specific skeleton/status. Fix empty Discover's self-link with a meaningful creator action, and retain viewer alternatives.
- [ ] Add route-request cancellation/version checks to prevent late response overwrite. Preserve search and feed position across Back/navigation.
- [ ] Inspect complete desktop/mobile states in the phase's single batch; one correction batch and confirmation. No global redesign or canvas edits.
