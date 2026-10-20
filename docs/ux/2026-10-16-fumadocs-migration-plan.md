# Standalone VideoFlow documentation

## Scope and architecture

Create a separate `gen-ai-videoflow/docs` source repository using the official create-fumadocs-app CLI, Next.js static-export template, local MDX and local search. Deploy independently on Vercel. Preserve VideoFlow's existing logo, restrained orange accent, light/dark themes and calm readable layout. English and Simplified Chinese use explicit locale routes; no browser-language middleware is required on static hosting.

## User experience

Readers first choose an audience or task. The main documentation tree contains Start, Watch, Create, Publish, Account and Troubleshoot groups. A separate product-reference group explains availability and the distinction between project data, generated media, local editor media and public videos. Public operator information contains sanitized configuration guidance only. Detailed internal runbooks remain in the API/admin repositories.

Use Fumadocs navigation, breadcrumbs, table of contents, search, locale switching, related-page cards and previous/next navigation. Use Steps for procedures, Tabs for genuine alternatives, Callouts for limits, Accordions for troubleshooting and code blocks/file trees for operator instructions. Provide Markdown exports, llms.txt, sitemap, metadata and a contributor guide. No artificial AI chatbot or unconfigured feedback backend.

## Implementation

1. Scaffold and inspect the official static template, then record current dependency/API versions.
2. Configure branding, statically generated locale routes, local search, metadata and static-only outputs.
3. Migrate all11 existing guide topics into task-oriented MDX pages, add English/Mandarin landing pages and supporting reference.
4. Produce a complete page inventory and feature/UX checklist; translate all public guide pages without pretending untranslated pages are localized.
5. Add build/content-link/locale checks and a Vercel configuration. Keep provider keys and private user information out of content.
6. Verify desktop/mobile navigation, search, locale links, deep links and Markdown downloads. Create/push the separate repository and provide a preview. Redirect application help only after an actual documentation URL is available.

## Acceptance

- Independent repository and static build with no paid-service dependencies.
- Both locales have matching real page coverage and navigable topic trees.
- Documentation describes existing behavior and clearly labels service activation limits.
- Search, mobile navigation, theme switching and task links work.
- No required visible framework credit; retain dependency license notices.
- Main app remains SvelteKit and its working help remains available until the new site is reachable.
