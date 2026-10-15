# File publication operating gate

`VIDEO_UPLOADS_ENABLED` defaults to false; Terraform exposes the same default. Do not enable it until private upload/original/playback buckets, restricted task permissions, validation workers with FFprobe/FFmpeg, queue dispatch and moderation/report handling have been checked in the target environment. This change does not apply AWS infrastructure.

Limits: 100 MiB per file, 1000 MiB combined workspace upload reservations, one active video upload and ten minutes measured duration. MP4/WebM metadata and checksum are inspected by the worker before the asset can enter the publication composer. Files are never made public directly from quarantine.

Both legacy source uploads and video uploads hold the workspace credit-account lock and count both reservation tables. Failed or cancelled uploads retain their reservation until cleanup confirms the owned objects and multipart uploads are gone. Ready original assets continue to consume storage quota.

## Interrupted multipart creation

Creation holds the upload row lock across the storage request and identifier update. Cancellation and cleanup cannot pass that lock and release the reservation while the request is running. A crashed process, cancelled request, database commit failure or ambiguous storage timeout can leave a `creating`/`aborting` record without a storage upload identifier. Cleanup lists the exact owned key and aborts any returned identifiers. An empty listing alone is not proof that a delayed creation cannot arrive: automatic cleanup retains that reservation instead of releasing it.

For these uncertain cases, inspect the record, API request timing, storage access logs and the exact key's multipart uploads. Reconcile the provider outcome; abort all returned IDs, verify they no longer accept parts, and delete the quarantine/original/poster objects. Only after positively establishing that no creation request remains in flight may an operator release that record's reservation. Do not blindly set `quota_released` for empty listings or bulk-release all expired rows. This exceptional reconciliation is deliberately manual; it is not a self-service quota reset. Configure an incomplete-multipart lifecycle rule as an additional provider cleanup safeguard, not as evidence that the application has completed reconciliation.

## Local evidence

Tests exercise signed part sizes, combined quota admission, creation/cancellation locking, uncertain-creation quota retention, real FFmpeg/FFprobe parsing, checksum rejection, owned downloads and shared publication creation using an HTTP storage protocol fixture. They do not exercise real AWS network failures, billing, production moderation, MediaConvert or CDN load. The normal local preview leaves uploads disabled; no live payment or reward flags are changed.
