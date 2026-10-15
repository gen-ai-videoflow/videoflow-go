> Current product decision: new credit and Premium checkout uses [CryptoRails](CRYPTORAILS.md). Premium is platform-wide prepaid 30-day access with explicit renewal; generation credits are separate. Historical Stripe payments remain reconcilable, with no new Stripe sales. The original VideoFlow canvas/sidebar remains the protected Studio; public browsing uses Discover and starters use Templates.

# Cloud development and AWS deployment

The cloud binaries are separate from the SQLite desktop/local server. Infrastructure is a staging foundation, not evidence of production readiness. Nothing in these scripts applies Terraform, deletes the source repository, or enables payments automatically.

## Local PostgreSQL and service emulators

```sh
cp .env.cloud.example .env.cloud
docker compose --env-file .env.cloud -f compose.cloud.yml up -d
# Optional application services; requires cmd/cloud, cmd/cloud-worker and cloud UI.
docker compose --env-file .env.cloud -f compose.cloud.yml --profile app up --build
```

Studio: http://localhost:8080; API: http://localhost:8081. Infrastructure ports bind to loopback. The frontend local build uses `http://localhost:8081/v1`; production uses same-origin `/api/v1`. Do not use local defaults on a shared host. PostgreSQL persists in a named volume; `down` preserves it. LocalStack initializes private-development S3 buckets and a generation queue with a DLQ. AWS SDK endpoint support requires `AWS_ENDPOINT_URL`; LocalStack is not MediaConvert, CloudFront, email, or a payment processor. Development cloud mode may use local adapters; verify actual adapter selection in the current binary. Blank CryptoRails credentials and disabled billing/payment switches keep new checkout unavailable; no development response constitutes a paid purchase. Stripe settings are optional for historical payment reconciliation. Do not copy production data or credentials into this stack.

## Terraform foundation

`infra/aws` requires Terraform >=1.11 and AWS provider 6.x. It provisions two public and two private subnets, one NAT gateway, private ECS tasks, an HTTPS ALB, separate API/worker/web ECR repositories and ECS services, PostgreSQL with managed admin credentials/PITR, encrypted authenticated TLS Redis, private encrypted S3 buckets, four encrypted SQS queues and DLQs, MediaConvert role, EventBridge encoding events, media and web CloudFront distributions, trusted playback public keys, logs, queue alarms, and budget notifications. The web image uses the cloud SvelteKit Node adapter. Existing desktop images remain independent.

The default topology has single-AZ database/cache and one NAT gateway. Set `multi_az=true` for database/cache failover; the single NAT gateway still remains a cross-AZ dependency. This is not a high-availability promise or a verified price. Desired workers have a configured maximum scalable capacity; no automatic scaling policy is supplied. Provider spend and admission limits must also be enforced by the application.

Before planning:

1. Use an isolated staging AWS account/role. Select a region and obtain a current AWS cost estimate, including NAT, ALB, RDS, Redis, media delivery, logs and backups.
2. Create an ACM certificate in the chosen region covering `alb_origin_domain`, `web_domain`, and `media_domain`, and a certificate in us-east-1 covering web/media domains. Validate them through your DNS provider. Generate an RSA signing key outside Terraform; pass only the public PEM to Terraform, and store the private PEM in Secrets Manager.
3. Pre-create runtime Secrets Manager entries and a separate raw Redis AUTH token (16–128 permitted characters). Terraform's ephemeral secret read feeds the write-only Redis token field. Increment `auth_token_wo_version` in a reviewed change when rotating it. Use AWS-managed Secrets Manager encryption, or extend execution-role KMS permissions for a customer-managed key. Never put secret values in tfvars, task environment, command history, or source control.
4. Supply the required secret ARN map. `DATABASE_URL` must use a dedicated least-privilege application database role, `sslmode=require`, and the RDS endpoint. `REDIS_URL` must use `rediss://`, the Redis endpoint and matching token. These URLs cannot be completed before endpoints exist: bootstrap with service desired counts zero, populate secrets using a secure administrative session, run PostgreSQL migrations, then deploy services. Do not give the runtime the managed RDS admin password.
5. Create an encrypted versioned remote Terraform state bucket with access controls and locking, then add a reviewed backend configuration. Local state is for validation only. State still contains infrastructure identifiers; secure it even though write-only credentials are excluded.

```sh
cd infra/aws
cp terraform.tfvars.example staging.tfvars
# Fill identifiers, public PEM and secret ARNs locally.
terraform init
terraform fmt -check
terraform validate
terraform plan -var-file=staging.tfvars -out=staging.tfplan
# Review the concrete plan, cost and launch gates before an operator applies it.
```

Bootstrap `desired_count=0`, then build and push all three images with a unique immutable release tag to the output ECR repositories. Build from the monorepo root with `deploy/Dockerfile.cloud`, `deploy/Dockerfile.cloud-worker`, and `deploy/Dockerfile.cloud-web`. Set the same `image_tag`, populate secrets, migrate and plan again with `desired_count=1`. Never deploy `latest` repeatedly into immutable ECR. Migrations are an explicit release step, not Terraform provisioners. Use the explicit migration/provision/check procedure in the database operations runbook.

Set DNS records: origin domain CNAME/alias to ALB; web and media aliases to their respective CloudFront distributions. CloudFront uses HTTPS to the origin domain, whose certificate must cover all three hostnames: the origin request policy forwards viewer Host along with generated CloudFront viewer-address/protocol headers. The web distribution disables caching by default and caches only immutable frontend assets. ALB routes `/api/*`, `/v1/*`, `/authorize` and `/health` to the API; other paths reach SSR. Authenticated HTML must remain uncached.

The media distribution uses OAC with a distribution-scoped S3 policy and trusts a public-key group on every default playback object. `/authorize` is an uncached, unsigned API origin behavior forwarding its one-use token query so the API can recheck permission and issue host-only cookies on the media hostname. The playback grant supplies this authorization URL; fetch it with credentials before requesting HLS. `/api/*` also routes uncached to the API. S3 direct access remains denied. Cookie scope, five-minute expiry, CORS, account authorization and publication checks are application responsibilities and require real-browser verification. The key-pair ID is the CloudFront public-key ID. Signing-key rotation requires updating public-key trust and the secret together with overlap.

MediaConvert terminal events enter the encoding queue, with EventBridge delivery failure DLQ and consumer retry DLQ. The worker must consume `ENCODING_QUEUE_URL`, handle the native EventBridge envelope idempotently, and reconcile missing completion events. Merely provisioning these resources does not implement encoding or processing. Generation, assembly and analytics queues are distinct resources; connect their consumers before enabling those flows. DLQ alarms exist without notification actions; attach the operations team's SNS/on-call destination before launch.

## Extraction

```sh
scripts/split-repositories.sh                       # dry run; no writes
scripts/split-repositories.sh --output /tmp/videoflow-split
```

The output directory must not exist and must be outside the original repository. The script copies the current working source (including uncommitted work), excludes secrets/build caches/state, initializes two local Git repositories, and leaves them uncommitted with no remotes. Frontend contents become the web repository root; Go/API/infrastructure become the backend repository. Original frontend and desktop release paths remain untouched. This is a reproducible source snapshot, **not history-preserving extraction**. For full history, perform a separately reviewed `git filter-repo` operation on disposable clones. Review every copied file before a first commit. Adapt Docker build contexts and compose after splitting: the backend snapshot intentionally retains monorepo cloud-web references for review; independent web builds use the generated root `Dockerfile` in the web repository.

## Acceptance before production

Run real AWS staging smoke tests: migration and restore rehearsal; private RDS/Redis connectivity; denied direct S3; valid/expired/out-of-scope cookies for manifests and segments; credentialed browser HLS; worker redelivery and poison messages; MediaConvert completion and reconciliation; session/tenant isolation; provider spend admission; verified webhook ledger reconciliation. Validate SSR metadata and no private response caching. Configure email, alarm destinations, operational dashboards, secret rotation, backup restore procedures and DNS. Keep billing, memberships, rewards and payouts disabled until their business and technical launch gates pass. Budget email notifications are alerts, not a hard spend cap.

Terraform schema validation and compose configuration validation do not prove an AWS deployment or container runtime works. No resources have been applied by this work.

References: [ECS secret injection](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/secrets-envvar-secrets-manager.html), [CloudFront signed cookies](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-signed-cookies.html), [CloudFront trusted key groups](https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/private-content-trusted-signers.html).

The uploads bucket is an unversioned, private quarantine with one-day expiry and browser POST CORS restricted to the web origin. Presigned upload policies must cap bytes and bind a workspace key; validate uploaded objects before promotion to versioned originals. API IAM can put only quarantine objects and read originals for workspace-authorized presigned export downloads; worker IAM handles validation/promotion and deletion. Do not serve quarantine keys as published media.

The ALB accepts HTTPS only from the AWS-managed CloudFront origin-facing prefix list and the ECS tasks security group. `TRUSTED_PROXY_CIDRS` is the VPC CIDR; the API may trust CloudFront viewer-address headers only when the immediate peer belongs to that configured network. Test spoofed forwarding headers and direct-origin denial in staging. Because CloudFront forwards viewer Host, the regional ALB certificate must cover origin, web and media names; the edge us-east-1 certificate covers web and media names. Custom media DNS is required, which avoids reliance on third-party cookies from a `cloudfront.net` hostname.

`OPENROUTER_API_KEY` is another required runtime secret ARN. `openrouter_video_model` selects an explicitly approved model; both provider budget variables default to zero and block spend until approved configuration is supplied. Memberships, Connect and rewards remain disabled by default. SES IAM is optional and scoped to the configured verified identity and From address; request SES production access and verify the binary's SESv2 delivery adapter before enabling external account email.

## Current commands and activation settings

Run `go run ./cmd/cloud` for the PostgreSQL HTTP API, and `go run ./cmd/cloud-worker` for outbox dispatch, generation, encoding and encoding-event consumption. Production startup only checks migration names; local `CLOUD_DEV_MODE=true` may migrate automatically. Run `go run ./cmd/cloud-migrate migrate` explicitly with `MIGRATION_DATABASE_URL`, then `provision-role` with a separate runtime password and `check` using the restricted runtime URL. See [database operations and restore runbook](ops/DATABASE-OPERATIONS.md). Terraform never executes migrations. For operator tasks, `go run ./cmd/cloud-admin` supports `report-list`, `block-video`, `resolve-job`, `reward-close`, `payout-prepare` and `payout-execute`; each command requires an explicit audited `--actor` or `AWS_ROLE_ARN`. A production operator must use restricted credentials and review the command's flags and inputs first.

Account verification/recovery mail uses AWS SESv2 with `MAIL_FROM` and `AWS_REGION`; `MAIL_ENDPOINT` is not a delivery adapter. Production requires a verified SES identity and permission to email intended recipients. No mail credentials belong in the frontend.

Runtime optional switches are `MEMBERSHIPS_ENABLED`, `CONNECT_ENABLED`, `SUPPORTED_PAYOUT_COUNTRIES`, `REWARDS_ENABLED`, `REWARD_BPS`, `REWARD_MONTHLY_CAP_CENTS`, and `CASH_PAYOUTS_ENABLED`. All monetization switches default off. Supplying `MEMBERSHIP_COMMISSION_BPS` explicitly establishes the commission policy; zero is a valid explicit policy and differs from an unset policy. Reward close consumes approved external contribution/weight inputs and does not independently qualify watch traffic. Keep reward and payout activation blocked until fraud review, financial reconciliation and commercial terms are approved. Real cash transfer requires both `CASH_PAYOUTS_ENABLED=true` and the administrative execute flag; a provider transfer into a connected account is not evidence that a bank payout settled.

The cloud CI workflow runs PostgreSQL-backed Go race tests, legacy tests, frontend checks and both frontend adapter builds, local Docker image builds, and Terraform formatting/schema validation. It performs no deployment. Extracted snapshots receive the same cloud workflow; file detection skips absent backend/frontend/infra jobs, and standalone web builds and its adapted Dockerfile run from the repository root. Actual extraction is tested only while a monorepo `frontend/` source exists.

## CloudFront billing activation

Terraform currently creates standard CloudFront distributions. It does **not** enroll them in the $15 Pro or $200 Business flat-rate plans used in the illustrative cost comparison. Before serving production traffic, explicitly check plan compatibility and subscribe each eligible distribution through the CloudFront console or PricingPlanManager API. Paid plan activation requires approval and begins billing; no subscription or approval is performed by this repository. Without that step, budget using ordinary CloudFront usage rates. See the [AWS plan management documentation](https://docs.aws.amazon.com/PricingPlanManager/latest/UserGuide/plan-management.html).

## Safe local source split

The split script creates cloud-default package scripts and a standalone Dockerfile in `videoflow-web`. Its sibling `videoflow-api` compose file points to `../videoflow-web` for the SSR image. Set remotes and publish only after reviewing the snapshots. Shared API wire types are generated by `make cloud-contract` in the source tree; in a standalone backend, the generated TypeScript output is written under `api/` for a release artifact instead of assuming frontend filesystem access.

Story/assembly deployment settings are `OPENROUTER_STORY_MODEL`, `STORY_CREDITS`, `STORY_MAX_COST_MICROS`, `ASSEMBLY_CREDITS`, and `FFMPEG_PATH`. Model/price/cost policy remains operator controlled; blank model and zero prices/cost ceilings keep paid story/assembly work disabled by default. FFmpeg defaults to `ffmpeg` in the worker image. Compose receives these values from `.env.cloud`, and Terraform exposes matching variables.
