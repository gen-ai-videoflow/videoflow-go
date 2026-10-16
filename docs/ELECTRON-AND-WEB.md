# One codebase, desktop and web

The Svelte frontend and Go backend remain shared. Electron starts a private loopback backend on an automatically assigned port and displays the same frontend in an app window. Closing the app stops the backend. Projects and the OpenRouter key use the existing 6eek user configuration directory, so existing native download data remains available. Avoid running the browser download and Electron simultaneously against the same data folder.

Desktop packages include Electron, Go engine, compiled frontend, FFmpeg, FFprobe, fonts and the media licenses/corresponding source archive. End users do not install Node, Go, Python or FFmpeg. They still need internet access and an OpenRouter API key with credits for AI generation. Packaging does not make unsupported OpenRouter capabilities available.

## Build desktop packages (maintainers)

Build on the matching OS and CPU, using the existing native packaging instructions first. For example on Linux x64:

```sh
cd frontend
npm ci
npm run check
npm run build
cd ..
python3 scripts/build_media.py
python3 scripts/package.py --target linux-amd64
python3 scripts/smoke_package.py --archive dist/6eek-linux-amd64.zip
python3 scripts/prepare_electron.py --archive dist/6eek-linux-amd64.zip --target linux-amd64
python3 scripts/test_prepare_electron.py
cd electron
npm ci
npm test
VIDEOFLOW_INTEGRATION=1 npm test
npm start
npm run dist -- --linux --x64 --publish never
```

For macOS use `darwin-amd64` / `--mac --x64` or `darwin-arm64` / `--mac --arm64`. For Linux ARM use `linux-arm64` / `--linux --arm64`. Pass the matching `--target` to payload preparation. Binary headers and the Electron builder hook reject OS/CPU mismatches. Native executables are outside the Electron ASAR in `resources/engine` (macOS: `Contents/Resources/engine`). Output is `dist/electron`: macOS DMG and ZIP; Linux DEB, AppImage and tar.gz. On Ubuntu/Debian, prefer the DEB installer: it configures the Chromium sandbox and AppArmor profile automatically. AppImage/tar.gz depend on the distribution permitting Chromium user namespaces. Some Linux distributions need FUSE for AppImage; the extracted tar.gz is the alternative and Electron still requires standard graphical desktop libraries.

Electron dependencies are pinned by `electron/package-lock.json`; release builds use `npm ci`. The selected Electron 44 patch release should remain current.

Before publication, launch each built package on its native OS/CPU, verify projects and settings persist after reopening, run a media export, confirm closing the app leaves no Go/FFmpeg process, and verify the license/source files are present. Sign and notarize macOS releases with the maintainer's Apple Developer credentials for a smooth beginner installation; unsigned packages can show Gatekeeper warnings. No signing credentials are embedded in the source.

## Run the web frontend and backend

```sh
docker compose up --build -d
```

Open `http://127.0.0.1:8080`. The frontend container serves the SPA and proxies `/api/` to the Go backend, including streaming events and storage URLs. The backend is not published separately. Projects, settings and uploaded/generated files persist in the `videoflow-data` volume across container restarts and upgrades. `docker compose down` keeps that volume; deleting the volume removes the data.

The configuration is a single-user local deployment. For remote access, place an authenticated HTTPS reverse proxy in front of the loopback frontend port: the app's settings and generation endpoints are not a public multi-user service. Web runtime media tools are installed inside the backend image. Web deployment does not depend on Electron.

## Verification

The build workflow packages each OS/CPU on a matching native runner. The native ZIP smoke test checks media encoding, audio extraction and caption rendering using bundled tools; the Electron smoke test checks the rendered app window and backend shutdown. Run the latter with `python3 scripts/smoke_electron.py --executable <packaged-app-executable>` (use `xvfb-run -a` on Linux CI).

macOS signing/notarization is available through standard electron-builder credentials configured as repository secrets. Without those credentials, packages are unsigned and macOS may require explicit approval when opening them.
