# Studio navigation and dashboard refinement

User evidence: the October 15 screenshot of Project details showed duplicate navigation, prominent JSON transfer actions, several full-width explanatory bars and a white empty editor in dark mode. The user also reported Characters/Scenes/Assets appeared unclickable. Browser reproduction confirmed /characters changed the URL but rendered the same OriginalStudio canvas, with no Characters heading. This report supersedes earlier interface approval for the affected workspace.

## Direction contract

THESIS: one task, one resource workspace; the canvas remains its own destination.
OWN-WORLD: preserve supplied VideoFlow SVGs, signal orange, warm ink/bone tokens, Inter and shadcn controls.
STORY: creators enter an overview, continue a selected project, open canvas or a real resource editor, save and return without losing project context.
FIRST VIEWPORT: quiet overview with one continue action; workspace has one heading/project selector, one resource tab row and one content area. No empty editor alongside an empty list. Import/export belongs in Project actions. Desktop navigation follows familiar YouTube conventions: a menu toggle, 232px expanded rail, 76px icon rail, 44px rows, rounded neutral active state and quiet section dividers. Account/billing uses a footer menu. The collapse preference persists locally; mobile uses a compact menu.
FORM: incumbent application extension; no replacement visual world or composition seed. Signature behaviour: every route/tab opens the requested editor and preserves the owned selected project.
FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

## Verification

Route/project-selection regression assertions first failed against the previous helper, then passed after implementation. Browser checks must exercise all seven resource destinations, actual character creation/persistence, project selection, canvas links and responsive empty/editing states. No backend or generation/payment policy changes are part of this refinement.

Browser verification used an isolated synthetic account/database. All seven tabs opened their named section with the same selected project; an actual saved character survived reload and appeared on the canvas. Dashboard Open canvas reached the selected project. Desktop collapse changed the measured rail from 232px to 76px and persisted across full navigation. Mobile had no document overflow; resource tabs scroll locally. The bounded visual pass corrected the account-trigger styling across the component boundary and the mobile editing-list grid specificity. No production data was changed for QA.
