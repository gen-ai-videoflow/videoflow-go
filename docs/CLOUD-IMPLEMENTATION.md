# Cloud implementation status

This branch implements an initial SaaS application, not a completed production launch. The master specification remains the target; this document records actual coverage and limitations.

## Implemented

- Separate PostgreSQL cloud API, background worker, and audited operator CLI; existing desktop server remains separate.
- SvelteKit cloud mode with existing shadcn-svelte, public SSR metadata, same-origin API gateway, responsive navigation, account recovery, projects, credit purchases, generation quotes/jobs, channels, publishing, memberships, earnings and credentialed HLS playback.
- Application-owned Argon2id credentials, opaque hashed sessions, verification/recovery tokens, expiry/revocation, origin checks and Redis-backed sensitive request admission. No Cognito.
- Durable launch quotas: 100 projects and 500 total studio entities per workspace, including archive imports.
- Workspace ownership checks; the cloud server never mounts unscoped legacy routes or local storage.
- Transactional credit reservation/settlement, balanced credit/money postings, replay-safe payment webhooks and checkout keys, proportional cumulative refunds, and debt accounting after spent-credit refunds.
- Reviewed model allowlist, daily OpenRouter video catalog, cheapest eligible supported pricing, frozen quote policy/cost ceiling, and transactional daily provider budget.
- PostgreSQL outbox to SQS, durable generation attempts/leases, stale-worker settlement fencing, interrupted-submission reconciliation, S3 outputs, MediaConvert HLS, encoding event correlation and periodic recovery.
- Scoped short-lived playback cookies issued on the media host using single-use authorization grants; original downloads require workspace access. Audio/silent-source handling and source aspect ratio preservation.
- Workspace-scoped scripts, characters, scenes, nested shots, asset metadata, captions, and project import/export with remapped identifiers; a corresponding advanced studio editor uses the cloud API. Publication snapshots captions into protected WebVTT sidecars.
- Size/type-bound quarantine uploads with verified-user checks and durable per-workspace quotas. Quarantine objects are not automatically promoted into trusted project assets.
- Channels, follows, private/unlisted/public access, member-only grants, basic search/following/trending feeds, hidden-channel preferences, rate-limited watch events, deduplicated view counters, and a per-viewer elapsed-time clock.
- Membership plan drafts, gated recurring checkout, provider-state entitlement reconciliation, partial refunds, Connect onboarding, and pending creator earnings.
- Capped deterministic engagement reward allocation/monthly close using externally approved accounting and qualified-watch inputs. Operator payout holds/thresholds, country/capability checks, frozen batches and stable provider transfer keys.
- Bounded operational cleanup, confirmed-deletion upload quota recovery, archive-before-delete analytics retention, viewing-history clearing and account-deletion requests that revoke sessions while preserving financial records.
- Private AWS Terraform foundation, local PostgreSQL/Redis/LocalStack compose, Dockerfiles, CI, API contract/shared wire types, and safe source-repository extraction.

## Deliberately gated configuration

Paid generation needs provider credentials, reviewed model IDs and positive per-job/daily cost caps. Credit checkout needs Stripe credentials, verified email and BILLING_ENABLED. Memberships also need an explicit commission policy and an onboarded eligible creator. Payout transfers need supported countries, CASH_PAYOUTS_ENABLED and an operator execution flag. Rewards default off. None of these controls represents a live provider approval, tax decision or production readiness guarantee.

Terraform creates CloudFront distributions but does not subscribe them to a flat-rate pricing plan. Standard usage billing applies until an operator separately activates a compatible plan. The earlier $15/$200 delivery comparisons are conditional on that subscription, not Terraform's default bill. Both media and web distributions need their own pricing review. Budget alarms are notifications, not hard caps.

## Remaining master-spec work

- Complete the desktop visual canvas/timeline experience and automated multi-stage screenplay/scene assembly pipeline. Cloud resource editing and captions are implemented, but the initial cloud generator submits a prompt-based provider video job.
- Automated content safety/copyright checks, appeals and administrative MFA interface. Current moderation is reports plus audited operator blocking; a grant expiry limits revocation delay and cannot recall downloaded content.
- Automated fraud qualification for monetary rewards, auditable revenue recognition inputs and accounting review. Watch events alone must not authorize money. Current recommendations are deterministic follow/recency/trending rules, not a trained personalization model.
- High-volume analytics queue ingestion, advanced recommendation controls, notifications, comments, scheduling and publication collections. S3 archival and account-level history clearing are implemented.
- Full upload content inspection/promotion into trusted assets; quota reclamation is implemented only after S3 deletion is confirmed, never merely because a URL or lifecycle timer elapsed.
- Automated dispute lifecycle handling and full tax/accounting treatment. Refunded credit purchases and membership refunds are handled; payment disputes need an operations workflow before paid public launch.
- Automated database runtime-role provisioning with restricted DDL permissions, controlled migration execution, restoration/failover/load drills, operational dashboards and complete retention/deletion enforcement.
- Actual AWS apply, DNS/TLS/SES approval, selected CloudFront billing-plan activation, live provider/Stripe sandbox integration and end-to-end HLS playback against deployed services.
- Git history-preserving extraction and remote repository publication if required. The supplied split creates independent local source snapshots, preserving the original repository.

## Verification record

Local PostgreSQL-backed tests cover account lifecycle, concurrent reservations, webhook/checkout replay, refunds/debt, lease fencing, encoding event correlation, upload quota races, signed-cookie scope/single-use grants, provider budgets, membership lifecycle, reward rounding/overflow and payout timeout/concurrent retry safety. Frontend checks and desktop/cloud builds, Terraform schema/format validation, compose configuration and source extraction are verified separately.

The collaborative browser preview failed to navigate/snapshot reliably in this session. HTTP smoke checks through the SvelteKit gateway passed; this is not equivalent to browser interaction, visual QA or live streaming verification. Container builds and final test outcomes are recorded in the implementation plan ledger after completion.
