# 2. Install from zero

[Handbook](README.md) · Previous: [How it works](01-how-it-works.md) · Next: [Providers](03-providers.md)

You will run two processes: the high-performance Go backend on port 8000 and the SvelteKit frontend on port 5173. 

> [!NOTE]
> **No Python or uv Required**: 6eek Go is a streamlined, single-binary Go backend. It eliminates Python, `uv`, virtual environments (`.venv`), and complex runtime setups.

---

## 1. Install the prerequisites

| Tool | Requirement | Install |
|---|---|---|
| Git | Download and update the repository | [Official installers](https://git-scm.com/downloads/) |
| Go | Go 1.24+ (if building from source) | [Official Go download](https://go.dev/dl/) |
| Node.js and npm | Node 20+ (Node 22.12+ recommended); npm comes with Node | [Official Node download](https://nodejs.org/en/download) |
| FFmpeg | Needed for captions, thumbnails, and QA frame extraction; use a build with libass | [Official download options](https://ffmpeg.org/download.html) |

Open Terminal on macOS/Linux or PowerShell on Windows. Check the installed tools:

```sh
git --version
go version
node --version
npm --version
ffmpeg -version
```

A GPU is not required by the local application: AI media generation is handled via remote provider APIs, and the backend runs as an ultra-lightweight compiled executable (~13 MB RAM).

---

## 2. Download 6eek Go

Clone the repository:

```sh
git clone https://github.com/JienWeng/videoflow-go.git
cd 6eek-go
```

The repository root contains the Go backend source code (`cmd/server`, `internal/`) and the frontend directory (`frontend/`). No Python package synchronization or virtualenv creation is needed.

---

## 3. Create configuration

On macOS/Linux:

```sh
cp .env.example .env
```

On Windows PowerShell:

```powershell
Copy-Item .env.example .env
```

Use this copy step only on a fresh clone. Preserve an existing `.env` when upgrading. Open `.env` in a text editor. You may leave keys empty to explore the workspace; generation requires configured accounts. Complete [Part 3](03-providers.md) before generating.

The default configuration creates `db.sqlite` and `storage/` locally. The embedded SQLite database initializes tables automatically on startup.

---

## 4. Build and Start the Go Backend

In terminal A, from the repository root:

```sh
make build
./bin/videoflow-server
```

*(Alternatively, without `make`: `go build -o bin/videoflow-server ./cmd/server && ./bin/videoflow-server`)*

The server will start instantly (< 50 ms) and listen on `http://127.0.0.1:8000`.

Keep the terminal open. Visit [API health](http://localhost:8000/health). A successful health response confirms that the Go API is up.

---

## 5. Start the frontend

Open terminal B in the repository root:

```sh
cd frontend
npm install
npm run dev -- --host 127.0.0.1
```

Visit [Create video](http://localhost:5173/create). The Go API must remain running. If Vite selects another port because 5173 is occupied, free port 5173 and restart: backend CORS currently permits 5173 and 4173.

For a local production build:

```sh
npm run build
npm run preview -- --host 127.0.0.1
```

---

## 6. Prepare captions if needed

FFmpeg must include the `ass` filter. Check it with `ffmpeg -filters`. For Chinese captions, put `NotoSansCJKsc-Bold.otf` in `storage/fonts/`. Create that directory before downloading. Obtain the font from the [Noto CJK repository](https://github.com/notofonts/noto-cjk/tree/main/Sans/OTF/SimplifiedChinese).

---

## Stop and restart

Wait for operations to finish, then press `Ctrl+C` in each terminal. On subsequent launches, just run:
1. Terminal A: `./bin/videoflow-server`
2. Terminal B: `cd frontend && npm run dev -- --host 127.0.0.1`

Back up the database (`db.sqlite`) and `storage/` together before upgrades. See [Projects](04-projects.md). For startup errors, use [Troubleshooting](11-troubleshooting.md).
