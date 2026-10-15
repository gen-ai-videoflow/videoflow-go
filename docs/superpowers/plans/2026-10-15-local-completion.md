# Local completion after repository publication

Continue the approved local product work without AWS deployment or payment activation. Keep the viewer navigation, original Studio canvas and existing shadcn styling.

- Add account-owned active session listing with opaque public IDs, creation/activity/expiry and current-session indication. Revoke one other session or all other sessions; never expose session tokens/hashes, and never allow one account to revoke another's session.
- Add a featured public video picker to the profile editor and show the selected video first on public profiles. Existing ownership/publication/visibility checks remain authoritative; unpinning and stale pins must be safe.
- Write reviewable launch copy, demo scripts, outreach drafts and a first-month content checklist. Do not send messages or claim unactivated earnings/payment services are live.
- Verify HTTP ownership/revocation, profile visibility, frontend checks/builds and local preview. Update remaining-work status, synchronize and publish the two private repositories.

## Verification and limits

- Session HTTP regression: unauthenticated access denied, credentials/hashes omitted, expired/idle sessions hidden, foreign ownership denied, current session protected, individual and bulk revocation enforced, pending account-bound challenges cleared. Cleanup removes expired/idle sessions while preserving active sessions.
- Profile regression: published-public ownership checks, private/unlisted rejection and stale-feature hiding remain covered. Featured ordering now takes priority over creation time, retaining the feature in the 60-video profile result.
- Full Go race suite passed with PostgreSQL/Redis after clearing disk pressure; earlier ENOSPC failures were environmental and rerun. Go vet passed. Svelte check reports zero errors/warnings, desktop/static and cloud/Node builds passed.
- Browser verified mobile session listing and sign-out, profile feature selection/save/display/unpin, desktop/mobile overflow. Temporary metadata-only profile fixture was explicitly labelled and removed; no real media-generation/playback verification is claimed.
- Browser caught missing JSON on single-session DELETE; fixed client and documented the empty JSON request in OpenAPI. Rebuilt preview verification passed: individual and bulk sign-out both left the current session authenticated, and returned the correct success notices.
- Launch content is prepared in docs/launch/LAUNCH-CONTENT.md; no messages sent, no advertising bought, no earnings promises published. Live provider/payment/payout and AWS gates remain deferred.
