# Cloud SaaS continuation

The existing specification and cloud/desktop boundary remain authoritative. The user authorized immediate implementation without further questions.

## Current delivery slice

1. Implement a durable storyboard → bounded clip generation → assembled video pipeline, sharing existing quotes, credit reservation, provider budgets and publication. Never retry an ambiguous paid submission automatically. Freeze all model/cost parameters in the quote. Assemble only owned successful outputs using local FFmpeg and bounded inputs.
2. Expose pipeline stages, costs and assembly selection in the existing shadcn cloud studio. Keep user model selection unavailable.
3. Add explicit production migrations and restricted runtime database roles, retaining local development convenience. Document restoration and activation steps.
4. Reconcile payment disputes from current Stripe state, deduplicate receipts, freeze affected credit spending and creator payout batches, preserve independent operator restrictions, and audit all transitions. Lost disputes remain held for financial operations review; no invented loss/refund accounting.
5. Review and run PostgreSQL-backed race tests, frontend type/build checks, Go vet, infrastructure validation and container builds. Update source snapshots and implementation status with actual results.

## Acceptance

- Existing desktop build remains functional.
- Cross-workspace pipeline inputs and outputs are rejected.
- Credits settle only after final output succeeds, and all paid stages fit a frozen admitted ceiling.
- Duplicate messages and expired leases cannot cause duplicate settlement or blind paid resubmission.
- Production processes cannot mutate the schema at startup.
- Dispute outcomes cannot unlock unrelated restrictions or allow an affected prepared payout to bypass its hold.
- Any unavailable external credential or unimplemented master-spec requirement remains explicitly recorded.

## Delivered and verified

- Durable story/scene/shot/clip pipeline and FFmpeg assembly implemented with frozen prices, bounded token/clip/duration limits, parent credit reservation and current-UTC-day paid admission. Polling and assembly do not require a new paid admission. Unfunded work waits; users may cancel a running job before its first provider admission. Admitted/ambiguous work remains fenced for reconciliation.
- Cloud studio now includes scene/shot flow navigation, ordering/timing editors, story and assembly quote confirmation, stage detail and original downloads. The desktop build remains available.
- Explicit migrations/restricted runtime provisioning, isolated ECS operations definition and recovery runbooks implemented.
- Dispute holds and late-refund transfer checks implemented. Independent review also found and fixed creator lock ordering, atomic operator failure resolution, stale pipeline recovery/stage writes and submission CAS checks.
- Full `go test -race ./...` passed with dedicated PostgreSQL/Redis and the real Docker FFmpeg integration enabled (cloud package 15.930s in the final run). `go vet ./...` passed. Svelte check: zero errors/warnings. Desktop static and cloud Node builds passed. API, worker, root web and standalone web Docker images built successfully.
- Terraform format/schema checks, compose configuration and workflow lint passed. API contract now describes 75 operations and 10 shared types. Independent backend tests passed outside the original checkout.
- Explicit migration command succeeded against the separate local application database. Updated API/web containers serve http://localhost:5173. Gateway smoke passed account verification, scoped studio CRUD/jobs, denied foreign resources, disabled-integration responses, archive remapping, zero-credit and disabled-payout states, empty public feed and session revocation. No paid provider request or money transfer ran.
- Both independent local repositories were updated. No remotes were created or pushed.

The T3 preview remained unable to navigate/snapshot; no visual/browser-interaction QA is claimed. The full remaining master-spec list is maintained in `docs/CLOUD-IMPLEMENTATION.md`, including moderation, trusted upload inspection, monetary fraud qualification, richer editing and actual AWS/payment/media deployment validation. This continuation does not claim a production launch or completion of those remaining features.
