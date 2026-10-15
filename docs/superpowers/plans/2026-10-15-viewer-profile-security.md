# Viewer profiles and security implementation plan

- [x] Add durable encrypted TOTP, recovery codes, challenge/session enforcement and tests.
- [x] Add library-verified WebAuthn registration/login, credential management and tests.
- [x] Add public/owned profile APIs, exact published-public counts and ownership/URL tests.
- [x] Adapt the official user-profile1 design, separate viewer/Studio navigation and preserve the original canvas.
- [x] Add shared auth recovery/MFA screens and native browser passkey integration/security management.
- [x] Wire restricted-role grants, API contract, secret configuration and deployment documentation without deploying AWS.
- [x] Run race/integration/frontend/build checks, bounded desktop/mobile browser verification and an independent security review.
- [x] Update local preview, synchronize independent repositories and commit verified changes.

## Verification evidence

- Full repository `go test -race ./...` passed with isolated PostgreSQL and Redis; cloud suite 43.236s. Go vet passed.
- Restricted runtime role tests exercise new security tables and profile columns. Profile HTTP tests cover authenticated ownership, public privacy and persisted follow state.
- Svelte check: zero errors/warnings. Both desktop/static and cloud/Node builds passed; local API/web Docker images built.
- Terraform format/validate and Compose validation passed. No AWS deployment or live payment activation.
- Independent security review found a profile middleware integration defect and missing following state; both fixed with routed regression tests. A personalized client refresh avoids reusing anonymous SSR follow state.
- Browser QA at 1366px and 390px: viewer signup defaults to Discover; profile changes persist publicly; original Studio entry remains; forgot-password uses auth-02 layout. Real TOTP enrollment yields ten recovery codes, password login remains unauthenticated before MFA, recovery login consumes one code. Final deployed TOTP pattern verified without DOM patching. No horizontal overflow in checked mobile views.
- Browser testing caught and fixed an interpolated TOTP input pattern; confirmation repeated against the rebuilt preview. Profile avatar alignment corrected. Design detector reported no findings on the changed component set.
- WebAuthn cryptographic verification uses a signed software authenticator fixture. Physical/platform passkey enrollment is not verified; delivered email and deployed media/load checks remain environment gates.
- API encryption key generated privately once and persisted in ignored local configuration. Key is absent from frontend, worker and committed files.

Independent API/web repositories are synchronized from this source snapshot. See `docs/ops/AUTH-SECURITY.md` for key backup/restore and rotation requirements.
