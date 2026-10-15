# Editor technical audit — 2026-10-16

Scope: current uncommitted OpenCut integration, technical correctness, security boundaries, persistence and deployability. Read-only source audit; no application code changed. Base HEAD: `bb5167baa89e0c3855164285499e1de79c1b2d25`. Runtime QA was being performed separately; this report does not claim browser/export/Docker or database test success. Findings describe the source hashes below, not later concurrent repairs.

## Findings

### P1 — Successful saves after revision 1 always conflict

`internal/cloud/editor.go:44`: `INSERT ... SELECT ... WHERE $3::bigint=0 ON CONFLICT ...` produces no candidate row for every nonzero expected revision. PostgreSQL therefore never enters the conflict update branch. Initial save succeeds, but the next valid save at revision 1 returns no rows and becomes `ErrConflict` at lines 45–46. `frontend/editor/src/main.tsx:3,7` then permanently blocks cloud writes for that session. This prevents ordinary continued editing from reaching the server.

Fix: implement conditional update for existing rows plus an atomic revision-zero insert, or an equivalent correctly conditioned UPSERT. Verify successful 0→1→2 saves, stale 0 and stale 1 rejection, and simultaneous writes. `internal/cloud/editor_test.go:23–28` currently tests initial success and stale initial rejection, but never a successful nonzero revision update.

### P1 — In-app navigation can discard the last edit before local persistence

`frontend/editor/src/main.tsx:10` sends exit, Storyboard and publish messages immediately. `frontend/src/lib/cloud/EditorPage.svelte:5,7` changes project or calls Svelte `goto`, removing the iframe. `frontend/editor/src/upstream/core/managers/save-manager.ts:17,59–81` defers saving for 800 ms. No adapter navigation handler flushes that timer. The child `beforeunload` guard (`components/providers/editor-provider.tsx:140–148`) protects document unload, not removal during same-document parent routing. A final edit followed immediately by Storyboard or project switching can be lost before the local write/pending marker.

Fix: add an exit/flush handshake for every parent and child navigation path; flush local persistence before iframe removal and make cloud queue state explicit. Verify edit-and-immediate-switch/reopen, including slow IndexedDB and an in-flight cloud PUT.

### P1 — Missing source files silently produce an incomplete render

`frontend/editor/src/main.tsx:5,7` transfers the serialized timeline into browser storage; it does not preserve a durable media-ID manifest or verify referenced files. Fresh-browser/cross-device opens therefore have scenes but no local sources. `services/storage/service.ts:351,393–403` omits unavailable media, and `services/renderer/scene-builder.ts:62–65` silently skips missing video/image elements. The adapter still shows ready and exposes Export (`main.tsx:7,10`). Reimporting a generated source assigns a new editor asset ID; there is no mapping back to the referenced IDs in the saved scene. The browser-local restriction is documented, but the current behavior can yield a successful-looking incomplete output.

Fix: detect all unresolved scene media IDs on load and before export, display the missing files explicitly, and block incomplete exports until relink/recovery. A documented same-browser scope is acceptable only with truthful missing-source handling; do not claim cross-device restoration.

### P2 — Clean development start does not produce the editor app

`frontend/package.json` runs Vite directly for `dev`; only `build` invokes `scripts/build-editor.mjs`. `scripts/split-repositories.sh:29,48–49` excludes dependencies, ignores generated editor assets and advertises `npm ci` then `npm run dev`. On a clean source/split checkout the iframe target `/editor-app/index.html` is absent until an explicit editor build. Production Docker does invoke the correct cloud build sequence.

Fix: provide a cloud dev preparation/watch command or document and run the explicit initial `VITE_CLOUD_MODE=true npm run build:editor` step. Verify from an extraction without generated assets.

## Boundaries checked and limitations

- Editor API routes are inside `requireUser` (`internal/cloud/server.go:93–94,129–130`). Reads/writes call `ownsStudioProject`, which checks workspace ownership (`studio.go:62–70`). Migration uses a project foreign key with cascade deletion and positive revisions. No ownership bypass was identified in these paths. Workspace ownership matches existing Studio semantics; it is not per-user project privacy.
- PostMessage checks both same origin and exact parent/iframe source (`main.tsx:8`; `EditorPage.svelte:7`). Project IDs used in new editor routes are encoded. Generated media preview requests use the authenticated media endpoint rather than trusting a caller-supplied preview URL. No new direct publication or credit-charging path was found; publish navigates to the existing upload flow.
- Browser pointers use owner/project keys and the adapter checks `/me` before opening. Physical upstream IndexedDB/OPFS stores remain origin-wide, keyed by editor project ID; this is browser-local cache partitioning, not an encryption/privacy boundary. Existing browser profiles retain source data after logout.
- The pending-revision marker preserves local edits when cloud revisions diverge. A conflict blocks writes, but reload with a divergent pending marker remains blocked. Backup downloads JSON only, and the adapter has no timeline-backup import or conflict merge/discard UI. This is a recovery usability limitation; JSON backups do not include source files.
- Docker installs the editor lockfile independently with lifecycle scripts disabled, builds into static/editor-app and packages the Svelte build. Split Docker COPY transformations cover the editor manifest. No container build or independent split build was executed in this audit.
- The research memo and README correctly distinguish MIT application source from MPL Mediabunny and LGPL SoundTouchJS. `build-editor.mjs:5–11` emits notices and serves dependency archives. Mediabunny includes src; SoundTouchJS includes original module contents in its source map, but its published archive lacks a literal src directory and its build entry refers to src/index.js. Do not represent generated package archives alone as a verified complete source/relinking compliance process. The served BUILD.md also references the wider source distribution; this audit did not verify that distribution is actually published or reachable. Exact dependency/WASM/font/model inventory and any LGPL replacement/rebuild obligations remain the release gates already stated in the research memo. This is a technical evidence assessment, not legal clearance.

## Reviewed file fingerprints

- `internal/cloud/editor.go`: `02103e56ac8b2a8a7fc8856fbeccde95aa62d50050466522521c61717c87abd0`
- `internal/cloud/editor_test.go`: `0c2d9703ab3f0bd0cbb1e0ee519ad3713dd9fc3e9e82ca47a28d8b749bc7b53b`
- `frontend/editor/src/main.tsx`: `d7ca01cb48322830f50ce9edf65d0f2ee3678c82620bdba71afe83201518c7cf`
- `frontend/src/lib/cloud/EditorPage.svelte`: `bf3ed1c6aac4813be71450cbda08af82ffd04476b9b015d72408b229362bc920`
- `frontend/scripts/build-editor.mjs`: `4ef4513be2620c24654c647de136af9a8a74f6e8b857f1d769f288961ca6b62e`
- `deploy/Dockerfile.cloud-web`: `fc55fd763d1b92b1036454182b9422e34de73a4624a0e5c3cb5666ea0d13baa4`
- `scripts/split-repositories.sh`: `c3ba6d353a9fcfbc7e4103ca88efdf0f6abea1719e15b3b2253eab2576015df3`
- `frontend/editor/package-lock.json`: `808bd79698a4241ecdcdb0d6789b7b78c0b3411b21843a4f3865524444c6eb17`

## Repair verification — 2026-10-16

The findings above preserve the initial audit. Subsequent review confirms the CAS defect has been repaired: `SaveEditorDocument` now uses a revision-zero INSERT ... ON CONFLICT DO NOTHING and a separate conditional UPDATE for nonzero revisions. The regression test includes successful revision 1→2. Database/race execution was reported by the coordinating agent; this audit subtask inspected the revised SQL/test but did not rerun its database fixture.

The navigation finding has been repaired in the adapter and host. The host cancels immediate Svelte navigation and project switching, sends a UUID-tagged flush request, and waits for the exact same-origin iframe response. The child makes its UI inert, waits for local dirty work, flushes, and awaits the cloud save queue before acknowledging. Conflicts/errors reject navigation; a 10-second host timeout retains the frame and resumes its UI. Late or mismatched responses cannot release a later navigation. Parent navigation after a successful acknowledgment resumes the old frame if it survives, avoiding a stuck same-route view. Native browser tab closure remains protected by cancellation/leave warning; JavaScript cannot defer a native close into an asynchronous route transition.

Validation performed here: editor TypeScript compiler completed with no diagnostics; cloud Svelte check completed with 0 errors and 0 warnings. Focused executable tests extracted and transpiled the actual child bridge and host flush/ack handlers: cloud queue delay prevents acknowledgment; conflicts restore editing and return failure; malformed request IDs are rejected; foreign-origin and stale request acknowledgments are ignored; timeout sends resume and retains the iframe. These focused tests do not substitute for the coordinator’s browser navigation QA. SourceSafety integration requested by the coordinator is wired into the entry point; missing-media behavior is being verified separately.

Repaired file fingerprints:
- `internal/cloud/editor.go`: `fc279937a6a04134b4b67fc61e811c388f31abaeea9b4de589d8644f53871df8`
- `internal/cloud/editor_test.go`: `a84218041dda0ca04f6912b2555b2baef6938b6611a0c525f583f5a87953b298`
- `frontend/editor/src/main.tsx`: `818ce88332cffe1c5a7bcd96eea5d8d66cc4d987376c64e417e3dc39d5fe41b6`
- `frontend/src/lib/cloud/EditorPage.svelte`: `4b8f84bae6a5923fd56b99e673df7feb7d3decbc288e58a5d2676e7aa6f25e15`

## Coordinating release verification

The repaired backend passed the full `go test -race ./...` suite against isolated PostgreSQL/Redis. Additional real FFmpeg/probe tests passed for media probing, mixed-audio assembly and validated upload/publication/download. `go vet ./...`, editor TypeScript and Svelte check passed (0 errors,0 warnings). Browser QA imported an authenticated private12-second640×360 MP4, added it to the timeline, reloaded the editor and exported a1,121,334-byte MP4. Browser metadata decoded12seconds640×360; seeking to5seconds decoded a nonblack image (99.99% nonblack pixels). An immediate text insertion followed by Storyboard navigation persisted the text and source manifest at cloud revision4 before changing routes.

Clean cloud-mode development now invokes the editor build before Vite. Both API and SSR web Docker images built successfully. Production dependency audit found0 advisories; the complete editor dependency audit retains2 moderate advisories through build-only Tailwind typography/PostCSS selector parsing. No critical/high advisories were reported. These observations do not certify external providers, global payout availability, legal compliance or every browser/codec.

Missing-source recovery was exercised in an isolated cloned QA timeline with a deliberately absent source ID. The editor displayed the original filename and browser-local storage explanation and disabled Export. Reconnecting the original1,095,690-byte MP4 removed the warning and enabled Export. No source files or documents belonging to normal preview accounts were modified. The initial malformed test fixture (mismatched serialized root/metadata IDs) was corrected before this check; it was not an application save regression.
