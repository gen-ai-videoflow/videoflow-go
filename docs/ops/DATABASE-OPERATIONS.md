# Database releases, restricted runtime roles, and recovery

Production API, worker and administrative binaries check the migration ledger at startup and perform no implicit DDL. API/worker also reject owner/elevated runtime DB roles and schema CREATE/TEMP/migration-write privileges; administrative commands retain separately authorized operator credentials. `CLOUD_DEV_MODE=true` retains local convenience migrations. `cloud-migrate` uses only PostgreSQL settings; AWS, Redis, public-origin and payment settings are not required for this command. Operator tasks need owner credentials; runtime tasks must use a different login.

## Provision a staging database and deploy a release

1. Build and review one immutable release containing API, worker, admin and `cloud-migrate`. Review all new embedded SQL and compatibility with currently running processes. Take a pre-release backup/check PITR availability. Never edit an already-applied migration.
2. From a controlled private-network operator session, supply `MIGRATION_DATABASE_URL` through Secrets Manager or a secure environment. It must use the database owner/migration login, RDS endpoint, target database, and TLS. Never export the master password to ECS API/worker services or paste credentials into shell history.
3. Explicitly execute `cloud-migrate migrate` (or `make cloud-migrate`). Its transaction and database advisory lock serialize the embedded migration set. An error rolls back the complete transaction. No down migrations are supplied.
4. Supply a generated password of at least 20 bytes in `RUNTIME_DATABASE_PASSWORD`; execute `cloud-migrate provision-role --role videoflow_runtime`. This creates or rotates a restricted login, grants DML on current application tables and usage on serial sequences, and grants read-only access to the migration ledger. It excludes the migration ledger from writes and excludes payout batch and reward-period tables from runtime DML. Audit access is INSERT-only for verified dispute webhooks; credit entries/postings, money postings and earnings are SELECT/INSERT-only in runtime. Earnings status transitions belong to the separate operator role. It rejects existing roles with elevated privileges, role memberships, or object ownership. It removes public schema CREATE and public database TEMP privileges; review this intentional database-wide hardening if sharing a database with other applications.
5. Store a URL-encoded runtime connection URL in the runtime `DATABASE_URL` secret: username `videoflow_runtime`, the matching generated password, RDS endpoint, `videoflow` database and `sslmode=require` (use verified CA/`verify-full` where configured). Keep owner and runtime secrets distinct. Do not print either URL in release logs. New tables intentionally receive no automatic default grants: rerun the provisioning command after every migration and inspect changes to its exclusions as operator-only features grow.
6. Execute `cloud-migrate check` with the runtime `DATABASE_URL`. Also verify runtime DML works and attempts to create tables, temporary tables, edit the migration ledger and edit operator tables fail. Read-only schema checking rejects missing migrations and unknown future migration names; reverting to an older binary after schema expansion requires a compatible build containing the current migration set. A successful ledger check is not a checksum proof or a compatibility test.
7. Plan/approve the API and worker rollout, then deploy them using runtime credentials. Observe health, authentication, reservation/ledger checks, outbox dispatch and redelivery. Keep billing/provider admission off until smoke tests pass.

For secret rotation, provision a second runtime role first, test it, update the runtime DSN secret and redeploy services to establish new pools, then revoke the old login and remove its grants after old tasks stop. Replacing a password on a single role can race rolling tasks retaining old pools. Never drop roles with financial objects, and never use `DROP OWNED` against production owner roles.

## Optional ECS operations task

Set `migration_database_secret_arn` and `runtime_database_password_secret_arn` to provision the **definition** of an isolated one-off ECS task. Terraform does not run it. The API image includes `cloud-migrate`; the operations definition overrides the entrypoint and defaults to read-only `check`. Its execution role can read these privileged secrets; API/worker execution roles cannot. The definition also reads the runtime DSN for `check`. Use an operations role with tightly controlled `ecs:RunTask`, `ecs:DescribeTasks`, `logs` read and `iam:PassRole` permissions. Runtime roles and ordinary frontend CI must have none of those privileged execution grants.

Launch it in the output private subnet IDs and task security group with public-IP assignment disabled. Use explicit container command overrides `['migrate']`, then `['provision-role','--role','videoflow_runtime']`, then `['check']`; wait for task STOPPED and verify exit code zero and nonsecret logs after each command. Do not proceed based only on a successful RunTask API response. Privileged task definitions must not be exposed to runtime `ecs:RunTask` permissions. Removing an ECS task definition from Terraform does not revoke an already-running task; stop it and revoke credentials when needed.

The task definition uses optional external secret ARNs and does not create passwords or store plaintext secrets in Terraform. A customer-managed secret key requires additional narrowly scoped KMS decrypt permission on the operations execution role. Manual operator CLI sessions also need private RDS connectivity; do not open the RDS security group to a public IP for convenience.

## Restore rehearsal and recovery

Recovery objectives are planning targets until measured: RPO <=15 minutes and RTO <=4 hours. Rehearse at least quarterly and before enabling paid features. A backup's existence is not a demonstrated restore.

1. Record incident/rehearsal start time, source identifier, latest restorable timestamp, selected target timestamp and the immutable release compatible with that point. Freeze new paid jobs and payouts before a production recovery. Preserve the old database and logs for financial reconciliation. Do not overwrite or delete it.
2. Restore to a **new** RDS identifier using PITR, in private database subnets, encrypted, nonpublic and using the database security group. Obtain identifiers from reviewed Terraform outputs. The following is an operator template, not an instruction to run against an unspecified account:

   ```sh
   aws rds restore-db-instance-to-point-in-time \
     --source-db-instance-identifier "$SOURCE_DB_IDENTIFIER" \
     --target-db-instance-identifier "$NEW_RESTORE_IDENTIFIER" \
     --restore-time "$APPROVED_RESTORE_TIMESTAMP" \
     --db-subnet-group-name "$PRIVATE_DB_SUBNET_GROUP" \
     --vpc-security-group-ids "$DATABASE_SECURITY_GROUP" \
     --no-publicly-accessible
   aws rds wait db-instance-available --db-instance-identifier "$NEW_RESTORE_IDENTIFIER"
   ```

3. Inspect restored encryption, subnet/security groups, backup retention, deletion protection, parameter settings and managed secret behavior; restore APIs do not guarantee every operational preference matches the source. Connect with a separate recovery secret. Test owner and runtime role authentication, then read-only `cloud-migrate check` against the intended release. Apply only reviewed forward migrations if needed. Re-provision role grants for the restored schema.
4. Reconcile the durable credit/money journals, balances/reservations/debt, webhook receipts, provider attempts, outbox records and publication metadata. A database restore can rewind acknowledgments while SQS, Stripe, OpenRouter and S3 remain at newer state. Quarantine existing queues until jobs are reconciled; do not replay provider submissions or money transfers blindly. Check ambiguous provider operations and payout transfers directly with the provider using durable references. Financial corrections must append reviewed adjustments, not rewrite history.
5. Use restored data only in a private isolated rehearsal environment, with email, paid provider calls, billing, memberships, rewards and cash transfers disabled. Production data remains sensitive; sanitize it before ordinary development access. Verify login, tenant isolation, signed-media permissions and basic reads. Rehearse secrets and endpoint cutover with recorded timings.
6. For actual recovery, prepare a reviewed Terraform/state reconciliation to represent the restored database while preserving the old instance. Do not apply a plan that replaces or deletes either database unintentionally. Update runtime DSN secrets to the approved new endpoint, redeploy API/worker pools, smoke-test and measure recovery time. Re-enable admission gradually after ledger/provider reconciliation. DNS alone does not redirect an existing database connection pool.
7. Record achieved RPO/RTO, missing transactions, reconciliation decisions and follow-up work. Keep both source and restored databases under an explicit retention decision; deleting either requires a separately reviewed operation.

## Payment disputes

Subscribe the Stripe webhook endpoint to `charge.dispute.created`, `charge.dispute.updated` and `charge.dispute.closed`, in addition to checkout, invoice, refund, subscription and connected-account events. Configure/verify the endpoint API version against the implementation. Disputes are recorded durably and current provider state determines holds. A successful dispute resolution may clear its hold; lost disputes remain held for financial operations review. This flow does not automatically refund credits, reverse a creator transfer, or execute a bank payout. Reconcile provider money movements and create audited corrections before release of a lost-dispute hold.
