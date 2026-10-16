# 6eek administration

The standalone SvelteKit application lives in the separate `admin` repository at `/home/jienweng/projects/videoflow-saas/admin`. It uses the same product origin, mounted at `/admin`, while the existing application handles sign-in and security enrollment. It never receives a database connection or provider credentials.

## Access and setup

1. Apply migrations 031, 034 and 037 along with the full migration sequence using the migration owner.
2. Re-run `cloud-migrate provision-role` after migration. The runtime role can read administrator grants, never create, edit, or remove them. Administrative audit is append-only with read access. `CheckRuntimeRole` rejects credentials that can change administrator grants.
3. Register the intended operator normally and complete a passkey or authenticator setup in 6eek security settings.
4. An authorized database operator grants the first owner in an audited transaction. Substitute the actual operator email and an identifying change ticket. No public API can grant administrator access:

```sql
BEGIN;
INSERT INTO cloud_admin_roles(user_id,role,granted_by)
SELECT id,'owner','operator:CHANGE-TICKET' FROM cloud_users WHERE email='operator@example.com';
INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)
SELECT 'operator:CHANGE-TICKET','grant_admin_role',id,'Bootstrap reviewed owner'
FROM cloud_users WHERE email='operator@example.com';
COMMIT;
```

Validate the email resolves to exactly one intended account before committing. `viewer` grants non-financial read access with user emails omitted. `support` can inspect user metadata and owned-content records. `moderation` can review content/reports and block, restore or resolve them; legacy `moderator` grants retain this meaning. `finance` can read/export payments and subscription records. `operations` can inspect generation, capacity, audit and settings, and operate the generation/publishing switches. `owner` combines these permissions and can suspend ordinary users or change runtime model/pricing policy. Use the same reviewed transaction pattern for grant changes and revoke existing sessions when withdrawing access.

Every admin request checks the role from PostgreSQL and requires an enrolled factor plus actual MFA proof in the current session. Reads accept proof within 24 hours, mutations within 15 minutes. Enrollment is not enough. Existing sessions from before this release must sign in again. TOTP completion and verified passkey flows stamp `cloud_sessions.mfa_verified_at`; password-only/magic-link sessions never invent factor proof.

## Running and routing

```sh
npm ci
npm run check
npm run build
CLOUD_API_INTERNAL_URL=http://api:8000/v1 CLOUD_PUBLIC_ORIGIN=https://videoflow.example ORIGIN=https://videoflow.example PORT=5175 npm start
```

Route `/admin` and `/admin/*` on the existing product origin to this service without stripping `/admin`. Keep `/login`, `/settings`, and all other application routes on the existing web service. First-party session cookies are shared through that origin. Do not deploy this application on an unrelated domain and expect the existing cookie to work.

The server-only gateway accepts only fixed admin path patterns, checks the browser's request origin on mutations, forwards the HTTP-only session cookie, and uses a 15-second deadline. `CLOUD_PUBLIC_ORIGIN` must match the API's `PUBLIC_ORIGIN`; it is not a browser-controlled value. It never proxies arbitrary paths, hosts, or redirects. Protected API routes also enforce role, recent factor proof, rate limits and database deadlines.

## Available operations

- Full-history people, videos, generation jobs, moderation reports, payments and audit lists use stable timestamp/ID keyset pagination (25 rows by default, maximum100), bound prefix search and status filters. Cursors are bound to kind, search, status and user scope. Timestamp ties are ordered by ID. No offset scan is used. Prefix-search fields have expression indexes. Viewer and moderation people records omit email and suspension details.
- Per-user video, project and generation-job drilldowns use the same scoped keyset pagination, excluding private prompts and media keys.
- Owner suspend/restore of ordinary users. Suspension revokes sessions and `Store.Authenticate` rejects new sessions while restricted. The endpoint cannot suspend the current operator or any administrator. Restoring access does not restore revoked sessions. Public content is reviewed independently.
- Moderator/owner video blocking, restoration to **draft** (never silently republished), and report resolution. All require reasons and transactional audit entries.
- Stripe credit orders, current Stripe Premium subscriptions, Stripe Premium invoice history and CryptoRails credit/Premium invoices, with no payment mutation controls.
- Role-gated CSV exports require a reason and fresh15-minute factor proof; maximum1000 matching rows are streamed in100-row batches, with three exports/minute per operator. String cells beginning with spreadsheet formula characters (including leading whitespace/control characters) are escaped. Audit records cover export start and completion/failure with row count and cap status. Narrow search/status filters for histories larger than1000 rows.
- Process request admission counters and database pool wait/connection telemetry (process-local, not latency percentile measurements).
- Thirty-day generation status/credit totals, provider reserved-budget days and order-value totals; top published videos with lifetime views/watch time and channel follower counts. Reserved budget is not settled provider cost. Order totals are not a net revenue ledger. The UI states these boundaries.
- OpenRouter task readiness inventory and effective pricing/model/budget values. Owners can edit the versioned runtime model/pricing policy with a reason; activation flags and credentials remain deployment managed. Credentials are exposed only as configured booleans. No live provider request is made when loading the panel. Unsupported adapters remain explicitly unavailable.
- Owner-controlled `generation_enabled` and `publishing_enabled` switches, consumed by request admission, plus a bounded announcement string. Settings reject unknown keys, require reasons and use revision comparison to prevent stale writes. Announcement is stored for a future public surface and explicitly labeled as not currently displayed.

## Deliberate boundaries

The first console does not grant arbitrary environment editing, expose secrets, mutate payment state, execute payouts, run model-provider jobs, alter historical ledgers or replay webhooks. Those use the existing operator workflow and reviewed deployment configuration. Lifetime lists are paginated; CSV exports deliberately cap each download at1000 rows. Changes made while paging may move updated subscription rows; refresh to see current entitlement state. Detailed provider invoice reconciliation, per-model realized costs, regional audience breakdowns and owner role management UI remain future work. Do not represent these as implemented.

## Verification

`admin` has its own CI for clean dependency installation, Svelte checks and production build. Backend integration coverage checks anonymous/user denial, factor enrollment versus actual session proof, viewer restrictions, query allowlist, every fixed read query, setting CAS/audit, protected suspension, session denial after suspension and expired mutation proof. Runtime role tests reject administrator-grant mutation and audit rewriting.

Browser QA used a dedicated database/API and genuine TOTP enrollment/confirmation. Production browser build verified overview, readiness/capacity, reason-confirmed setting mutation (revision 1 to 2), and its resulting audit row. Desktop 1280×800 and mobile 390×844 showed no horizontal body overflow. No real provider, purchase, refund or payout call was performed.

## Phase2 verification

Admin permission tests cover support, moderation/legacy moderator, finance, operations, viewer and unknown-role denial. Database tests cover timestamp ties across pages, no duplicate rows, cursor/filter binding, literal wildcard escaping, viewer email exclusion, paginated HTTP responses and CSV/audit results. All list SQL uses fixed descriptors and bound values; clients cannot choose columns, joins or sort expressions. CSV does not include private media keys, prompts or provider credentials. The UI sends explicit searches rather than issuing requests on every keystroke, cancels superseded loads and preserves active filters on refresh.
