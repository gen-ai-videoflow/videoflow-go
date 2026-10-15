# Open-source Studio editor integration

Approved intent: implement the reviewed Discover/Watch/Studio direction and reuse an available open-source browser editor. Preserve SvelteKit/shadcn, Go, application auth, generation credits, original Storyboard canvas and existing publish/download ownership checks. No AWS rollout or live payment activation.

Foundation: pin OpenCut Classic cf5e79e (full upstream pin in research). Reuse its real media processing, preview, timeline, commands, transforms, text/captions and browser export. Isolate React/editor CSS in a same-origin bundled iframe; Svelte owns application routes, account, project choice and publication. No second auth system or upstream SaaS services. Unsupported upstream catalogues must not present broken controls.

Persistence: Go stores a versioned editor document per owned project, with optimistic revision checks. Local IndexedDB/OPFS remains an explicit working cache for imported source media. Saving a timeline to cloud must not falsely claim local-only files are backed up. Editor shell identifies local-media scope; cloud-owned completed assets can be imported through existing authorized preview/download access. Cross-account browser caches use separate identifiers. No arbitrary URL fetch proxy.

Export: reuse actual upstream renderer, codec capability errors and progress/cancellation; finished output is downloadable. Publication goes through existing authenticated upload/publication flow and remains gated by storage configuration. No fake successful export, client-provided publication URLs, or generation charge for opening/editing. Existing generated-video publication stays available.

Integration steps: vendor pinned reachable editor code and notices; build isolated React bundle; wire authenticated project load/save with CAS; adapt platform header, asset imports, export handoff and Storyboard link; apply approved browsing composition to real video data; verify editor with a real media fixture, split/trim/text/undo/save/reload/export, ownership/CAS tests, desktop/mobile and generated/upload publication gates. Ship only verified capabilities and report browser/local-media limitations.
