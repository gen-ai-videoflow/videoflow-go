# 6eek

6eek is a cloud video creation and viewing platform: create AI-assisted videos, edit and publish them, discover videos and Shorts, and access Premium content. Production domains are `6eek.com`, `api.6eek.com`, `admin.6eek.com`, and `docs.6eek.com`. Provider credentials stay on the server; SaaS viewers and creators use their account and platform credits.

The current split applications are maintained in the companion web, API, admin and documentation repositories. This aggregate workspace retains the original desktop tooling as well. See [the creator-economy implementation audit](docs/CREATOR-REWARDS-ADMIN-GAP-AUDIT.md) for implemented controls and the remaining activation gates. Runtime rewards and cash transfers stay disabled until funded rollout approval.


For the SaaS web app, use the [cloud user guide](docs/CLOUD-USER-GUIDE.md), or open **Help & guide** in the application. It covers watching, projects, Story, Canvas, editing, publishing, billing and account security. The download instructions below describe the separate desktop application.

## Download and use

For a beginner, choose the Electron app from the [GitHub downloads](https://github.com/JienWeng/videoflow-go/releases): install the macOS DMG or the Ubuntu/Debian DEB for your processor, open **6eek**, and add your OpenRouter key in Settings. The desktop app includes its backend, frontend, FFmpeg, FFprobe and caption fonts. No Go, Node, Python or FFmpeg installation is required. AI generation needs internet access and OpenRouter credits. See [desktop beginner instructions](docs/DESKTOP-QUICKSTART.txt).

Use the [rebuilt macOS 0.3.1 preview](https://github.com/JienWeng/videoflow-go/releases/tag/v0.3.1-macos-preview.1) instead of the affected 0.3.0 Mac packages. Its ad-hoc signatures pass strict integrity checks, but it is not Apple-notarized and requires explicit approval in System Settings → Privacy & Security → Open Anyway. Linux AppImage and tar.gz alternatives are available for distributions permitting Chromium user namespaces; AppImage can also need FUSE.

The earlier browser launcher remains available: extract its native ZIP and open **Start 6eek**. See [browser launcher instructions](docs/QUICKSTART.txt). Developers can run the separate web frontend and backend using `docker compose up --build -d`; see [desktop and web build instructions](docs/ELECTRON-AND-WEB.md).

To build a download, build the frontend, run `python3 scripts/build_media.py` on the target platform, then run `python3 scripts/package.py --target <platform-architecture>`. Media build tools are documented in the builder; users do not need them. A tagged release runs checks and publishes four macOS/Linux archives with checksums. Start a local combined server with `bin/6eek-server --desktop --ui-dir frontend/build`; add `--no-browser` for a service. The server binds to localhost by default; `--host` explicitly sets another interface.

# 6eek Go

A streamlined, high-performance, single-binary AI video studio backend built in **Go (Golang)** with an interactive **SvelteKit** frontend.

Forked and optimized from [videoflow](https://github.com/JienWeng/videoflow) to deliver **30x lower memory consumption**, instant database access with SQLite WAL mode, and seamless zero-dependency deployment.

---

## Performance Highlights

* **Single 17 MB Executable**: Contains the entire REST API, SSE streaming broker, background worker pool, media runner, and SQLite database engine.
* **~12.9 MB Idle RAM**: Over 30x less memory than the Python/FastAPI runtime (~400MB–1.2GB).
* **Instant Cold Boot**: Starts in `< 50 ms`.
* **Zero Dependency Setup**: No Python, no UV, no virtualenvs, no C++ compilation required.

---

## Architecture

```text
┌────────────────────────────────────────────────────────┐
│           Single Go Binary (6eek-server)          │
│                                                        │
│  [ REST API Router ]      [ SSE Event Broker ]         │
│         │                          │                   │
│         ▼                          ▼                   │
│  [ Embedded SQLite ]      [ Background Worker Pool ]   │
│  (WAL mode, in-process)   (Goroutines, no Redis/Celery)│
│         │                          │                   │
│         ▼                          ▼                   │
│  [ Static Storage Server] [ FFmpeg Process Runner ]    │
└────────────────────────────────────────────────────────┘
                           ▲
                           │ HTTP / SSE
                           ▼
              [ SvelteKit Frontend UI ]
```

---

## Quick Start

### 1. Build and Run the Go Backend

```bash
make build
./bin/6eek-server
```
The server will start listening on `http://127.0.0.1:8000`.

### 2. Run the SvelteKit Frontend

In a second terminal:

```bash
cd frontend
npm install
npm run dev -- --host 127.0.0.1
```

Open [http://localhost:5173](http://localhost:5173) in your browser.

---

## Cross-Compilation

To produce standalone executables for other operating systems:

```bash
# Build for all platforms (Linux amd64/arm64, macOS Apple Silicon/Intel, Windows)
make build-all
```

Outputs will be saved in `bin/`:
* `bin/6eek-server-linux-amd64`
* `bin/6eek-server-darwin-arm64` (macOS Apple Silicon)
* `bin/6eek-server-darwin-amd64` (macOS Intel)
* `bin/6eek-server-windows-amd64.exe` (Windows)

---

## Configuration

Set environment variables in `.env` or in your shell:

| Variable | Default | Description |
| :--- | :--- | :--- |
| `PORT` | `8000` | HTTP API listen port |
| `STORAGE_ROOT` | `storage` | Path to store generated videos and images |
| `DATABASE_PATH` | `db.sqlite` | SQLite database file |
| `OPENROUTER_API_KEY` | `""` | OpenRouter key for all AI generation and analysis |
| `OPENROUTER_TEXT_MODEL` | `openai/gpt-4o-mini` | Default text agent model |
| `OPENROUTER_VISION_MODEL` | `qwen/qwen3-vl-30b-a3b-instruct` | Default vision model |
| `OPENROUTER_IMAGE_MODEL` | `openai/gpt-image-2` | Default image/reference model |
| `OPENROUTER_VIDEO_MODEL` | `google/veo-3.1-lite` | Default video model |

---

All AI work uses OpenRouter. Save the key in Settings or set `OPENROUTER_API_KEY`; saved settings take effect without restarting. See [OpenRouter setup](handbook/03-providers.md) for model catalogs, paid checks, and FFmpeg requirements.

## User Handbook

Explore the complete guide to using 6eek Go in the [`handbook/`](handbook/README.md) directory:

| Part | Description |
|---|---|
| [1. How 6eek works](handbook/01-how-it-works.md) | Core concepts, workflow philosophy, and automated steps |
| [2. Install from zero](handbook/02-installation.md) | Prerequisites, launch, health checks, and shutdown |
| [3. Configure providers](handbook/03-providers.md) | API keys, model setup, agents, and media engine selection |
| [4. Projects](handbook/04-projects.md) | Creating, switching, exporting, importing, and backing up projects |
| [5. First video](handbook/05-first-video.md) | Step-by-step workflow starting from an initial text concept |
| [6. Characters and assets](handbook/06-characters-assets.md) | Setting up character identities, reference images, and styles |
| [7. Scenes and shots](handbook/07-scenes-shots.md) | Writing scripts, building scenes, and managing shot breakdowns |
| [8. Studio](handbook/08-studio.md) | Working with the interactive canvas and chat assistant |
| [9. Renders and outputs](handbook/09-renders.md) | Handling render jobs, quality checks, retries, and downloads |
| [10. Captions and editor](handbook/10-captions-editor.md) | Speech transcription, timing adjustments, and burnt-in captions |
| [11. Troubleshooting](handbook/11-troubleshooting.md) | Diagnostics and resolution for common setup & render issues |
| [12. Reference](handbook/12-reference.md) | Technical reference, route map, environment variables, and CLI commands |


## Desktop app and web deployment

Electron packaging reuses the Go backend and Svelte interface, bundling the media tools for desktop users. Separate frontend/backend containers remain available for web deployment. See [Electron and web build instructions](docs/ELECTRON-AND-WEB.md). The Electron workflow builds macOS and Linux installers for Intel and ARM processors.

## Cloud SaaS development

The AWS SaaS application has separate PostgreSQL API, worker and operator commands, plus a SvelteKit/shadcn-svelte web interface. It does not expose the desktop server's global SQLite routes to cloud users. See [cloud deployment instructions](docs/CLOUD-DEPLOYMENT.md), [implementation status](docs/CLOUD-IMPLEMENTATION.md), and the [platform specification](docs/superpowers/specs/2026-10-15-cloud-saas-platform-design.md).

Start local dependencies with `docker compose --env-file .env.cloud -f compose.cloud.yml up -d` after copying `.env.cloud.example` to `.env.cloud`. Add `--profile app up --build` to run the cloud applications. Credits start at zero; external generation, payments and payouts require explicitly configured services and spending policies. Existing desktop commands remain unchanged.

Use `scripts/split-repositories.sh --output NEW_DIRECTORY` to create independent local `6eek-web` and `6eek-api` source repositories without deleting this repository or pushing to GitHub.
