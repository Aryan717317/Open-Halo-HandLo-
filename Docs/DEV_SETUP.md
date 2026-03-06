# GestureShare — Developer Environment & Toolchain Setup

**Version:** 1.0  
**Purpose:** Complete environment setup an agent can execute top-to-bottom on a fresh machine to get a working dev environment. Zero assumptions about what's installed.

---

## 1. Prerequisites

### Required Versions

| Tool | Minimum Version | Check Command | Install |
|------|----------------|--------------|---------|
| Go | 1.22+ | `go version` | https://go.dev/dl |
| Rust | 1.77+ | `rustc --version` | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` |
| Node.js | 20 LTS+ | `node --version` | https://nodejs.org |
| npm | 10+ | `npm --version` | bundled with Node |
| Git | 2.40+ | `git --version` | https://git-scm.com |

### Platform-specific Requirements

**macOS:**
```bash
# Xcode Command Line Tools (required for Rust build)
xcode-select --install

# Required for Tauri
brew install pkg-config
```

**Windows:**
```powershell
# Visual Studio Build Tools (required for Rust)
# Download from: https://aka.ms/vs/17/release/vs_BuildTools.exe
# Install: "Desktop development with C++"

# WebView2 (required for Tauri — usually pre-installed on Windows 11)
# If missing: https://developer.microsoft.com/en-us/microsoft-edge/webview2/
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt update
sudo apt install -y libwebkit2gtk-4.0-dev build-essential curl wget file \
  libssl-dev libgtk-3-dev libayatana-appindicator3-dev librsvg2-dev \
  libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev
```

---

## 2. Repository Setup

```bash
# Clone
git clone https://github.com/your-org/gestureshare
cd gestureshare

# Verify structure
ls
# Should show: apps/  sidecar/  browser-client/  docs/  scripts/
```

---

## 3. Go Sidecar Setup

```bash
cd sidecar

# Install dependencies
go mod tidy

# Verify it compiles
go build ./...

# Run tests
go test ./...

# Expected output: ok  github.com/gestureshare/sidecar/crypto
#                  ok  github.com/gestureshare/sidecar/ipc
# (some packages may show [no test files] — that's fine)

# Install air for hot reload in development (optional but recommended)
go install github.com/air-verse/air@latest

cd ..
```

### Go Module Dependencies Explained

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/hashicorp/mdns` | v1.0.5 | mDNS broadcast and discovery |
| `github.com/pion/webrtc/v3` | v3.2.40 | WebRTC (desktop-to-desktop signaling only in Phase 4) |
| `golang.org/x/crypto` | latest | HKDF-SHA512, Curve25519 extras |
| `nhooyr.io/websocket` | v1.8.10 | WebSocket server for push notifications |
| `golang.design/x/clipboard` | v0.7.0 | Read/write system clipboard |

---

## 4. Rust / Tauri Setup

```bash
cd apps/desktop

# Install Tauri CLI
cargo install tauri-cli

# Verify Rust toolchain
rustc --version   # should be 1.77+
cargo --version

# Build dependencies (first time takes 2-5 minutes)
cd src-tauri
cargo build
cd ..
```

### Rust Crate Dependencies Explained

| Crate | Version | Purpose |
|-------|---------|---------|
| `tauri` | 1.6 | Desktop shell, WebView, system APIs |
| `serde` + `serde_json` | 1.0 | JSON serialization for IPC |
| `tokio` | 1 (full) | Async runtime for sidecar communication |
| `rcgen` | 0.12 | Self-signed TLS certificate generation |
| `uuid` | 1.6 | Generate transfer IDs |
| `hostname` | 0.3 | Get device hostname |

---

## 5. SvelteKit Frontend Setup

```bash
cd apps/desktop/frontend

# Install all dependencies
npm install

# Type check
npm run check

# Verify dev server starts
npm run dev
# Should show: Local: http://localhost:5173

cd ../../..
```

### NPM Package Dependencies Explained

| Package | Version | Purpose |
|---------|---------|---------|
| `@mediapipe/tasks-vision` | ^0.10.9 | MediaPipe Hands WASM — offline gesture detection |
| `@tauri-apps/api` | ^1.6.0 | Tauri invoke() and listen() bridge |
| `three` | ^0.160.0 | 3D phantom hand + file orb rendering |
| `qrcode` | ^1.5.3 | QR code image generation |
| `jsqr` | ^1.4.0 | QR code scanning from camera frame |
| `@sveltejs/adapter-static` | ^3.0 | Build to static files for Tauri |

---

## 6. Running in Development

### Method A: All Together (Recommended)
```bash
cd apps/desktop
cargo tauri dev
```
This command:
1. Starts SvelteKit dev server on `:5173` (hot reload enabled)
2. Builds and launches the Tauri native window pointing to `:5173`
3. Rust spawns the Go sidecar automatically on startup

Watch logs in:
- Terminal: Rust + Go sidecar logs (stderr)
- Browser DevTools (F12): SvelteKit + IPC event logs

### Method B: Sidecar Separately (For Go Development)
```bash
# Terminal 1: Go sidecar with hot reload
cd sidecar
air     # requires: go install github.com/air-verse/air@latest
# OR without air:
go run .

# Terminal 2: Tauri app (will spawn sidecar again — kill the first one)
cd apps/desktop
GESTURESHARE_DEV_SIDECAR=0 cargo tauri dev
```

### Method C: Frontend Only (For UI Development)
```bash
cd apps/desktop/frontend
npm run dev
# Open http://localhost:5173 in browser
# Tauri APIs won't work — mock them with the dev stubs
```

---

## 7. Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `GESTURESHARE_PORT` | `47291` | HTTPS server port |
| `GESTURESHARE_DEV_SIDECAR` | `1` | `0` = don't spawn sidecar (use external) |
| `GESTURESHARE_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `GESTURESHARE_NO_TLS` | `0` | `1` = disable TLS (dev only, breaks security) |
| `RUST_LOG` | `warn` | Rust log filter (e.g., `gestureshare=debug`) |

Set in your shell or in a `.env` file at repo root (not committed).

---

## 8. Building for Production

```bash
# Build Go sidecar for all platforms
./scripts/build.sh

# This produces binaries in apps/desktop/src-tauri/binaries/:
# sidecar-x86_64-apple-darwin
# sidecar-aarch64-apple-darwin
# sidecar-x86_64-pc-windows-msvc.exe
# sidecar-x86_64-unknown-linux-gnu

# Build Tauri app (runs on current platform)
cd apps/desktop
cargo tauri build

# Installers appear in:
# apps/desktop/src-tauri/target/release/bundle/
#   macos/  → GestureShare.dmg
#   windows/ → GestureShare_0.1.0_x64_en-US.msi + .exe
#   appimage/ → GestureShare_0.1.0_amd64.AppImage
```

---

## 9. Common Issues & Fixes

### "Go sidecar not found"
```bash
# Check binary exists
ls apps/desktop/src-tauri/binaries/
# If empty: run ./scripts/build.sh first
# In dev mode: the sidecar is run via `go run .` not a binary
```

### "WebView2 not found" (Windows)
```powershell
# Install WebView2 runtime
Start-Process "https://developer.microsoft.com/en-us/microsoft-edge/webview2/"
```

### "MediaPipe fails to load in Tauri"
```javascript
// Add to tauri.conf.json security.csp:
"connect-src": "https://cdn.jsdelivr.net 'self'"
// MediaPipe WASM downloads from jsdelivr CDN on first run
// After first run it's cached — add offline caching in Phase 7
```

### "mDNS doesn't work"
```bash
# macOS: check firewall
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --listapps | grep gestureshare

# Linux: check firewall
sudo ufw status
# If active: sudo ufw allow 5353/udp    (mDNS port)
# Also allow the HTTPS port:
sudo ufw allow 47291/tcp

# All platforms: mDNS requires devices on same subnet
# VPNs often break mDNS — disable VPN for testing
```

### "TLS certificate not trusted by browser"
This is expected — the cert is self-signed. The browser will show a warning.
In development: click "Advanced" → "Proceed anyway" on first visit.
In production: cert fingerprint is verified via QR code, bypassing the browser warning.

### "camera permission denied in Tauri"
```json
// Add to tauri.conf.json:
"allowlist": {
    "window": {
        "create": true
    }
}
// On macOS: add camera usage description to Info.plist
```

### "Go sidecar crashes immediately"
```bash
# Run sidecar standalone to see the error
cd sidecar
go run . 2>&1
# Check for: port already in use, missing dependencies
```

---

## 10. IDE Setup

### VS Code (Recommended)

Extensions to install:
```
rust-analyzer          # Rust IntelliSense
golang.go              # Go IntelliSense
svelte.svelte-vscode   # Svelte
bradlc.vscode-tailwindcss  # Tailwind (if used)
```

Settings (`/.vscode/settings.json`):
```json
{
  "rust-analyzer.cargo.features": "all",
  "go.toolsManagement.autoUpdate": true,
  "[svelte]": {
    "editor.defaultFormatter": "svelte.svelte-vscode"
  },
  "editor.formatOnSave": true
}
```

### Project-level TypeScript Config

```json
// apps/desktop/frontend/tsconfig.json
{
  "extends": "./.svelte-kit/tsconfig.json",
  "compilerOptions": {
    "strict": true,
    "noUncheckedIndexedAccess": true
  }
}
```

---

## 11. CI/CD Pipeline (GitHub Actions)

File: `.github/workflows/build.yml`
```yaml
name: Build & Test

on: [push, pull_request]

jobs:
  test-go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - run: cd sidecar && go test ./...
      - run: cd sidecar && go vet ./...

  test-frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: '20' }
      - run: cd apps/desktop/frontend && npm ci && npm test

  build-tauri:
    needs: [test-go, test-frontend]
    strategy:
      matrix:
        os: [macos-latest, windows-latest, ubuntu-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: '20' }
      - uses: actions-rs/toolchain@v1
        with: { toolchain: stable }
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - name: Install Linux deps
        if: matrix.os == 'ubuntu-latest'
        run: |
          sudo apt update
          sudo apt install -y libwebkit2gtk-4.0-dev libssl-dev
      - name: Build Go sidecar
        run: ./scripts/build.sh
      - name: Build Tauri
        run: cd apps/desktop && cargo tauri build
      - uses: actions/upload-artifact@v4
        with:
          name: bundle-${{ matrix.os }}
          path: apps/desktop/src-tauri/target/release/bundle/
```
