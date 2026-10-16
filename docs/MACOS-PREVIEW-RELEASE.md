This rebuild fixes the invalid/incomplete macOS bundle signatures in version 0.3.0. Both the Apple Silicon and Intel apps, their frameworks, and the bundled Go/FFmpeg/FFprobe executables are signed consistently with ad-hoc signatures. The final ZIPs are extracted and checked with strict recursive signature verification on their native Mac runners before publication.

**This is an ad-hoc preview, not a Developer ID-signed or Apple-notarized release.** macOS still needs your explicit approval; the package does not disable Gatekeeper. A company-managed Mac can prohibit that approval.

1. Download the DMG matching your Mac: **arm64** for Apple Silicon, **x64** for Intel.
2. Quit the older 6eek app and replace it in Applications with this new version. Your user data folder remains separate.
3. Try opening 6eek. If blocked, use **System Settings → Privacy & Security → Open Anyway** and confirm. [Apple's instructions](https://support.apple.com/102445).
4. Add your OpenRouter key in 6eek Settings.

The DMG includes a first-launch guide. Go, Node, Python and FFmpeg do not need to be installed separately. The interface, engine and media bundle are unchanged from the tested 0.3.0 release; this rebuild corrects packaging/signatures. Linux and web deployment remain available through the 0.3.0 release and repository. Default Gatekeeper acceptance still requires an Apple Developer ID certificate and notarization.
