6eek now has a desktop app window powered by Electron, sharing the Svelte interface and Go backend with the web version.

Download the package matching your computer:

- macOS: DMG or ZIP, `arm64` for Apple Silicon; `x64` for Intel.
- Ubuntu/Debian Linux: DEB installer, `arm64` or `x64`. This installer configures the Chromium sandbox and AppArmor integration.
- Other Linux distributions: AppImage or tar.gz. These need standard graphical desktop libraries and a distribution policy permitting Chromium user namespaces; AppImage can also require FUSE.

The app includes its Go backend, frontend, FFmpeg, FFprobe and caption fonts. You do not need to install Node, Go, Python or FFmpeg. Open Settings and add an OpenRouter API key with credits to use AI generation; AI requests require internet access.

Projects and settings stay on your computer in the existing 6eek user configuration directory. Closing the app shuts down its engine. Avoid running the older browser launcher and Electron against the same data directory simultaneously.

macOS packages are unsigned unless signing credentials are configured, so macOS may request explicit approval before first use. Each OS/CPU package is built on a native runner and tested for app rendering, backend cleanup and bundled media processing. SHA256SUMS.txt contains download checksums.

Developers can also run the separate web frontend/backend containers using `docker compose up --build -d`, then open http://127.0.0.1:8080. Remote deployments need an authenticated HTTPS proxy. This release does not deploy a remote server.
