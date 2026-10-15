# Browser editor deployment and operations

VideoFlow embeds a pinned OpenCut Classic browser editor at `/studio/editor`. The original canvas remains available as Storyboard. Discover, Watch and Shorts serve viewers; Studio groups projects, generation, editing and publishing. Premium is one platform subscription, with Premium-only video access; follows are free. Generation uses separate prepaid credits and a server-managed eligible model policy, without a customer model picker.

## Build and migrate

Use Node 24 and the committed frontend and editor lockfiles. `VITE_CLOUD_MODE=true npm --prefix frontend run build` builds both applications. The web Dockerfile does the same. `npm run dev` also builds the editor when cloud mode is enabled. Keep `/editor-app` assets, fonts, effects, notices and source archives in the web deployment. Deploy a complete atomic build: copying new hashed editor files into an old running Svelte static manifest is insufficient.

Run the cloud migrations before releasing the API; migration 029 creates owned project timeline documents. GET and PUT `/v1/projects/{project}/editor` enforce ownership. PUT supplies the last known revision; stale revisions return409. Back up PostgreSQL and exercise restoration. Redis is not the timeline source of truth.

## What is saved

Timelines sync to PostgreSQL. Video, image and audio source files remain in the importing browser's OPFS/IndexedDB. This release does not automatically back up source files or render on the server. Users must keep their original files. A different browser can load the timeline and reconnect missing sources; export is blocked until referenced sources are restored or removed. A timeline JSON backup does not contain source media. Clearing browser storage can delete local sources and unsynced edits.

Navigation flushes local and cloud saves through a validated iframe handshake. A cloud conflict pauses writes and keeps the editor open, offering a local timeline backup. Avoid editing the same project in multiple tabs. Publishing currently means export/download, then upload the finished file through the validated publication flow. Existing generation jobs can be imported through Project sources. Generate clip returns to the project's credit quotation flow; no editing action charges generation credits.

## Browser and provider requirements

Use a current Chromium browser with WebCodecs, sufficient device memory/storage and supported input codecs for editing/export. Other browsers and constrained mobile devices can have codec or resource limitations. Browser captions download a fixed transcription model. Production generation, crypto settlement, email, upload/encoding, signed streaming and payouts require configured real providers. Keep their activation flags off until staging tests and operational approval are complete; a successful local build does not activate them.

See CLOUD-DEPLOYMENT.md for AWS staging, secrets, migrations, health checks and rollback. No infrastructure apply or live payment activation is part of this release preparation.

## Licences

OpenCut Classic is MIT; included dependencies have their own licences. In particular Mediabunny is MPL-2.0 and SoundTouchJS LGPL-2.1. Preserve the served notices and source archives, corresponding project sources, lockfiles and rebuild instructions. Review downstream distribution obligations before commercial distribution; do not label the entire bundle MIT.

The release audit found zero production dependency advisories in the editor lockfile. Two moderate development dependency advisories remain through Tailwind typography/PostCSS selector parsing; builds use repository CSS, not user-submitted selectors. Recheck advisories and upstream fixes during maintenance.
