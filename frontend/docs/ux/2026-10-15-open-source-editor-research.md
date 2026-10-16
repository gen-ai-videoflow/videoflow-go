# Browser video editor reuse for 6eek

Research date: 2026-10-15. Source inspection only: no candidate was installed, built, or exercised in a browser. “Implemented” below means a source path exists, not that its behavior passed QA. This is an engineering selection memo, not a complete dependency/legal audit.

## Recommendation

Use **OpenCut Classic as the first bounded integration spike**, with OpenReel as the second candidate. OpenCut has a permissive MIT application license, an actual timeline/project model and browser renderer, and no Remotion package in its inspected web manifest. It is an application fork, not a supported embeddable SDK. Its archived status means 6eek must own maintenance. **Neither candidate is an all-permissive dependency stack:** OpenCut ships MPL Mediabunny and LGPL SoundTouchJS; OpenReel adds FFmpeg WASM and GSAP terms. If “permissive only” includes every dependency, neither qualifies unchanged. Narrow the adopted surface and remove/replace unwanted dependencies before shipping.

Do not pick Twick or current DesignCombo/OpenVideo under an assumption that “open source” means unrestricted SaaS reuse. Their actual license files impose commercial restrictions.

## Immutable versions inspected

| Candidate | Repository and inspected commit | Commit date | Application license |
|---|---|---|---|
| OpenCut Classic | [cf5e79e919144200294fb9fed22a222592a0aeea](https://github.com/OpenCut-app/opencut-classic/tree/cf5e79e919144200294fb9fed22a222592a0aeea) | 2026-05-17 | [MIT](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/LICENSE) |
| OpenReel | [c9340465e5d37e684cc25bdbe746c4ccd45e165c](https://github.com/Augani/openreel-video/tree/c9340465e5d37e684cc25bdbe746c4ccd45e165c) | 2026-10-03 | [MIT](https://github.com/Augani/openreel-video/blob/c9340465e5d37e684cc25bdbe746c4ccd45e165c/LICENSE) |
| Twick | [3044c23e282b87c639c3f0b3fd5fb3fd972aa04c](https://github.com/ncounterspecialist/twick/tree/3044c23e282b87c639c3f0b3fd5fb3fd972aa04c) | 2026-05-07 | [Sustainable Use License](https://github.com/ncounterspecialist/twick/blob/3044c23e282b87c639c3f0b3fd5fb3fd972aa04c/LICENSE.md) |
| DesignCombo react-video-editor, now OpenVideo basic | [9a8c5296da4b258f66dfb7ad73de96be62478bca](https://github.com/designcombo/react-video-editor/tree/9a8c5296da4b258f66dfb7ad73de96be62478bca) | 2026-06-29 | [OpenVideo License](https://github.com/designcombo/react-video-editor/blob/9a8c5296da4b258f66dfb7ad73de96be62478bca/LICENSE) |

The [current OpenCut README](https://github.com/OpenCut-app/OpenCut#status) says the main repository is a ground-up rewrite, directs users to Classic today, and describes future editor API/plugin/headless features. Those promises must not be attributed to Classic. GitHub marks [Classic archived](https://github.com/OpenCut-app/opencut-classic). A recent commit or test file is evidence of activity, not a production support commitment.

## OpenCut Classic: closest starting point, own the fork

The [web manifest](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/package.json) specifies React 19, Next 16, Zustand, Mediabunny, and opencut-wasm. It is not a Svelte component. The [SceneExporter](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/services/renderer/scene-exporter.ts) renders the scene frame by frame through CanvasRenderer and feeds Mediabunny CanvasSource/AudioBufferSource into MP4 AVC/AAC or WebM VP9/Opus. It buffers output in memory; long or high-resolution projects require explicit memory testing. AAC capability fallback exists in this path, so verify the actual container/audio output rather than assuming every MP4 is AAC.

The [TProject model](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/project/types.ts) contains metadata, scenes, current scene, settings, version, and timeline view state. [Storage service](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/apps/web/src/services/storage/service.ts) and adjacent IndexedDB/OPFS adapters persist projects and media locally. Many version migrations exist. This is not automatically a portable cloud project file: 6eek must map media IDs to durable server assets and preserve project schema versions.

## OpenReel: active alternative with a broader surface

The [web manifest](https://github.com/Augani/openreel-video/blob/c9340465e5d37e684cc25bdbe746c4ccd45e165c/apps/web/package.json) uses React 19/Vite. Its [core package](https://github.com/Augani/openreel-video/blob/c9340465e5d37e684cc25bdbe746c4ccd45e165c/packages/core/package.json) exposes TypeScript source modules, while the full UI remains an application. This separation is potentially useful for an adapter but does not establish a stable public embedding API.

[ExportEngine](https://github.com/Augani/openreel-video/blob/c9340465e5d37e684cc25bdbe746c4ccd45e165c/packages/core/src/export/export-engine.ts) imports Mediabunny and defaults to WebCodecsBackend, with an encoder factory seam. [Project manager](https://github.com/Augani/openreel-video/blob/c9340465e5d37e684cc25bdbe746c4ccd45e165c/apps/web/src/services/project-manager.ts) saves `.oreel`/JSON through file picker or download, separately from local persistence. [Project serializer](https://github.com/Augani/openreel-video/blob/c9340465e5d37e684cc25bdbe746c4ccd45e165c/packages/core/src/storage/project-serializer.ts) defines a versioned wrapper (schema 1.2.0) and compatibility capabilities. These paths differ: do not assume every `.oreel` is the wrapped serializer form or includes original media bytes.

The repo contains export tests and a useful [browser validation procedure](https://github.com/Augani/openreel-video/blob/c9340465e5d37e684cc25bdbe746c4ccd45e165c/apps/web/docs/browser-export-validation.md), explicitly distinguishing actual browser outputs from fixture-only checks. This research did not run those checks. It is a broader dependency and product surface than a minimal timeline; remove unrelated AI/cloud/telemetry integrations when defining the adopted build.

## Commercial and dependency terms

- **MIT application licenses:** both permit commercial modification/distribution while requiring copyright and permission notices. They do not relicense dependencies or included fonts/models/assets.
- **Mediabunny is MPL-2.0, not MIT.** OpenCut’s [lockfile](https://github.com/OpenCut-app/opencut-classic/blob/cf5e79e919144200294fb9fed22a222592a0aeea/bun.lock) resolves 1.41.0. The published [1.41.0 package](https://registry.npmjs.org/mediabunny/1.41.0) and its tarball LICENSE were inspected; the [upstream license](https://github.com/Vanilagy/mediabunny/blob/main/LICENSE) is MPL. Commercial proprietary applications can use MPL components, with covered-source/notice obligations for distributed covered files and their changes. Browser delivery is distribution; keep required source availability and notices. Do not describe the entire bundle as permissive.
- **OpenCut SoundTouchJS:** its locked 0.3.0 [published package](https://registry.npmjs.org/soundtouchjs/0.3.0) includes LGPL-2.1 LICENSE, verified from the tarball. Review bundling/relinking and source obligations, or remove the affected audio feature/dependency from the spike.
- **OpenReel FFmpeg:** core manifests include `@ffmpeg/core`, `core-mt`, and wrapper packages. The 0.12.6 core registry metadata says MIT but its tarball has no LICENSE; that metadata is insufficient clearance for the compiled FFmpeg binary. The [maintainer FAQ](https://github.com/ffmpegwasm/ffmpeg.wasm/blob/main/apps/website/docs/faq.md) distinguishes wrapper terms from FFmpeg and external-library terms. Inspect exact core build flags, source, and codec licenses; do not infer MIT from the wrapper. Prefer removing the fallback entirely if a WebCodecs-only product satisfies supported inputs.
- **GSAP:** OpenReel uses 3.14.2; its [published metadata](https://registry.npmjs.org/gsap/3.14.2) points to the [standard no-charge license](https://gsap.com/standard-license), not MIT. Check redistribution/product restrictions against the intended editor business or replace it.
- **Remotion:** neither inspected OpenCut/OpenReel web/core manifest declares it. A React UI does not imply Remotion. If adding a Remotion-based alternative or renderer, inspect its [actual license](https://github.com/remotion-dev/remotion/blob/main/packages/core/LICENSE.md): the free tier is conditional (including company size), not unrestricted permissive SaaS licensing. This is a separate decision and cost, not required by the inspected render paths.
- **Twick:** LICENSE sections 3–4 prohibit providing software/backend functions as SaaS without a commercial agreement and rebranding as your own product. Its README gives SaaS-friendly examples that do not resolve this conflict in the license grant. Get written commercial terms before adoption. It offers React packages and browser/server render packages, but fails the least-risk unrestricted-license criterion.
- **DesignCombo/OpenVideo:** current LICENSE permits free use for individuals, nonprofit entities, and for-profit organizations up to three employees, and disallows copying/modifying to sell/rent/license a derivative. A larger company needs a company license. Do not rely on old MIT-era descriptions. The current manifest uses Next/React plus external `@openvideo/core`, `engine-pixi`, and `timeline`, whose own terms also require inspection.

This was a targeted direct-dependency review, not an SBOM clearance. Exact adopted lockfile, WASM artifacts, fonts, icon sets, pretrained models, sample media, and transitive dependencies remain release gates.

## Integration boundary for SvelteKit + Go

Recommended architecture (proposal, not an upstream supported API): run a pinned React editor application at an isolated editor route/origin and open it from 6eek. A full-page route is the simplest first spike; an iframe requires a small explicit bridge. Keep SvelteKit’s project workflow and Go’s auth, asset ownership, storage, and export job API authoritative. Do not rewrite the editor UI into Svelte.

Define a versioned bridge with `loadProject`, `saveProject`, `export`, `progress`, `completed`, and `error`. Validate message origin/schema, exchange short-lived project-scoped credentials, and supply signed asset URLs with correct CORS/range behavior. Keep a 6eek envelope `{schemaVersion, editorKind, editorCommit, editorProject, assetManifest}`. Store durable asset IDs rather than blob URLs/OPFS handles, and rehydrate local media caches on open. A JSON save alone does not make a project portable.

Use browser export for the first proof. The existing Go FFmpeg pipeline cannot consume either editor’s native JSON without a semantic adapter; effects, text measurement, compositing, keyframes, transitions, and audio mixing need fidelity mapping. Do not promise identical Go exports simply because both produce MP4. A server running the same browser renderer is another option, with separate reliability and resource work.

## Acceptance gates before committing to the fork

1. Pin source and lockfile, produce a dependency/asset notice inventory, and resolve MPL/LGPL/FFmpeg/GSAP exposure for the exact adopted build.
2. Import a real 6eek video with audio; trim/split, add text and a second clip, preview, save, close the browser, reopen from server state, and recover originals on another machine.
3. Export actual MP4 and WebM files in supported Chromium/Safari/Firefox combinations; inspect codecs, duration, frames, audio sync, cut boundaries and text via ffprobe and visual playback. Test unsupported codec, expired asset URL, missing original, cancellation, tab crash and retry.
4. Measure time and memory with the intended median and largest supported inputs. No 4K, mobile, or long-form reliability claim until measured.
5. Compare preview and export effects. Set an explicit supported feature set, document schema upgrade ownership, and budget security/maintenance work for archived Classic.

Decision: **OpenCut Classic for a narrow, maintained fork with dependency cleanup; OpenReel if its measured export/recovery behavior is materially better. No candidate is a zero-work drop-in SDK or a verified all-permissive stack.**
