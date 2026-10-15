# Authentication key operations

`AUTH_ENCRYPTION_KEY` encrypts TOTP secrets using AES-256-GCM with a fresh nonce and the user's immutable ID as associated data. Its value is one standard Base64 encoding of exactly 32 random bytes. Recovery codes are stored as hashes; WebAuthn stores public credential records rather than private passkey keys.

## Configure once and preserve

Generate a unique random key into a private file using a trusted operator environment. Do not paste the key into chat, source code, Terraform variables, browser configuration or command arguments. For example, after choosing a private destination:

```sh
umask 077
openssl rand -base64 32 > /private/operator-directory/videoflow-auth-key
```

Development uses the private, ignored `.env.cloud` file. Keep the same key when rebuilding or recreating containers. The example value is deliberately blank. Compose overrides the worker's value to blank and does not inject it into the web service. A key is not generated automatically on startup.

For AWS, store the exact Base64 string as a Secrets Manager secret value and set only its ARN in `auth_encryption_secret_arn`. ECS injects it into the API container. The generic shared `secret_arns` map rejects `AUTH_ENCRYPTION_KEY`, preventing accidental worker injection. No Terraform secret-value resource is created, so this configuration does not place the key itself in state. The deployment execution role can read the referenced secret to perform ECS injection; the application task roles receive no new Secrets Manager read permission.

A missing key disables new TOTP enrollment. An invalid configured key fails startup. Losing or replacing a valid key does not disable existing MFA or permit password-only login: existing authenticator secrets become unreadable. Valid UV passkeys remain an independent sign-in method. Treat loss of the encryption key as an authentication incident, not an invitation to remove MFA rows.

## Backup and restore

Back up the key through restricted secret-management recovery procedures separately from the database. Retain the secret version corresponding to every retained database backup and record that version in the backup inventory. Database backups alone cannot restore TOTP login. Never include plaintext keys in database exports, logs or a general project backup.

A restore drill must restore the matching database and key version into an isolated environment, verify an existing encrypted TOTP row can be decrypted and a controlled enrolled account can complete MFA, and confirm a password alone still receives only an MFA challenge. Test recovery-code single use and passkey authentication independently. Do not rotate or regenerate the key merely because the database or application was restored. Passkeys also require the original relying-party hostname and allowed origin; changing the hostname is not a transparent credential migration.

## Rotation

The current application accepts one encryption key and has no automatic dual-key decrypt/re-encrypt command. Changing the environment variable or promoting a newly generated secret version by itself is unsafe. Do not enable a Secrets Manager rotation schedule that simply replaces this value.

A deliberate rotation requires a separately reviewed offline migration:

1. Preserve a database backup and the old secret version. Generate the new key through the same private procedure.
2. Stop API instances so authentication and enrollment cannot write ciphertext during migration.
3. With a narrowly authorized private database connection, decrypt every `cloud_totp.secret_ciphertext` using the old key and that row's `user_id` as associated data. Re-encrypt each unchanged TOTP secret under the new key with a fresh random nonce, preserving the existing encoding, enabled state and replay counter. Include pending enrollment rows. Update all rows atomically; abort if any row cannot be decrypted or verified. Never print plaintext secrets or either key.
4. Verify all resulting ciphertext decrypts under the new key before committing. Deploy the new secret version together with the migrated database state, then restart API instances and verify controlled MFA and recovery flows.
5. If verification fails, restore the old ciphertext/database state and old key version together. Never mix old ciphertext with the new key. Retain the old key under restricted access until its corresponding backups have expired.

This runbook specifies the rotation requirements; an offline rotation tool and a completed rotation drill are not claimed. Normal deployments should continue using the existing persistent key.

## Recovery and factor changes

Password reset revokes sessions and pending authentication challenges while preserving enrolled MFA and passkeys. Recovery codes are single-use login factors and cannot authorize destructive factor changes. A recent UV-verified passkey plus the password can authorize TOTP removal when the authenticator is lost; otherwise a fresh TOTP plus the password can authorize TOTP changes. Existing passkey management requires recent UV passkey verification plus the password. Factor changes revoke old sessions and issue a replacement session for the current successful operation.

API-only key injection and the local tests do not establish production secret backup, a completed restore/rotation drill or live deployment. Provisioning and these operational drills remain operator responsibilities; no AWS changes are performed by this documentation.
