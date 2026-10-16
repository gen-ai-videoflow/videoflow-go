---
name: 6eek
description: Warm ink and signal orange for watching and creating video.
colors:
  signal: "#ff5b1f"
  ink: "#0b0c0b"
  ink-2: "#121312"
  ink-3: "#1b1c1a"
  bone: "#ebe9e2"
  mute: "#a3a199"
  dim: "#75736c"
  light-card: "#f5f3ed"
  light-secondary: "#dedbd2"
  light-sidebar: "#e4e1d8"
  light-muted: "#615f58"
  light-active: "#a93008"
  dark-border: "rgb(235 233 226 / 0.12)"
  dark-input: "rgb(235 233 226 / 0.24)"
  light-border: "rgb(11 12 11 / 0.16)"
  light-input: "rgb(11 12 11 / 0.24)"
  dark-accent: "rgb(255 91 31 / 0.14)"
  light-accent: "rgb(255 91 31 / 0.12)"
  destructive-light: "oklch(0.577 0.245 27.325)"
  destructive-dark: "oklch(0.704 0.191 22.216)"
typography:
  body:
    fontFamily: "Inter Variable, Noto Sans CJK SC, Noto Sans SC, PingFang SC, Microsoft YaHei, Hiragino Sans GB, Source Han Sans SC, sans-serif"
    fontSize: "0.875rem"
    lineHeight: 1.428571
  title:
    fontSize: "1.5rem"
    fontWeight: 700
    lineHeight: 1.333333
  label:
    fontSize: "0.75rem"
    fontWeight: 500
    lineHeight: 1.333333
rounded:
  sm: "0.375rem"
  md: "0.5rem"
  lg: "0.625rem"
  xl: "0.875rem"
  4xl: "1.625rem"
spacing:
  2: "0.5rem"
  3: "0.75rem"
  4: "1rem"
  6: "1.5rem"
  10: "2.5rem"
components:
  button-primary:
    backgroundColor: "{colors.signal}"
    textColor: "{colors.ink}"
    rounded: "{rounded.lg}"
    height: "2rem"
    padding: "0 0.625rem"
  input:
    rounded: "{rounded.lg}"
    height: "2rem"
    padding: "0.25rem 0.625rem"
  card:
    rounded: "{rounded.xl}"
    padding: "1rem 0"
  badge:
    rounded: "{rounded.4xl}"
    height: "1.25rem"
    padding: "0.125rem 0.5rem"
---

# Design System: 6eek

## Overview

**Creative North Star: "6eek’s existing workspace, aligned with its landing-page brand"**

6eek uses warm ink surfaces, bone text and a restrained signal-orange accent within its existing shadcn interface. The supplied logo connects watching, account management and Studio without changing their task-specific layouts.

Inter, compact controls, fine borders and muted supporting copy remain the incumbent visual language. Media and the original Studio canvas lead; brand appears in the logo, primary actions and selected states.

**Key Characteristics:**

- Warm ink and bone surfaces with signal-orange actions.
- Compact shadcn controls and readable Inter typography.
- Original Studio canvas with a quiet overview and focused project resource workspace.

## Colors

### Primary

Signal orange identifies primary actions, selected resource states and focus in dark mode. Studio’s desktop rail uses a neutral muted active surface with foreground text; other navigation retains its own semantic treatment. Primary controls use ink text. Light-mode selected text and focus use the darker light-active token rather than bright signal orange.

### Neutral

Dark mode maps background to ink, cards/popovers/sidebar to ink-2, secondary/muted surfaces to ink-3, foreground to bone and supporting text to mute. Fine bone borders separate surfaces; input borders use a stronger opacity. Dim remains a chart/supporting palette value rather than the default body-text color.

Light mode uses bone for the page, light-card for cards/popovers, light-secondary for secondary/muted surfaces and light-sidebar for navigation. Ink is foreground; light-muted is supporting text. Borders and inputs use translucent ink. Semantic destructive tokens stay separate from the brand accent.

**The Contrast Rule.** Use ink text on signal-orange fills and the theme-specific active/muted tokens for text. Measured pairs: signal/ink 6.32:1, bone/ink 16.13:1, mute/ink-2 7.20:1, light-muted/bone 5.26:1 and light-active/bone 5.55:1. These pairs do not certify every opacity, state or overlay.

Palette authority: [landing-page repository](https://github.com/gen-ai-videoflow/landing-page/tree/940188e875b7cca1193c5fb27011247230beaa64), specifically `styles/variables.css` and `data/site.ts`. The light-theme supporting colors adapt that palette for contrast. Four SVGs in `frontend/static/brand` are unchanged upstream assets; provenance is recorded in `frontend/static/brand/ORIGIN.md`.

## Typography

Use Inter Variable for Latin and the system CJK fallback stack in the frontmatter for Chinese. Do not replace the typeface to imitate the marketing site. Common interface text is 14px, compact labels 12px, and authentication headings 24px bold. Inputs use 16px text below the medium breakpoint and 14px above it. Individual surfaces retain their existing heading hierarchy.

## Layout

Preserve the existing responsive shells: watching has its own header, settings separates navigation from forms, and Studio retains its original canvas. Authentication keeps the shadcn two-column composition on large screens and a centered form on small screens. Existing spacing commonly uses 8, 12, 16 and 24px increments; authentication outer padding increases from 24px to 40px at the medium breakpoint. Do not impose a new global container width or marketing-page grid.

Watching uses a desktop rail above 650px: 232px expanded or 76px collapsed, with 44px minimum expanded rows, 48px minimum collapsed rows, 21px icons and 10px corners. Its independent `videoflow.viewer.sidebar` preference persists locally; without a saved preference, widths at or below 1000px initially collapse the rail. At 650px and below, the rail and menu toggle give way to the four-destination Discover, Shorts, Following and You bottom navigation, with safe-area padding. Header search becomes a mobile search link.

Studio uses a familiar desktop rail from 768px: 232px expanded or 76px collapsed. Expanded rows have a 44px minimum height, 21px icons and 10px corners; collapsed icon rows have a 48px minimum height. Quiet one-pixel section dividers separate groups. The menu toggle persists the preference locally under `videoflow.studio.sidebar`. Below 768px, a compact header dropdown replaces the rail.

The overview has a 1120px maximum width, a selected-project continuation area with one primary Open canvas action, recent-project rows and secondary publishing/credit links. Fine dividers and muted copy keep this surface quiet. Its two-column sections stack below 650px.

The resource workspace has a 1280px maximum width, one heading/project-selector row and one horizontally scrollable resource tab strip. Canvas remains a separate destination. Resource lists fill the content area until an item is selected or created; only then does the editor appear. Editing uses a two-column list/editor grid above 1000px, stacks below it and returns the list to one column below 600px. Header controls stack below 650px.

## Elevation & Depth

Tonal surface changes, fine borders and the card’s one-pixel foreground ring provide the main separation. Keep existing popover/dialog layering and component motion. Focus uses a three-pixel ring at half ring-color opacity. This brand alignment introduces no decorative shadows, glow or new animation system.

## Shapes

The base radius is 10px. Derived corners are 6px, 8px, 10px and 14px for the common small through extra-large sizes. Buttons and inputs use 10px corners; cards use 14px; badges use the larger 26px radius on their compact 20px height. Keep these shared component defaults and existing surface overrides.

## Components

### Buttons

Primary buttons use signal orange with ink text; default height is 32px and horizontal padding is 10px. Secondary, outline and ghost variants retain semantic theme surfaces. Keyboard focus changes the border and adds the ring; eligible pressed buttons move down one pixel. Disabled controls halve opacity and suppress interaction. Preserve the source variant behavior: the primary opacity hover rule is applied to links.

### Inputs / Fields

Inputs are full-width, 32px-high controls with input-token borders, muted placeholder text and theme-specific backgrounds. Focus uses the shared ring; invalid state uses the destructive border/ring, and disabled fields retain their subdued background and opacity. Maintain visible labels and error copy rather than communicating state through color alone.

### Cards / Containers

Cards use card/foreground semantic colors, 14px corners, a fine foreground ring and 16px vertical padding/gap. The small variant reduces spacing to 12px. Keep media clipping and existing header/content/footer padding from the shared components.

### Chips

Badges are compact 20px-high labels with 12px medium text. Primary, secondary, outline and destructive variants keep the same semantic assignments as related controls. Status labels preserve their meaning independently of the orange brand accent.

### Navigation

Watching, account and Studio navigation remain distinct. Studio’s desktop rail follows familiar YouTube navigation proportions, using foreground text and icons, a neutral `--muted` active surface with semibold text, `--accent` hover and `aria-current="page"`. Collapsed destinations retain accessible names and title hints. Quiet dividers replace visible group headings. The footer account dropdown contains generation credits, Premium viewing, payments, creator earnings, settings and sign out. The compact mobile dropdown exposes Studio destinations plus account settings and credits.

The viewing rail exposes Discover, Shorts, Following, Saved videos and Watch history, with separated Premium and Open Studio links. Current destinations use a neutral `--muted` surface, semibold foreground text and `aria-current="page"`; hover also uses `--muted`. Collapsed icons retain accessible names and title hints. Search lives in the desktop header as a labelled, 42px-high rounded form; `/search` hides that header form and retains its own search form. Both submit trimmed, encoded queries to `/search?q=…`.

Project resources use one line-style tab strip: scripts, characters, scenes, shots, storyboard, captions and assets. Active tabs use `--accent-foreground`; selected resource cards retain the primary border and accent surface. Import/export lives inside Project actions rather than the main content. Project selection and links preserve the owned project context; unsaved edits require a keep-editing/discard choice before switching resources or projects. Captions and storyboard retain their task-specific editors. Other watching/account navigation keeps its existing semantic accent states.

### Template artwork and empty Discover

Editable starter templates use image-led 16:9 covers for Explainer, Product story and Short story. The 960px WebP concept images are clipped with 12px cover corners and labelled “AI concept artwork”; they are examples of ideas, not published videos, playable previews or generation results. Preserve actual published-video thumbnails as authoritative. Exact generation prompts and asset provenance live in `frontend/static/artwork/provenance.json`.

The successfully loaded empty Discover feed offers a three-column starter section labelled “Editable templates · AI concept artwork”, stacking to one column at 650px and below. Its covers link to `/templates?starter=explainer`, `/templates?starter=product-story` and `/templates?starter=short-story`, selecting the matching template. Keep these suggestions out of populated feeds, errors, library and Following empty states. At 600px and below, the empty-state introduction uses 20px vertical padding and hides its redundant film icon; the guest action offers Explore templates. The template chooser keeps three columns above 1000px; at 1000px and below, the cards become a locally scrolling row with 260px minimum card widths and proximity snapping, while the structure/form review stacks. Mount and container resize center the selected card within that local row without scrolling the page. It retains editable script, scene and shot import behavior: project creation waits for the server identifier, starts no video generation and spends no generation credits.

### Logo

`BrandLogo.svelte` selects the supplied transparent logo on dark surfaces and supplied light variant on light surfaces. Width is 122px by default and 108px in compact headers, with intrinsic aspect ratio preserved. Both images expose the 6eek name; the supplied mark is the favicon. Do not reproduce logo lettering with a font or generate substitute artwork.

## Do's and Don'ts

### Do:

- Do use semantic CSS tokens from frontend/src/app.css in both themes.
- Do use the supplied theme-appropriate logo through BrandLogo.svelte.
- Do preserve CJK font fallbacks, visible keyboard focus and distinct error states.
- Do preserve media proportions and Studio canvas behavior while using one resource tab strip per project workspace.
- Do keep overview continuation prominent, editors conditional on selection or creation, and project transfer actions inside Project actions.

### Don't:

- Don't redraw, recolor or stretch the supplied logo.
- Don't replace Inter or the existing shadcn compositions as part of brand alignment.
- Don't use bright orange for small light-mode text when the contrast-safe active token is available.
- Don't turn status colors into brand orange or invent production media and earnings claims.
