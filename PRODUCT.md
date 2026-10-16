# VideoFlow

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Viewers who mainly watch, discover and follow creators; creators who make videos in Studio or upload existing work, publish to a public profile and review their audience. One account supports both roles. Ordinary viewers must not be pushed into creator tools or required to buy generation credits.

## Product Purpose

Combine a visual AI video creation workspace with a video publishing and viewing platform. Users can watch standard videos and Shorts, manage their account, publish finished Studio outputs or uploaded videos on VideoFlow, and download their own finished outputs.

## Positioning

A visual canvas for arranging scenes, characters and shots, connected to creator profiles and viewing. The platform selects underlying AI models; customers buy generation credits rather than choosing a provider/model.

## Operating Context

SvelteKit/shadcn-svelte frontend and Go API, published as private videoflow-web and videoflow-api repositories under gen-ai-videoflow. Local preview is available through Tailscale. AWS deployment and live payment activation have been deferred by the user. Existing cloud integrations and local tests are not evidence of deployed production media delivery.

## Capabilities and Constraints

- Preserve the original VideoFlow Studio canvas and working creation logic. Story, Canvas and Edit share project context; Resources supports them.
- The public cloud user guide at /help and its downloadable Markdown share one content source, separate from the desktop handbook.
- Browsing, settings and Studio have distinct navigation and task contexts.
- Platform-wide Premium viewing is separate from generation credits and free following. No per-channel paid membership pricing.
- Stripe and CryptoRails are integrated and separately activation-gated. Stripe Premium can recur; CryptoRails Premium is prepaid for the offered term. Neither integration is evidence of live settlement.
- Creator rewards are intended to use a capped share of platform revenue; they remain disabled until operational/policy approval. No guaranteed payment per view.
- Application-owned authentication, MFA, passkeys and active-session controls already exist. Physical-authenticator and real delivered-email checks remain environment gates.
- User confirmed on 2026-10-15: publish both generated Studio videos and uploaded files; extend the current shadcn interface directly, without a visual mockup round.
- Owned source assets, measured upload metadata, Shorts lifecycle and uploaded-video publication have implementation and fixture-test coverage. Live storage, transcoding and playback still require configured providers.
- Video uploads enforce server-owned limits, source metadata validation and publication-rights confirmation. Deployment configuration controls availability.
- Planned business reach is global. Availability of paid services and payouts must follow actual configured provider/country support, not a blanket worldwide guarantee.

## Brand Commitments

VideoFlow name; current interface and original canvas as visual authority; shadcn login-02/signup-02 composition and consistent recovery layouts; public profile based on user-profile1 adapted to Svelte. User requested familiar YouTube-like watching and Shorts behaviour, not a replacement brand.

## Evidence on Hand

Current implementation in frontend/src/lib/cloud and internal/cloud; product-gap status in docs/PRODUCT-EXPERIENCE-GAPS.md; dated UX audit/spec in docs/ux; local fixture-based verification and CI. No production catalogue, independent audience metrics, testimonials or live earnings proof should be fabricated.

## Product Principles

1. Watching is a complete experience independent of creating.
2. Preserve work and context across navigation, errors and authentication.
3. Public visibility, Premium access and download rights are separate decisions enforced by the server.
4. Show actual processing, payment and eligibility state, never optimistic financial or publication claims.
5. Make one source usable for publication and authorised download without unnecessary re-uploading.

Brand authority: the user-approved gen-ai-videoflow/landing-page repository supplies the VideoFlow SVG logo and signal-orange/warm-ink/bone palette. Preserve the existing shadcn layouts and canvas; the approved change is logo/colour alignment, not a typography or layout replacement.

## Progressive disclosure

User direction, 2026-10-16: make workflows simple by default. Show the primary result and next action first. Move secondary metrics, technical explanations, histories and advanced controls into clearly labelled expandable details or contextual views. Do not expose every backend concept as a top-level tab or card. Keep material payment/account blockers and estimate qualifications visible when relevant; simplicity must not conceal financial terms. Load optional detail data when opened rather than fetching every hidden panel on entry. Apply this rule to future interface work.
