# Viewer experience, profiles and account security

The user authorized MFA, passkeys, richer profiles using the official user-profile1 design, viewer/creator separation and recovery layouts matching login/signup. Existing preferences remain: Svelte/shadcn-svelte, original Studio canvas, no Cognito, no AWS deployment or live payments in this phase.

One account can watch and create. The default landing and post-login destination is Discover. Browsing, Following, public profiles and personal account/security pages use viewer navigation; project/generation/publishing/earnings tools stay in Studio. Merely registering does not require creator onboarding beyond a unique account handle.

Profiles expose truthful public counts and published public videos only. Optional avatar/banner/about/location/HTTPS links are editable by their owner. Follow and Premium controls retain server authorization; private/unlisted outputs and account email must not leak through profile metadata or aggregate counts. The official React user-profile1 block is adapted to the existing Svelte primitives rather than installing a second frontend framework.

Authenticator-app MFA uses encrypted TOTP secrets, rate-limited one-use password challenges, replay-resistant codes and hashed single-use recovery codes. Enabling/disabling security settings revokes old sessions and requires fresh credentials. Password reset never silently bypasses MFA. Passkeys use the reviewed WebAuthn library with exact RP/origin, required user verification and short-lived one-use database ceremonies. Existing passkey settings require recent verified passkey authentication to prevent password/recovery-only credential takeover.

Login, signup, forgot/reset password, email verification and MFA challenge use the same auth-02 page composition. Passkey cancellation/browser unavailability produces recoverable UI, never a fake successful login. Security settings display real server state, authenticator QR/manual key and recovery codes once; secrets and credentials never go to third-party QR services or logs.

Verification covers database ownership, challenge/recovery/TOTP replay, invalid WebAuthn origins/signatures/user verification, session issuance before second-factor completion, profile leakage, native browser form/keyboard flows and desktop/mobile composition. Hardware passkey registration is only claimed when actually exercised; fixture/library validation is labeled separately.
