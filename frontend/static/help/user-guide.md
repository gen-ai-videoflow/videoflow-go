# VideoFlow cloud user guide

From your first account to watching, creating, editing and publishing.

This document is generated from the same content as the in-app Help guide. It covers the cloud web application, not the desktop handbook.

## Getting started

VideoFlow is a place to watch videos and create your own. Viewing and Studio use the same account, while each creative project keeps its story, designs and outputs together.

1. Browse Discover or Shorts. Sign in when you want to follow creators, save videos or create a project.

2. Create an account with email and password, then follow the verification email. Additional sign-in providers appear only when enabled by the platform.

3. Open Studio, then Projects. Create a named project for your first video.

4. Move between Story, Canvas and Edit using the project toolbar. The project switcher identifies the project you are working in.

- Your project remains private until you explicitly publish a finished video.

- Some services may be unavailable in a preview. An unavailable generation, checkout or upload action needs platform configuration; buying credits does not activate a missing service.

## Discover, Shorts and your library

Discover is for browsing videos; Shorts is the vertical viewing experience. You can move into Studio when you want to create without changing accounts.

1. Use search to find videos. Choose the available Recent, Popular, For you or Following feed options.

2. Open a video to watch it, read its description and visit the creator's channel.

3. Sign in to follow a creator, like a video or save it to your library.

4. Use Playback and Privacy settings to manage viewing preferences and history.

- Following a creator is different from paying for Premium. Premium is a platform subscription, rather than a separate paid subscription to each channel.

- For you uses bounded recommendations. Viewing-history preferences affect the use of history; explicit likes and follows remain signals of your choices.

- A video that requires Premium still requires an eligible account even if it appears in a feed.

## Projects and the shared workspace

A project is the home for your scripts, characters, scenes, shots, asset references and generated outputs. Story, Canvas and Edit are different views of that work.

1. Create a project from Projects. Give it a name that you can recognize in the project switcher.

2. Use Story to plan scenes and review generation. Use Canvas to explore the project's entities and their connections. Use Edit for the video timeline.

3. Open Resources for scripts, characters, scenes, shots, assets and generated-video captions.

4. Save forms and storyboard changes explicitly when the interface offers Save. Resolve the unsaved-change dialog before leaving.

5. Use Project actions in Resources to import or export project JSON.

- Project JSON is project data, not a backup of every media file or your browser editor's local media. Keep separate copies of source media and finished exports.

- Editing a scene's title, prompt, duration or order does not rewrite a clip already generated. A new generation is a separate paid operation.

- Changing the selected project does not merge projects or automatically import media into another editor timeline.

## Turn an idea into a video

Start in Story when you want VideoFlow to plan and generate a short sequence from an idea. Paid generation always has an explicit quote and confirmation.

1. Choose the intended project and open Story. Expand Generate or assemble video if it is collapsed.

2. Describe the subject, setting, action, dialogue and visual style. Specify any details that must remain consistent.

3. Choose the available clip count, clip duration and frame. The story workflow currently supports a total of up to 30 seconds.

4. Review the full credit cost. Check the selected project and settings, then confirm before the quote expires.

5. Follow Generation activity. You may leave while a server job runs; returning shows its saved status.

6. When scenes appear, select a scene or shot in the graph to edit its direction. Save storyboard changes.

7. Open Edit and import completed project sources to assemble the final timeline.

- The platform chooses the eligible AI model. There is no user model selector. Availability depends on the supported duration, frame and configured provider.

- Credits are reserved on confirmation and finalized according to the job outcome. Check credit activity for the authoritative reservation, charge or release.

- A failed request does not justify repeatedly pressing Confirm. Check Generation activity first; the app uses idempotent submission to protect the same attempt.

- Assemble clips is a simpler alternative for up to eight completed project outputs. It uses the displayed assembly settings; use Edit for a more flexible timeline.

## Canvas, characters and reusable prompts

Canvas lets you inspect and connect project material spatially. Resources contains the forms behind that material; both belong to the same project.

1. Select a project, then open Canvas. Select an entity to inspect or edit it using the available canvas controls.

2. Open Resources to create or refine characters, scripts, scenes, shots and asset references.

3. Use clear character descriptions and scene directions as continuity notes. Save edits before switching to another view.

4. Use the optional generation panel when you need to turn a story idea into a generation request.

5. Where Prompt Workbench is available, choose a template, fill in its variables, and apply the result to the form. Review the resulting prompt before saving or generating.

6. Choose English or Mandarin where the generation-language control is offered. Imported dialogue and manually written captions can retain their own language.

- Text continuity notes do not guarantee that the model will reproduce an exact character identity.

- An asset reference or uploaded image is not automatically an image-to-video conditioning input. Only capabilities explicitly enabled by the platform are available.

- Cloud image generation, speech, transcription, translation and reference-video conditioning are not implied by the prompt templates. Unsupported actions must remain unavailable.

## Edit video, audio, text and captions

Edit opens the built-in OpenCut browser editor for the selected project. Generated clips become timeline material through an explicit import.

1. Open Edit from the project toolbar and wait for the editor to load.

2. Use Project sources to import completed generated videos. You can also import supported local media through the editor.

3. Drag imported media from the media panel to the timeline. Arrange, trim and layer it using the editor's available controls.

4. Add or adjust text, audio and captions using the available editor tools. Existing SRT or ASS caption files can be imported where supported.

5. Use the editor's preview to inspect the result. Check framing, timing, spelling, audio and the final frame.

6. Before switching views, let the editor finish its save handshake. If saving fails or times out, keep the editor open and follow the error message.

7. Export when ready. Choose Download or Continue to publish when that destination is available.

- The editor performs media editing and export in your browser. Large media files and complex timelines depend on the device's memory and supported browser capabilities.

- Treat browser-local editor media as device-local. Do not assume it is a cross-device cloud backup; keep originals and download a finished export.

- Automatic browser transcription is not enabled as an alternative AI provider. Manual captions and supported subtitle imports remain available.

- The editor export handoff to publishing is temporary, account-bound and limited to 200 MiB. If it expires, or you reload or leave the browser session, export again or upload a downloaded copy.

## Download or publish a finished video

You can publish either a completed generated output or an existing video file. An edited timeline must be exported before it becomes a publishable video.

1. For a generated output, open My videos and select the completed output. Use the available download or publishing action.

2. For an edited project, export from Edit and choose Continue to publish. Download remains useful for keeping your own copy.

3. For an existing file, open Upload video from My content. Choose a supported file within the limits shown by the upload form.

4. Review the title, description and other required metadata. Confirm that you have the rights to publish the content.

5. Choose the offered visibility, format and access options. Premium content uses the platform's Premium entitlement.

6. Wait for upload and processing to finish. Review the resulting video and publish when ready.

7. Use My content to manage the video's current publication state.

- A saved project, a generated output, an editor timeline and a published video are related but distinct. Saving one does not automatically publish the others.

- Upload availability depends on platform configuration and account verification. If publishing is unavailable, keep a downloaded export.

- Private content is not public merely because it has an internal ID. Sharing and playback access remain subject to visibility and entitlement checks.

## Credits, Premium and payments

Credits pay for AI work. Premium pays for platform membership and eligible restricted viewing. They are different products.

1. Open Credits and billing to check available, reserved and used credits before generating.

2. Choose an offered credit pack or Premium plan. Review the price, currency and purchase type shown before checkout.

3. Select Stripe or CryptoRails when the platform offers both. Complete payment on the selected provider's checkout.

4. Return to billing and check the recorded payment status. A checkout redirect alone is not proof of settled payment.

5. Review your billing history and the available invoice or subscription-management actions.

- Stripe Premium can be recurring. Crypto Premium is prepaid for the displayed term; it is not an automatic recurring card subscription.

- Crypto payment instructions are invoice-specific. Use the asset, network, amount and expiry shown by the provider; do not reuse an old invoice address.

- Fees, expiry, refunds and disputes follow the displayed checkout terms and platform policy. Payment methods and regions can be unavailable.

- Creator rewards are a separate, capped platform program. There is no guaranteed payment per view, and connecting a payout account does not enable rewards.

## Profile, sign-in and account security

Profile settings control how your creator identity appears. Security settings control sign-in factors and recovery.

1. Open Profile settings. Click the avatar to choose an image, then frame it in the crop dialog.

2. Click the banner to upload an image, choose one of four decorative presets, use an image link or remove the current banner. Save the profile to publish the change.

3. Use Add link to choose a social platform and enter your complete HTTPS profile URL. Edit, reorder or remove existing links through their row controls.

4. Open Advanced profile options only when you need the additional fields.

5. In Security, enroll a passkey or authenticator using the setup steps shown. Keep any recovery codes somewhere secure outside the account.

6. Use Forgot password for password recovery. Email sign-in links are single-use and time-limited. Enabled security factors may still be required.

7. Sign in again with your factor when a sensitive operation requires recent authentication.

- A passkey depends on your browser, device and credential manager. A physical-device enrollment and recovery check is still recommended before relying on it as your only access method.

- Only enabled social sign-in providers are shown. Your profile's social links are public links, not account sign-in connections.

- Banner presets are bundled images. Selecting one does not call an AI service or spend credits.

## Creator analytics and earnings

Creator earnings shows measured audience activity alongside confirmed financial balances. Audience metrics and payable money should be read separately.

1. Open Creator earnings. Choose the available 7-, 28- or 90-day analytics window.

2. Review qualified views, measured watch time and the available per-video details. Read freshness and privacy notes next to the data.

3. Use My content to inspect the videos associated with the channel.

4. Review available, pending and paid balances separately from views.

5. When payout onboarding is enabled, enter your legal residence and complete the provider's identity and payment verification.

- Repeated viewing is validated and deduplicated under platform rules. Raw playback events are not all qualified views.

- Small audiences may have unique or rate metrics withheld for privacy. Watch-through and reach estimates are not universal, independently verified audience measurements.

- Global availability is an ambition, not a promise that every payment or payout method supports every country. Provider eligibility and platform activation still apply.

- Rewards require an enabled policy and capped revenue funding. Views alone do not guarantee earnings or a fixed rate.

## Troubleshooting and safe recovery

Use saved job status, billing history and explicit error messages to decide what to do next. Repeating a paid action is rarely the best first recovery step.

1. Generation pending: inspect Generation activity, verify available credits and check whether the platform reports AI availability. Refresh status rather than creating a second job.

2. Quote expired: review a new quote. A changed quote can have a different price or eligible model.

3. Upload or publication unavailable: verify your account and check the displayed service status. Download your export while the service is unavailable.

4. Editor save error: keep the editor open, retry saving and preserve local source files. Do not clear browser storage as a first response.

5. Missing editor media on another device: return to the original browser or import your saved source files. Browser-local media is not automatically synchronized.

6. Payment pending: check the provider invoice and billing history. Do not pay the same invoice repeatedly or treat a return URL as settlement.

7. Too many requests or temporary overload: wait for the offered retry interval. Avoid opening many tabs that poll the same job.

8. Still blocked: record the time, page, project or job ID and exact error message for your platform operator. Never include passwords, recovery codes, payment secrets or private API keys.

- This guide describes the cloud web application as implemented. The existing desktop handbook covers a different setup, including local provider configuration.

- Operators should use the separate deployment, AI activation, payment operations, request-capacity and admin-console documents. This user guide does not grant admin access or activate paid services.
