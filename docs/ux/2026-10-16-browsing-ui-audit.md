# Browsing UI completion audit

Scope: ViewerShell and cloud/viewing components. This is one bounded audit and fix pass against the approved Watch & Create prototype (`videoflow-design-review/frontend/static/prototypes/watch-create/index.html`, served on 10546), the editor integration spec, PRODUCT.md and the incumbent DESIGN.md. Original canvas/editor adapters and publication backend are outside this pass. The approved composition is applied to real API content; fictional prototype media and conversation UI are not represented as shipped features.

## Implementation integrity

Pass for the scoped implementation. Watching has a separate shell, focused Watch/Shorts destinations hide the persistent rail, Discover uses real video cards and a separate portrait shelf, and media comes from actual returned thumbnail URLs. Existing shadcn controls, tokens, brand and authentication remain authoritative. The mechanical detector ran once over ViewerShell and viewing: `[]`. This does not certify accessibility or media delivery.

## Findings and fixes

| Severity | Finding and user impact | Resolution |
| --- | --- | --- |
| P1 | Focused-watch hamburger name derived from the saved Discover collapsed state, so its accessible action could contradict its actual expanded state. | Name now follows Open/Close navigation in focused viewing; `aria-expanded` follows the same state. |
| P2 | Focused overlay had no Escape or outside-click dismissal and inherited icon-only rail preference, making navigation harder to recognize and leave. | Added a backdrop, Escape dismissal with focus restored to the trigger, link dismissal and route-change reset. Focused menu always displays destination labels. Overlay is anchored below the actual header; mobile hides both the rail and backdrop. |
| P1 | Like control's accessible name was only its numeric count. | Added an explicit Like video/count label while preserving pressed state. |
| P2 | Fixed 148px compact thumbnails and two related columns squeezed titles at intermediate widths; long unbroken names could widen content. | Compact media now uses a proportional column, related videos stack below 800px, and video titles, creator names and descriptions wrap. |
| P2 | Mobile Shorts previous/next controls were 36px wide and video actions 40px high. | Both are at least 44px in the affected dimensions; format chips and shelf Explore action also receive 44px minimum height. |
| P2 | Portrait shelf and custom navigation links lacked explicit local keyboard focus treatment. | Added token-based focus outlines to shelf links, format/library links and Shorts contextual links. |
| P2 | Shorts share fallback generated a standard watch link even when clipboard sharing used the Shorts route. | Fallback now preserves the Shorts destination. |
| P2 | Populated Discover hid its H1 along with the quiet heading block, removing a useful screen-reader landmark. | Added a visually hidden H1 only in the populated quiet composition; no visible heading or clutter is added. |
| P2 | Viewport shell used legacy `h-screen`; short mobile landscape could leave too little space for the Shorts player and actions. | Shell uses dynamic viewport height; Shorts layout keeps a 320px minimum so outer content scrolling can retain controls. |

## Health score

| Dimension | Score | Evidence and limit |
| --- | --- | --- |
| Accessibility | 3/4 | Explicit controls, pressed/expanded state and keyboard dismissal; a full screen-reader and contrast audit remains outside this bounded pass. |
| Performance | 3/4 | Lazy media posters and active-Short-only playback; no measured bundle/network or long-list performance claim. |
| Responsive design | 3/4 | Fluid columns, local portrait scrolling and 44px affected controls; physical touch gestures and text enlargement remain untested. |
| Theming | 3/4 | Semantic tokens retained. Black player fields/white playback-error text intentionally describe media-stage UI; light-mode visual verification is still required before claiming full theme coverage. |
| Implementation integrity | 4/4 | Product-specific navigation, actual API media, no fabricated audience claims; detector returned no findings. |
| Total | 16/20 | Good; evidence limits below remain explicit. |

## Verification

- `npm run check`: 0 errors and 0 warnings.
- `git diff --check`: passed at the audit batch.
- Impeccable detector: one pass, empty findings.
- Approved Watch prototype inspected in the collaborative Chromium browser. Production-like QA browser initially served the prior build; its observations are not evidence of the final source batch.

## Remaining limitations

No production catalogue, payment activation, external delivery or deployment readiness is certified by this UI pass. QA fixtures are explicitly labelled synthetic. No physical phone, synthesized swipe, VoiceOver/NVDA, zoom/text enlargement, live network performance or exhaustive light-theme check was performed. Native video control behavior depends on browser codecs and authenticated delivery, requiring the separate media/deployment verification.

If further UI work is requested, use `$impeccable adapt` for physical touch/text enlargement evidence, `$impeccable harden` for screen-reader checks, then `$impeccable polish` only after those bounded checks.

## Final browser verification

The rebuilt QA app at `https://asus-dev.tail66f665.ts.net:10544` was inspected once across 1440×900 desktop and 390×844 mobile using T3's Chromium browser, with actual synthetic colour-bar fixture videos. This is emulated viewport evidence, not a physical phone or synthesized swipe test. Six captures are saved in `.impeccable/review/2026-10-16-browsing/`: Discover, Watch and Shorts, each desktop/mobile.

- Discover: three real standard-video cards and a separate portrait Shorts shelf; no document horizontal overflow. Mobile shelf scroll stays local. Format controls measured 43.997px high (44px CSS, subpixel scaling).
- Watch: focused desktop rail is closed by default; trigger announces Open navigation/expanded false. Opening announces Close navigation/expanded true and exposes all labels in a 231.994px rail. Escape removes the overlay and returns focus to the trigger. Actual QA video playback and native controls render. Mobile has one related column, four 44px action controls, bottom navigation and no document overflow.
- Shorts: actual portrait fixture playback, one active player, readable context, mobile bottom navigation and 43.997×43.997px previous/next controls. After the Next interaction settles, counter changes to 2 of 3 and URL updates to the next fixture. Both captures retain the focused composition with no persistent desktop rail. Mobile has no document overflow.
- One minor served/source mismatch remains: the currently served populated Discover has only the old H1 inside the hidden heading block; the source's added standalone `sr-only` H1 is not in the inspected bundle. Treat that semantic fix as source-verified but not deployment-verified until the latest source is rebuilt. It does not alter the visible composition. No further source edits were made in this verification round.

Scoped result: core browsing/watch tasks pass the bounded desktop/mobile check; the H1 deployment mismatch is a remaining P2 evidence limitation. This UI result is separate from editor, backend and deployment gates.
