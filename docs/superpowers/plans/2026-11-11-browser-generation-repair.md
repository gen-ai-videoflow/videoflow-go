# Browser generation repair

Approved scope: repair the ten findings in docs/audits/2026-11-11-browser-generation-audit.md, align the existing single-user browser/Go workflow, remove duplicated/dead paths, introduce no dependencies, preserve existing data and working desktop/runtime features. No paid calls during verification.

1. Contract/readiness fixes: regression tests for ideas/card response and real chat project state; implement canonical idea response and DB-backed readiness. Keep compatibility with old model reply shape at the boundary.
2. Atomic persistence/cast: DB transaction helpers for script/scenes and scene/shot snapshots; share expansion/generation implementation, validate cast/project and persist it. Carry an effective generation brief and character notes/references through guided and manual generation. Test ownership, rollback, append/replacement and reference-conditioned payloads.
3. Paid-job safety and interrupted work: fenced durable render submission intent, ambiguous recovery (never re-POST), explicit retry state guards, startup interruption handling for background ops, bounded request replay tokens for browser mutations. Add counting-provider and restart tests.
4. Captions: save/edit versus burn contract must be explicit; implement successful re-burn before editor clears dirty, retain old output on failure, apply style/model/language at the right endpoints. Test with actual local FFmpeg where available.
5. Export: simple selected-output ordered assembly with existing FFmpeg, ownership/path checks, aspect/audio normalization and persistent downloadable output. Wire browser controls and truthful labels; do not pretend a scene clip is a deterministic shot timeline.
6. Cleanup: trace callers before removing duplicate scene logic, unused guided fields/stages or helpers; update user docs; tests/check/build/vet/binary smoke; fresh final review, one fix pass for important findings. Commit and push to main per standing user instruction, without force.

Acceptance: one-character/two-scene mocked end-to-end fixture, cast persistence, effective brief applied, truthful readiness, duplicate/lost-response/crash guards, actual captions and combined file, clear errors without false success. Real model fidelity/provider availability and deployed browser activation remain live verification gates.
