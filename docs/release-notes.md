6eek browser studio — repaired workflow preview

These are native browser-launcher ZIP packages for macOS and Linux, not Electron DMG/DEB installers. Choose darwin-arm64 for Apple Silicon, darwin-amd64 for Intel Mac, linux-amd64 for x86 Linux, or linux-arm64 for ARM Linux. Extract the entire archive and open Start 6eek; keep the launcher terminal open. The app opens at http://127.0.0.1:8000.

Each package includes the Go app, browser interface, FFmpeg, FFprobe, caption fonts, START HERE.txt and llms.txt. No Go, Node.js, Python or FFmpeg installation is needed. The standalone llms.txt and SHA256SUMS.txt are also attached to the release.

This preview contains the repaired character/reference upload and generation flow, canonical runnable chat actions, truthful readiness and model preflight, durable paid-request replay/recovery, real caption exports and ordered clip assembly. Old downloaded packages do not automatically receive these changes.

The preview is not Apple-notarized. macOS can block its first launch; use the system Open Anyway option only for a download you trust. Native Electron installers require a separate signing/release workflow. Windows native packages are not provided.

Start by testing project creation, reference upload and persistence without submitting generation. Add your OpenRouter key in Settings only when ready for a small budgeted test. Internet access and OpenRouter credits are required for AI generation and transcription. Live model/device testing is not part of the automated packaging checks. If a submission is uncertain, review saved work and provider activity before retrying.

Projects, media and your saved API key stay in the private user data folder across updates. Back it up. Do not run multiple app copies against that folder.

All four archives must pass native build/tests and the extracted-app smoke check before publication. Checks verify UI/API startup with an empty system PATH, bundled media tools, audio extraction and caption rendering. Each archive includes license texts and matching media sources/build scripts in media-sources.zip. FFmpeg is a separate process built with GPLv3 components.
