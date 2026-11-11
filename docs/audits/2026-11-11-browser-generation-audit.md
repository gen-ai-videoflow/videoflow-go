# Browser generation audit — 2026-11-11

Scope: the original `videoflow-go` single-user browser studio, HEAD `0788324`, from character upload through downloadable video. This is separate from the SaaS API/media repositories. No application code was changed during this audit. Provider calls used local mocks; no paid generation or billing transactions ran.

## Verdict

The individual endpoints exist, but the advertised character-to-finished-video workflow is not consistently connected. The recent reference-upload and legacy chat action fixes address two real bugs; they do not establish a working end-to-end pipeline. Do not run a long paid generation to validate the remaining joins.

## Confirmed findings

### 1. P1 — Develop ideas response does not match its action card

`internal/agents/agents.go:61` defines `option_a`, `option_b`, `recommendation`; `internal/api/scenes.go:618` returns that object unchanged. `frontend/src/lib/chat/ActionCard.svelte:51` expects `options[]`, `recommended_index`, `reasoning`, and each option's `premise`/`hook`. After a successful paid text request the choices do not populate; `composedIdea()` indexes the missing options property.

Reproduced using a mocked model response through POST `/ideas/develop`: HTTP 200 contains option_a/option_b but no options array. Fix: one validated response contract, mapping narrative to premise explicitly or updating both producer and consumer together. Keep a compatibility mapper for cached replies only where needed.

### 2. P1 — Cast selection and uploaded character identity do not propagate through generation

`internal/api/scenes.go:163` ignores the submitted `character_ids`; it generates shots and does not save cast membership. Both the Scenes UI and chat invoke this endpoint. Script generation (`:635`) creates scenes without cast; its agent receives the idea and no character bible. `sceneReferences()` (`:439`) can use the first reference for each assigned character, but empty cast therefore yields no references. Guided generation (`internal/api/videos.go:211`) explicitly passes nil references; reference-sheet generation (`internal/api/characters.go:98`) also passes nil despite uploaded references.

Reproduced: create a character and scene, POST scene expansion with that character ID, GET stored scene; `character_ids_json` remains `[]`. Fix: explicitly select and persist cast, validate ownership, supply character bible/reference snapshots to scene/shot/keyframe work, and pass supported visual conditioning to the video model. Fail visibly or require acknowledgment when the chosen model cannot use references. Reference photos alone cannot promise identical faces.

### 3. P1 — Caption Save says re-burn but only invalidates the existing captioned output

`frontend/src/routes/editor/[id]/+page.svelte:208` PUTs caption segments with the progress label “Re-burning captions” and clears dirty state on success. `internal/api/render.go` handleUpdateCaptions only saves JSON, sets `CaptionedPath=nil`, and returns saved. It does not call BurnCaptions. Chat's separate `/caption` action burns existing segments but does not transcribe; its model/language/style request is not decoded by handleCaptionOutput.

Reproduced: save captions for an output with a captioned path; HTTP 200 reports saved while the captioned path becomes nil. Fix: clearly separate transcribe → edit/save → burn, or make Save await an actual successful burn. Apply selected caption style and language at the correct stage. Never mark an exported file updated before it exists.

### 4. P1 — Crash between provider acceptance and local ID persistence can duplicate paid generation

`internal/jobs/worker.go:115` resumes pending/running jobs. It POSTs whenever ProviderJobID is missing (`:158` onward), including a recovered submitting state. The provider ID is persisted only after CreateVideo returns (`:318` onward). A crash after upstream acceptance but before that write leaves an ambiguous job that startup reconciliation can submit again. The request has no proven idempotency/reconciliation guard in this path.

Confirmed by state/control-flow inspection, not a live crash experiment. Fix: persist submission intent and request snapshot first; fence recovered ambiguous submissions into reconciliation_required. Use provider idempotency only if its contract is verified. Never blindly resubmit an uncertain paid request. Add lost-response/crash tests with a counting mock before live testing.

### 5. P1 — Script/scene writes can report success after database failure

`internal/api/scenes.go:635` ignores CreateScript/CreateScene errors and returns objects as if saved. Scene expansion (`:163`) similarly ignores CreateShot errors; scene rendering (`:322`) ignores CreateRenderJob errors then enqueues. Multi-row generation is not atomic, so failure can also leave partial scenes/shots. Re-running generation appends more shots without a revision/replacement contract.

Confirmed by source inspection. Fix: atomic short database transactions for each generated snapshot, check every write, return failure instead of success, and enqueue only after a committed job. Make “regenerate” versus “append” explicit and preserve previous approved output until replacement succeeds.

### 6. P2 — Project readiness and suggested next steps contain fabricated state

`internal/api/chat.go:83` marks every scene expanded and having shots, while always marking storyboard/render false. It reports scripts=1 and has_style=true even in an empty project. Suggestions can therefore skip missing prerequisites or repeat work already completed.

Reproduced with a fresh database: chat reports scripts=1 / has_style=true. Fix: derive the snapshot from actual scripts, style, cast, shot, asset and render records; share this readiness calculation between cards and guided generation. Never use hardcoded readiness as operational evidence.

### 7. P2 — Guided controls and progress do not match work performed

The Create page sends visual style, conversation mode and extra instruction. `internal/api/videos.go:77` parses them but does not apply those fields to the generation calls. Language is applied to the script context, but shot generation and the worker subsequently use project/default language. Preflight's response lacks the frontend's expected effective_style/has_project_style fields. The progress component has a Dialogue stage that the pipeline never records. The pipeline operation succeeds when render jobs are queued, not when a final export exists; the Create page does separately poll render jobs, so this is not a claim that it always displays completed immediately.

Fix: build one immutable effective brief (cast, language, style, duration, aspect, direction) used by every stage. Return the same brief from preflight. Track actual stages; remove fictitious stages or implement them.

### 8. P2 — “Render scene” is one model clip, not deterministic shot-by-shot assembly

`internal/jobs/worker.go:183` joins the scene and shot descriptions into a single prompt; it selects a supported model duration, possibly shorter than the planned scene. It does not render each planned shot then concatenate. The generated storyboard can become first-frame conditioning only when the chosen video model supports it. Shot-specific rendering also selects a scene-level storyboard and can inherit scene-level duration selection.

Fix: name the initial behavior honestly (“generate one scene clip”) and expose actual supported duration/cost. If shot-by-shot output is required, add an explicit shot queue and deterministic assembly contract, including audio and aspect normalization. Do not promise precise camera cuts from a multi-shot text prompt.

### 9. P2 — Background operations have no durable interruption recovery or per-request replay protection

`internal/api/generation.go:18` starts goroutines and persists running operations; startup (`cmd/server/main.go:63`) reconciles render jobs, not these operations. A forced crash can leave an operation running indefinitely. `frontend/src/lib/ops.ts` polls without a terminal interruption policy and does not return a lifecycle cleanup handle. Busy flags are card-local; separate cards/tabs can issue duplicate requests.

Fix: record interrupted/reconciliation states on restart; recover only deterministic safe stages. Add request IDs and operation uniqueness around paid actions, bounded polling/status diagnostics and lifecycle cleanup. Preserve uncertainty rather than automatically repeating an AI call.

### 10. P2 — Multi-scene final export is not connected to this browser output editor

My earlier explanation incorrectly implied the original browser editor already provides a connected multi-scene editing/export path. `/editor/{outputId}` is a single-output preview/caption editor; render page links lead there. This audit found no browser API path that assembles selected scene clips into one finished movie. The repository's separate embedded-editor assets do not establish this integration.

Fix: initially provide a simple ordered “assemble selected scene outputs” action using the existing FFmpeg tools, with normalization, audio handling and downloadable final output. More advanced timeline editing can follow; do not add a new dependency just to support a basic assembly.

## Ordered repair plan

1. **Prevent misleading success and spending mistakes:** fix idea/card and caption contracts, truthful readiness, checked transactional writes, idempotent submission intents and ambiguous-job fencing. Keep disabled actions accompanied by a reason and next step.
2. **Connect character continuity:** select cast explicitly, persist it, carry a versioned character/reference snapshot through script → scene → shot → keyframe → model-compatible video request. Test both conditioning-supported and unsupported models.
3. **Unify the generation brief and progress:** the guided page and Canvas actions should call the same existing generation functions with the same effective inputs. Honor style/language/direction, use actual durations and show what is being generated. Avoid building a second pipeline.
4. **Complete the output contract:** transcription/save/burn must produce the promised artifact. Add a basic ordered multi-scene assembly/export using existing FFmpeg. Provide raw versus captioned download choices only when their files exist.
5. **Verify before paid rollout:** deterministic mocked-provider end-to-end tests for one character/two scenes, failures, restart/lost response, duplicate clicks, expired polling and partial output. Then a small explicitly budgeted live generation on the deployed server, followed by exported-file playback on Mike's browser. No assertion of character fidelity or provider availability until that live check.

## Verification and limitations

- Existing targeted race suites passed: internal/api, internal/jobs, internal/media, internal/db; internal/agents has no tests.
- Four additional isolated audit probes fail on current behavior: Develop ideas contract, discarded cast, fabricated project state and caption re-burn mismatch. They use Go's overlay mechanism and live outside application source; these failures are audit findings, not unexpected failures in the existing suite.
- Frontend chat normalization tests, Svelte checks, Go vet and server build were run. Production frontend build passed.
- No live OpenRouter requests, character-fidelity assessment, real voice generation, forced cloud/host crashes, Safari/device run, public-host security review or SaaS/R2 deployment audit. A browser-only app screenshot does not identify the deployed commit; Mike's server must be checked after updates.
- The two previously fixed defects remain fixed in the checked source. Downloaded installers and hosted servers do not update merely because GitHub main changes.

## Repair implementation

The subsequent repair branch implements the ordered plan in `docs/superpowers/plans/2026-11-11-browser-generation-repair.md`. The findings above describe the audited baseline, not a claim that they remain present after the repair. See `docs/BROWSER-WORKFLOW.md` for the resulting user flow and recovery semantics. Live model/device/cloud checks remain separate.

### Repair verification (2026-11-11)

- Canonical idea cards, actual readiness, persistent cast/reference snapshots, atomic scene/shot replacement, checked writes, durable request replay, verified provider-job reconciliation, real caption burn and ordered combined exports are implemented.
- Final independent review identified seven additional issues. Regression coverage now fences retry lineages and interrupted accepted operations, rejects stale shot/output saves, refreshes conditioning after cast changes, pads short audio without truncating visuals, and preserves click order across output galleries. Guided render jobs and their operation result are committed as one batch before workers are notified.
- Removed the duplicate expansion implementation, unused provider download client/helper, obsolete cast mutation helpers, unused agent action structure, misleading caption-card transcription controls and obsolete provider-specific Create assumptions. Existing compatibility decoding remains intentional for saved/older model responses. No application dependencies were added.
- `go test ./...`, targeted `go test -race` for API/database/jobs/media/providers/routing, `go vet ./...`, frontend `npm run check`, `npm run build`, and both Node chat-action tests passed. Svelte reported zero errors and zero warnings.
- A local browser test generated two scene clips with an existing character, language and direction through counted provider mocks; switched scene galleries while retaining selection; assembled both clips; and opened the combined output in the editor with no browser errors. Separate integration tests verify reference and first-frame payloads, real caption burn, combined-file decoding and download. FFmpeg/FFprobe were real binaries; no paid AI requests were used.
- Scope remains the original single-user studio. No claim is made of live model fidelity, current production configuration, mobile Safari, SaaS billing/infrastructure, or a comprehensive security certification. GitHub changes require rebuilding/redeploying the hosted application.

### Decisions and retained limitations

- Work stayed in the existing feature checkout, preserving the prior audit; this reduced isolation but avoided moving the user's in-progress context.
- The approved audit plan supplied design authorization; no product redesign or application dependencies were introduced. A newly discovered product requirement may still need clarification.
- Ambiguous paid submissions and interrupted operations require explicit evidence-based recovery. This favors preventing duplicate charges and can require manual provider inspection.
- One scene render remains one model-generated clip. Exact edited camera cuts require separate clips and explicit assembly; the model cannot guarantee those cuts.
- Old captioned exports remain retained files to prevent loss of user artifacts. A later retention policy is needed to limit storage growth.
- Create defaults to one scene. A price estimate is not invented where provider metadata does not supply a reliable total.
- Deferred minor review suggestions: simplify the legacy idea decoder's intermediate structure and expand compressed frontend request/polling helper formatting. Neither changes runtime behavior.
