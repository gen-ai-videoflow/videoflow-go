# Authentication and profile follow-up

## Implemented

- Security overview reports real passkey count, authenticator status and recovery code count.
- Authenticator enrollment now shows the actual ordered steps: identity check, scan, verify, recovery codes. Recovery codes can be copied or downloaded locally; they are never sent to another service.
- Passkey enrollment explains the browser/device handoff and disables unsupported insecure/browser environments. Removal and authenticator disabling use the shared shadcn dialog with explicit consequences and cancel actions.
- Profile editor shows actual banner/avatar previews. Local JPEG/PNG/WebP selection opens a shared crop dialog with zoom and horizontal/vertical framing before upload; publishing still requires Save profile.
- Private S3-backed profile image upload validates base64, input bytes, format, pixel dimensions, re-encodes JPEG to strip metadata, enforces a per-channel recent-upload cap and records owner. Public image handler only serves images currently referenced by their owner's profile. No externally supplied image URL is fetched by the backend.
- Preset YouTube, Instagram, TikTok, LinkedIn, GitHub and website link actions include small monochrome platform marks, prevent duplicate preset rows and retain the existing eight-link limit. Public links use the same marks.
- Caption language uses a named-language selector with an unknown-current-value fallback. Motion preference uses native accessible radio buttons. Caption help explicitly states selecting a preference does not translate/generate captions.

## Verification and boundaries

- Run the shared frontend Svelte check and the new TestNormalizeProfileImage unit test before release; evidence is recorded in the parent release report.
- Existing server authentication and TOTP/passkey verification remain authoritative. No test enrolled a physical security device; no OAuth provider is represented as enabled.
- New migration: 032_profile_images.sql. Mount public GET /v1/profile-images/{id}; POST /v1/profile/images is mounted through profileRoutes. A configured private originals bucket is required; missing storage produces an actionable unavailable message.
- Upload uses the application API for bounded profile images, not the multipart video upload pipeline. It intentionally recompresses and strips metadata; framing controls preview the square/wide crop before upload.
- A bounded cleanup hook exists for old unreferenced objects; schedule it or configure an appropriate lifecycle policy before public launch. Profile image delivery currently proxies bounded objects with five-minute public cache; a CDN can serve this route later. The per-owner application cap complements mutation rate limiting; it is not a storage billing quota.
- Existing unsaved-change browser confirmation remains in older settings/profile routes; this change does not claim all app dialogs have been migrated.

## Additional implementation

- Secure email magic-link sign-in is available for existing accounts, using hashed single-use tokens, a 15-minute expiry, generic responses and independent IP/email limits. New requests invalidate older links. Email verification alone never bypasses enrolled TOTP/passkeys: it returns the existing MFA challenge. Link clicks render a confirmation button; a GET never consumes a token, reducing accidental email scanner consumption.
- Migration033 adds the magic token purpose. `magicLoginRoutes` mounts public POST issue/verify routes. `/magic-link` uses the shared authentication layout and retains the second-factor stage.
- Development profile uploads may use only the exact configured HTTP origin and generated UUID image route. Production and external profile URLs stay HTTPS-only. Saving an application-owned profile image additionally checks image ownership under the profile lock.
- Explicit maintenance hook `CleanupProfileImages(ctx)` deletes up to100 unreferenced profile images older than24hours, rechecking under the profile lock. It must be invoked by an authenticated operator/admin scheduled maintenance flow; merely adding the hook does not schedule it.
- Profile/playback unsaved-change prompts now use the shared application dialog helper.

Verification completed: targeted Go race tests `TestMagicLoginExpiryReplayAndMFA`, `TestDevelopmentProfileImageValidation`, `TestNormalizeProfileImage` passed against the isolated PostgreSQL test database. Magic link tests prove replay rejection, expiry rejection and enrolled-TOTP challenge enforcement. Earlier shared Svelte check passed0errors; parent reruns final check after concurrent edits. Impeccable mechanical detector returned no findings for the three principal changed forms.

Production email dispatch has a detached10-second timeout so SES response latency does not expose whether an account exists. It is not a durable email outbox; delivery failures are logged without tokens and require a new request. Production deployment still requires configured verified email delivery and delivered-email tests.

Email dispatch is additionally capped at eight concurrent deliveries. A production request acquires a slot before account lookup regardless of whether the email exists; each provider delivery is still capped at10seconds. This prevents request bursts from creating unlimited email goroutines.

## Google and Apple sign-in

Implemented with `github.com/coreos/go-oidc/v3` and `golang.org/x/oauth2`: discovery, signed ID token verification, issuer/audience/expiry validation, nonce binding, short-lived browser-bound single-use state, encrypted Google PKCE verifier and confidential server-side code exchange. Only providers with credentials, HTTPS origin and a valid application encryption key appear in sign-in capabilities. Apple requires a signed client-secret JWT supplied through secrets and rotated before its expiry.

Google uses S256 PKCE. Apple's documented token contract does not specify PKCE parameters; Apple uses nonce, state/browser binding and confidential client authentication. Do not describe Apple as providing PKCE enforcement. Apple `form_post` is trusted only at the exact backend callback; SvelteKit trusts the Apple origin to pass that form to the API, whose own origin/type/state/nonce rules remain authoritative.

New provider identities create an account transactionally. A matching existing email never auto-links: the user must sign in using the original method and explicitly link in Security after reauthentication. Linked identity ownership cannot move between users. Apple linking binds to the exact still-active original reauthenticated session hash because regular SameSite=Lax session cookies do not accompany Apple's cross-site POST. Suspended/revoked/expired sessions fail. Existing MFA remains mandatory after provider sign-in and social-provider claims do not stamp application MFA proof.

Official references reviewed: [Google OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect), [Apple token validation](https://developer.apple.com/documentation/signinwithapplerestapi/generate-and-validate-tokens). No live Google or Apple authorization or real credentials were used.

Deployment gates: register exact HTTPS callbacks `/api/v1/auth/oauth/google/callback` and `/api/v1/auth/oauth/apple/callback`, supply client credentials, configure Apple Services ID/domain verification and signed secret rotation, verify actual browser callback/cookie behavior with real test provider accounts, add identity unlinking with alternate-sign-in protection, and schedule expired OAuth-state cleanup. Provider-only new accounts have an undisclosed random password; use email reset to establish a password before operations currently requiring password reauthentication.

OAuth verification completed against a local mock discovery/JWKS server with RSA-signed ID tokens and an isolated PostgreSQL database. Race-enabled tests passed for valid identity verification, unsigned/wrong-audience/wrong-nonce/wrong-issuer/expired token rejection; wrong browser/state/provider, expiry and state replay rejection; existing-email collision refusal; immutable identity ownership; original link-session ownership/revocation; and transactional new provider account creation with an authenticatable session. Latest evidence: `/tmp/6eek-oauth-tests-final3.log`. Final combined frontend check remains the parent's release gate; the OAuth-focused shared check reported0errors with existing payment warnings.

Full profile image lifecycle integration test passed with `-race` using fresh real PostgreSQL and a mocked S3 HTTP transport (no AWS calls): upload returns draft URL; draft GET404; own profile save succeeds; anonymous GET200 returns decodable normalized JPEG; another user cannot attach the owned upload; cleanup deletes old orphan while retaining published media; a deleted orphan cannot be republished. A deterministic channel-lock concurrency test also passed: cleanup selected an orphan, waited while that image became published, then rechecked and retained it. Test: `TestProfileImageUploadPublishOwnershipAndCleanup`; evidence `/tmp/6eek-profile-flow-tests.log`.

## Profile progressive disclosure

The profile editor now uses clickable media surfaces: clicking the avatar opens the file picker directly; clicking the banner opens a compact shared dialog with upload, four generated presets, image-link disclosure and removal. Existing zoom/reposition cropping is preserved. Selecting a preset updates the preview and still requires Save profile. Only four exact bundled banner paths are accepted by the backend; unknown paths, traversal and query variants are rejected.

Social platforms and empty fields no longer fill the page. Add link opens one shared dialog with a platform selector, profile URL and custom-label input only when needed. Saved links appear as compact logo/name/domain rows. Each row opens Edit by clicking its label, with reorder/remove actions in the shared dropdown. Platform-domain mismatches and incomplete preset URLs are explained before adding a row; backend validation remains authoritative. Featured video, image-link fallback and avatar removal sit under More profile options.

Generated-banner provenance and optimization details live in `frontend/static/banners/README.md`. No external image-host dependency or runtime AI call is introduced by preset selection. Browser release review remains the parent's shared-preview pass.
