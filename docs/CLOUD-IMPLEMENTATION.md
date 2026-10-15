# Cloud implementation status

This branch implements an initial SaaS application, not a completed production launch. The master specification remains the target; this document records actual coverage and limitations.

## Implemented

- Separate PostgreSQL cloud API, background worker, and audited operator CLI; existing desktop server remains separate.
- SvelteKit cloud mode with existing shadcn-svelte, public SSR metadata, same-origin API gateway, responsive navigation, account recovery, projects, credit purchases, generation quotes/jobs, channels, publishing, memberships, earnings and credentialed HLS playback.
- Application-owned Argon2id credentials, opaque hashed sessions, verification/recovery tokens, expiry/revocation, origin checks and Redis-backed sensitive request admission. No Cognito.
- Durable launch quotas: 100 projects and 500 total studio entities per workspace, including archive imports.
- Workspace ownership checks; the cloud server never mounts unscoped legacy routes or local storage.
- Transactional credit reservation/settlement, balanced credit/money postings, replay-safe payment webhooks and checkout keys, proportional cumulative refunds, debt accounting after spent-credit refunds, current-state dispute reconciliation and independent credit/payout holds. The first payout transfer rechecks matured net earnings after late refunds.
- Reviewed model allowlist, daily OpenRouter video catalog, cheapest eligible supported pricing, frozen quote policy/cost ceiling, and transactional daily provider budget.
- PostgreSQL outbox to SQS, durable generation attempts/leases, stale-worker settlement fencing, interrupted-submission reconciliation, S3 outputs, MediaConvert HLS, encoding event correlation and periodic recovery.
- Scoped short-lived playback cookies issued on the media host using single-use authorization grants; original downloads require workspace access. Audio/silent-source handling and source aspect ratio preservation.
- Workspace-scoped scripts, characters, scenes, nested shots, asset metadata, captions, and project import/export with remapped identifiers; a corresponding shadcn studio editor includes scene/shot ordering and timing, plus an @xyflow scene/shot graph with zoom, keyboard selection and synchronized editor navigation. Publication snapshots captions into protected WebVTT sidecars.
- Durable screenplay → scene/shot generation → independent video clips → FFmpeg assembly, with one parent credit reservation, frozen story/video policy, aggregate spending admission, ambiguous-submission holds, current-day spending renewal, and final-output settlement. Work waiting for its first admission can be cancelled to release credits; admitted or ambiguous paid work remains protected for reconciliation. Owned successful project outputs can also be assembled in a chosen order. Launch limits are 1–4 generated clips, 5–10 seconds each, at most 30 seconds total; manual assembly accepts up to 8 sources within the same 30-second limit.
- Explicit operator migrations and runtime-role provisioning. Production API/worker startup checks schema compatibility and rejects elevated/owner database credentials. Runtime cannot change schema or mutate journal history; financial operator tables remain separate.
- Size/type-bound quarantine uploads with verified-user checks and durable per-workspace quotas. Quarantine objects are not automatically promoted into trusted project assets.
- Channels, follows, private/unlisted/public access, member-only grants, basic search/following/trending feeds, hidden-channel preferences, rate-limited watch events, deduplicated view counters, and a per-viewer elapsed-time clock.
- Membership plan drafts, gated recurring checkout, provider-state entitlement reconciliation, partial refunds, Connect onboarding, and pending creator earnings.
- Capped deterministic engagement reward allocation/monthly close using externally approved accounting and qualified-watch inputs. Operator payout holds/thresholds, country/capability checks, frozen batches and stable provider transfer keys.
- Bounded operational cleanup, confirmed-deletion upload quota recovery, archive-before-delete analytics retention, viewing-history clearing and account-deletion requests that revoke sessions while preserving financial records.
- Private AWS Terraform foundation, local PostgreSQL/Redis/LocalStack compose, Dockerfiles, CI, API contract/shared wire types, and safe source-repository extraction.

## Deliberately gated configuration

Paid generation needs provider credentials, reviewed model IDs and positive per-job/daily cost caps. Story pipelines additionally require OPENROUTER_STORY_MODEL, STORY_CREDITS and STORY_MAX_COST_MICROS; local assembly needs ASSEMBLY_CREDITS. Story requests route by price with provider token/request price ceilings and bounded output tokens; no user can select or supply a model. Credit checkout needs Stripe credentials, verified email and BILLING_ENABLED. Memberships also need an explicit commission policy and an onboarded eligible creator. Payout transfers need supported countries, CASH_PAYOUTS_ENABLED and an operator execution flag. Rewards default off. None of these controls represents a live provider approval, tax decision or production readiness guarantee.

Video cost admission uses the reviewed catalog snapshot; it cannot guarantee a third-party invoice if provider prices change after quoting. Reconcile actual usage and configure provider-account spending limits before public paid generation. Story requests additionally send OpenRouter token-price ceilings ([provider routing documentation](https://openrouter.ai/docs/guides/routing/provider-selection)).

Terraform creates CloudFront distributions but does not subscribe them to a flat-rate pricing plan. Standard usage billing applies until an operator separately activates a compatible plan. The earlier $15/$200 delivery comparisons are conditional on that subscription, not Terraform's default bill. Both media and web distributions need their own pricing review. Budget alarms are notifications, not hard caps.

## Remaining master-spec work

- Full parity with the desktop node canvas and frame-accurate nonlinear timeline, transitions, voice/music mixing and extended-duration rendering. Cloud storyboard/shot ordering, timing, automated screenplay-to-clips and bounded assembly are implemented.
- Automated content safety/copyright checks, appeals and administrative MFA interface. Current moderation is reports plus audited operator blocking; a grant expiry limits revocation delay and cannot recall downloaded content.
- Automated fraud qualification for monetary rewards, auditable revenue recognition inputs and accounting review. Watch events alone must not authorize money. Current recommendations are deterministic follow/recency/trending rules, not a trained personalization model.
- High-volume analytics queue ingestion, advanced recommendation controls, notifications, comments, scheduling and publication collections. S3 archival and account-level history clearing are implemented.
- Full upload content inspection/promotion into trusted assets; quota reclamation is implemented only after S3 deletion is confirmed, never merely because a URL or lifecycle timer elapsed.
- Lost-dispute financial reconciliation, transfer recovery and full tax/accounting treatment. Dispute lifecycle ingestion and credit/payout holds are implemented; loss settlement remains an audited operations responsibility (see CLOUD-PAYMENT-OPERATIONS.md).
- Executed restoration/failover/load drills, operational dashboards and complete retention/deletion enforcement. Restricted runtime-role provisioning and controlled explicit migrations are implemented; restoration/credential-rotation runbooks still require deployment-specific execution.
- Actual AWS apply, DNS/TLS/SES approval, selected CloudFront billing-plan activation, live provider/Stripe sandbox integration and end-to-end HLS playback against deployed services.
- Git history-preserving extraction and remote repository publication if required. The supplied split creates independent local source snapshots, preserving the original repository.

## Verification record

Local PostgreSQL-backed tests cover account lifecycle, concurrent reservations, webhook/checkout replay, refunds/debt/dispute holds, late-refund payout checks, concurrent operator resolution, pipeline stage fencing, daily rollover admission, real FFmpeg mixed-audio/duration bounds, lease fencing, encoding event correlation, upload quota races, signed-cookie scope/single-use grants, provider budgets, membership lifecycle, reward rounding/overflow and payout timeout/concurrent retry safety. Frontend checks and desktop/cloud builds, Terraform schema/format validation, compose configuration and source extraction are verified separately.

The collaborative browser preview failed to navigate/snapshot reliably in this session. HTTP smoke checks through the SvelteKit gateway passed; this is not equivalent to browser interaction, visual QA or live streaming verification. Container builds and final test outcomes are recorded in the implementation plan ledger after completion.
