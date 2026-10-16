# Visual browsing refinement

Direction: extend the existing 6eek identity with familiar YouTube navigation and meaningful imagery. Watching retains a separate workspace from creating. A desktop rail exposes Discover, Shorts, Following, saved videos and history; search belongs in the header (the mobile search destination retains its form). The rail expands/collapses with a persisted preference; narrow desktops default to icons. Mobile keeps its existing four-destination bottom bar.

The first empty Discover feed offers three clearly labelled editable starter templates. AI concept covers identify Explainer, Product story and Short story; they are never presented as actual published content, playable previews, creator thumbnails or generation results. Each image links to the matching selected template. Templates retain their existing script/scene import behavior and credit-free creation. Actual published-video thumbnails remain authoritative.

Assets use the built-in image generator, optimized to 960px WebP. Exact prompts and generation provenance live in frontend/static/artwork/provenance.json and per-image metadata sidecars. Originals remain in the image-generator output directory. No new font, brand or backend changes.

Verify desktop expanded/collapsed, mobile bottom navigation and template covers, image loading and document overflow; search routes and query encoding; selected template deep links and authoritative project creation.

Verification used desktop 1440×900 and mobile 390×844. All three generated WebP covers decoded at 960px and document overflow was absent. The populated QA feed displayed its real synthetic test-video posters. Search round-tripped `coast & light` through the encoded URL. Product story and Short story deep links selected their corresponding structures. Creating a Short story template in the isolated QA database opened a real new project with its connected scenes on the original canvas. No production fixture content was added. The bounded correction batch shortened the mobile empty state, replaced its redundant self-link with Explore templates, and made template cards scroll within their own row below 1000px.

Finish-review correction: reveal the selected template card inside its horizontal row on mount/container resize, so a deep-linked selection is visible at mobile widths without scrolling the page.
