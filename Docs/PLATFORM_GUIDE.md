# GestureShare — Platform Guide

**Platform-specific requirements, gotchas, and workarounds.**  
**Version:** 1.0 | **Platforms:** Windows 10+, macOS 12+, Ubuntu 20.04+

This document exists because the generic implementation plan cannot cover what breaks differently on each OS. An agent building only on macOS will produce a binary that fails silently on Windows and crashes on Linux. Read the relevant section before implementing anything that touches: network sockets, mDNS, TLS, camera, file system, system tray, or builds.

---

## 1. Tauri Sidecar: Binary Naming Convention

This is the single most common agent mistake. Tauri requires sidecar binaries to be named with the exact target triple appended.

### Required Binary Names

| Platform | Binary Name |
|----------|-------------|
| macOS Apple Silicon | `sidecar-aarch64-apple-darwin` |
| macOS Intel | `sidecar-x86_64-apple-darwin` |
| Windows x64 | `sidecar-x86_64-pc-windows-msvc.exe` |
| Linux x64 | `sidecar-x86_64-unknown-linux-gnu` |
| Linux ARM64 | `sidecar-aarch64-unknown-linux-gnu` |

### tauri.conf.json — Sidecar Registration

```json
{
  "tauri": {
    "bundle": {
      "externalBin": [
        "binaries/sidecar"
      ]
    },
    "allowlist": {
      "shell": {
        "sidecar": true,
        "scope": [
          { "name": "sidecar", "sidecar": true }
        ]
      }
    }
  }
}
```

The files must live at `src-tauri/binaries/sidecar-<triple>` and `src-tauri/binaries/sidecar-<triple>.exe`.

### Build Script — Cross-Compilation

```bash
# scripts/build-sidecar.sh
set -e
SIDECAR_DIR="apps/desktop/src-tauri/binaries"
mkdir -p "$SIDECAR_DIR"

# macOS (run on macOS only)
if [[ "$OSTYPE" == "darwin"* ]]; then
    GOOS=darwin  GOARCH=arm64  go build -o "$SIDECAR_DIR/sidecar-aarch64-apple-darwin" ./sidecar
    GOOS=darwin  GOARCH=amd64  go build -o "$SIDECAR_DIR/sidecar-x86_64-apple-darwin" ./sidecar
fi

# Windows (requires CGO_ENABLED=0 for cross-compile)
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o "$SIDECAR_DIR/sidecar-x86_64-pc-windows-msvc.exe" ./sidecar

# Linux
GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -o "$SIDECAR_DIR/sidecar-x86_64-unknown-linux-gnu" ./sidecar
```

### Rust: Spawning the Sidecar

```rust
// src-tauri/src/sidecar/mod.rs
use tauri::api::process::Command;

pub fn spawn(app: &tauri::AppHandle) {
    let (mut rx, _child) = Command::new_sidecar("sidecar")
        .expect("sidecar binary not found — run scripts/build-sidecar.sh first")
        .spawn()
        .expect("failed to spawn sidecar");

    // Read events from sidecar stdout
    tauri::async_runtime::spawn(async move {
        while let Some(event) = rx.recv().await {
            // handle event
        }
    });
}
```

---

## 2. macOS

### 2.1 Network Entitlements

The Tauri app requires network client and server entitlements. Without these, all socket operations fail silently in the sandboxed app.

Add to `src-tauri/Entitlements.plist` (create if it doesn't exist):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>com.apple.security.network.client</key>
    <true/>
    <key>com.apple.security.network.server</key>
    <true/>
    <key>com.apple.security.device.camera</key>
    <true/>
</dict>
</plist>
```

Reference it in `tauri.conf.json`:

```json
{
  "tauri": {
    "macOS": {
      "entitlements": "Entitlements.plist",
      "minimumSystemVersion": "12.0"
    }
  }
}
```

### 2.2 Camera Permission

macOS requires a privacy usage description for camera access. In `tauri.conf.json`:

```json
{
  "tauri": {
    "macOS": {
      "infoPlist": {
        "NSCameraUsageDescription": "GestureShare uses your camera to detect hand gestures for file transfer."
      }
    }
  }
}
```

The SvelteKit WebView inherits this permission — `getUserMedia()` will work after the user grants it once.

### 2.3 mDNS on macOS

macOS has native mDNS (Bonjour) support. The Go `hashicorp/mdns` package works without any additional dependencies. No firewall prompt is shown for mDNS traffic.

**Known issue:** If the app is not code-signed, macOS may block the Go sidecar binary from running. During development:
```bash
xattr -d com.apple.quarantine apps/desktop/src-tauri/binaries/sidecar-aarch64-apple-darwin
```

### 2.4 Self-signed TLS on macOS Safari

Safari on iOS will show a certificate warning when opening the QR URL. The user must:
1. Tap "Show Details"
2. Tap "Visit this website"
3. Tap "Visit Website" in the confirmation

This is expected and unavoidable without a real CA cert. Document this in the app's help text: *"Tap 'Visit website' when Safari shows a security warning — this is your local PC, not the internet."*

### 2.5 App Notarization (for distribution)

Unsigned builds will be blocked by Gatekeeper on macOS 12+. For distribution:

```bash
# 1. Sign with Developer ID
codesign --deep --force --sign "Developer ID Application: Your Name (TEAMID)"     "GestureShare.app"

# 2. Notarize
xcrun notarytool submit "GestureShare.dmg"     --apple-id "your@email.com"     --team-id "TEAMID"     --password "@keychain:AC_PASSWORD"     --wait

# 3. Staple
xcrun stapler staple "GestureShare.dmg"
```

Tauri's built-in signing handles this when `APPLE_SIGNING_IDENTITY` env var is set.

---

## 3. Windows

### 3.1 mDNS on Windows

**This is the biggest Windows-specific issue.** The `hashicorp/mdns` package uses raw UDP multicast which requires:

1. **Windows 10 1903+**: Works natively if Windows Defender Firewall allows the app (firewall dialog will appear on first launch)
2. **Older Windows 10 / Corporate firewalls**: mDNS may be blocked entirely

**Implementation requirement:**

```go
// sidecar/mdns/discovery.go
// On Windows, catch the mDNS error and emit CONN_MDNS_FAIL instead of crashing
func (d *Discovery) Start() (<-chan PeerInfo, error) {
    if err := d.advertise(); err != nil {
        // On Windows with firewall block, this returns "bind: permission denied"
        if isWindowsMDNSError(err) {
            ipc.Emit(ipc.EvtError, ipc.ErrorPayload{
                Code:    "CONN_MDNS_FAIL",
                Message: "mDNS blocked by firewall — use text code to pair",
            })
            return nil, err
        }
        return nil, fmt.Errorf("mDNS start: %w", err)
    }
    // ...
}

func isWindowsMDNSError(err error) bool {
    return strings.Contains(err.Error(), "permission denied") ||
           strings.Contains(err.Error(), "access is denied")
}
```

**Windows Firewall Inbound Rule:** Tauri's installer can add a firewall rule. Add to `tauri.conf.json`:

```json
{
  "tauri": {
    "windows": {
      "windowsSDK": true
    }
  }
}
```

And in the NSIS installer script, add:
```
netsh advfirewall firewall add rule name="GestureShare" dir=in action=allow program="$INSTDIR\GestureShare.exe" enable=yes
```

### 3.2 File Paths on Windows

**Never hardcode forward slashes.** Always use `filepath.Join()` in Go and `path.join()` in Node:

```go
// WRONG
outPath := downloadDir + "/" + filename

// CORRECT
outPath := filepath.Join(downloadDir, filename)
```

**Downloads directory on Windows:**

```go
// Go — works on all platforms including Windows
func getDownloadsDir() string {
    home, err := os.UserHomeDir()
    if err != nil { return os.TempDir() }
    
    // Windows: C:\Users\Username\Downloads
    // macOS:   /Users/Username/Downloads
    // Linux:   /home/username/Downloads
    return filepath.Join(home, "Downloads")
}
```

### 3.3 Camera Access on Windows

Windows 10+ requires a privacy permission for camera. If the Tauri WebView is Chromium-based (WebView2), camera access via `getUserMedia()` works with a one-time user permission prompt — no extra configuration needed.

**Required:** WebView2 runtime must be installed (bundled with Windows 11, installer needed for Windows 10). Tauri bundles this automatically in the installer.

### 3.4 Port 47291 and Windows Defender

When the Go sidecar first opens port 47291 as a TLS server, Windows Defender Firewall will show a popup asking to allow or deny. The user must click "Allow". 

Add user-facing instructions: *"When Windows asks about firewall access, click 'Allow access' to enable local network sharing."*

### 3.5 Windows Portable Binary

The portable `.exe` runs without installation. It cannot:
- Register a firewall rule automatically (needs elevation)
- Set itself as a startup application
- Access system tray reliably on all configurations

For the portable build, document these limitations. The installed version (NSIS installer) handles all of these.

### 3.6 Path Lengths on Windows

Windows has a 260-character MAX_PATH limit. Long filenames from phones (especially Chinese/Japanese characters) can hit this. Enforce a 200-character filename limit:

```go
func sanitizeFilename(name string) string {
    name = filepath.Base(name)  // strip path
    if len(name) > 200 {
        ext := filepath.Ext(name)
        name = name[:200-len(ext)] + ext
    }
    // Replace invalid Windows characters: \ / : * ? " < > |
    for _, c := range []string{"\\", "/", ":", "*", "?", "\"", "<", ">", "|"} {
        name = strings.ReplaceAll(name, c, "_")
    }
    return name
}
```

---

## 4. Linux

### 4.1 System Tray

Linux does not have a universal system tray. The `libappindicator` library is needed for GTK-based environments (GNOME, Unity). KDE uses StatusNotifierItem.

In `Cargo.toml`:
```toml
[dependencies]
tauri = { version = "1.6", features = ["system-tray"] }
```

In `tauri.conf.json`:
```json
{
  "tauri": {
    "systemTray": {
      "iconPath": "icons/tray.png",
      "iconAsTemplate": false
    }
  }
}
```

System tray may not appear on GNOME without the AppIndicator extension. Gracefully handle absence — the app must work without tray.

### 4.2 Camera Access on Linux

`getUserMedia()` works in Tauri WebView (WebKitGTK) but requires:
- User must be in the `video` group: `sudo usermod -a -G video $USER`
- Or: Flatpak/AppImage with proper portal permissions

For AppImage distribution, camera works without special handling on most modern distros.

### 4.3 mDNS on Linux

The `hashicorp/mdns` package works on Linux. No firewall issues by default on home networks. Corporate networks with `iptables` dropping multicast will block mDNS — same fallback as Windows.

**Avahi conflict:** If Avahi is running, it may conflict with the Go mDNS server trying to bind to the same multicast group. Detect and log:

```go
if err := d.advertise(); err != nil {
    if strings.Contains(err.Error(), "address already in use") {
        // Avahi is likely running — log warning, don't crash
        log.Printf("[mdns] warning: cannot advertise (Avahi conflict?) — discovery may be limited")
        // Fall through to scan-only mode
    }
}
```

### 4.4 AppImage Considerations

The AppImage bundles everything including WebKitGTK. It's the recommended Linux distribution format because it works across distros without dependencies.

```bash
# Tauri produces AppImage automatically:
cargo tauri build --target x86_64-unknown-linux-gnu
# Output: src-tauri/target/release/bundle/appimage/gestureshare_0.1.0_amd64.AppImage
```

Ensure the Go sidecar is compiled for Linux and placed in `src-tauri/binaries/` before building.

---

## 5. Self-Signed TLS — Browser Behavior by Platform

| Browser | Behavior on self-signed cert | Workaround |
|---------|------------------------------|-----------|
| Safari iOS | Full-page warning, user taps "Visit website" | Document in UX: expected behavior |
| Chrome Android | Interstitial warning, "Advanced" → "Proceed" | Document in UX: expected behavior |
| Firefox Android | Error page, "Accept the Risk" | Document in UX |
| Chrome Desktop | Same interstitial | Only for testing; production uses mobile |
| Edge | Same as Chrome | Same |

**Important:** The TLS warning does NOT bypass the cert fingerprint check. The fingerprint comparison happens AFTER the user proceeds through the warning. The security model holds even if the user clicks through a warning for a legitimate connection, and would also correctly fail if an attacker tried to MITM with a different cert.

---

## 6. GitHub Actions CI/CD

### Workflow File: `.github/workflows/build.yml`

```yaml
name: Build

on:
  push:
    tags: ["v*"]
  pull_request:
    branches: [main]

jobs:
  build:
    strategy:
      matrix:
        include:
          - os: macos-latest
            target: aarch64-apple-darwin
            go_os: darwin
            go_arch: arm64
          - os: macos-13
            target: x86_64-apple-darwin
            go_os: darwin
            go_arch: amd64
          - os: windows-latest
            target: x86_64-pc-windows-msvc
            go_os: windows
            go_arch: amd64
          - os: ubuntu-20.04
            target: x86_64-unknown-linux-gnu
            go_os: linux
            go_arch: amd64

    runs-on: ${{ matrix.os }}

    steps:
      - uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.22"

      - name: Build Go sidecar
        working-directory: sidecar
        env:
          GOOS: ${{ matrix.go_os }}
          GOARCH: ${{ matrix.go_arch }}
          CGO_ENABLED: 0
        run: |
          mkdir -p ../apps/desktop/src-tauri/binaries
          go build -o ../apps/desktop/src-tauri/binaries/sidecar-${{ matrix.target }}${{ matrix.go_os == 'windows' && '.exe' || '' }} .

      - name: Setup Rust
        uses: dtolnay/rust-toolchain@stable
        with:
          targets: ${{ matrix.target }}

      - name: Setup Node
        uses: actions/setup-node@v4
        with:
          node-version: 20

      - name: Install Linux dependencies
        if: matrix.os == 'ubuntu-20.04'
        run: |
          sudo apt-get update
          sudo apt-get install -y libwebkit2gtk-4.0-dev libayatana-appindicator3-dev

      - name: Install frontend dependencies
        working-directory: apps/desktop/frontend
        run: npm ci

      - name: Build Tauri app
        uses: tauri-apps/tauri-action@v0
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        with:
          projectPath: apps/desktop
          tagName: ${{ github.ref_name }}
          releaseName: GestureShare ${{ github.ref_name }}

      - name: Upload artifacts
        uses: actions/upload-artifact@v4
        with:
          name: gestureshare-${{ matrix.target }}
          path: apps/desktop/src-tauri/target/release/bundle/
```

---

## 7. Environment Variables and Configuration

All configurable values are centralized in `sidecar/config/config.go`. Never hardcode magic numbers outside this file.

```go
// sidecar/config/config.go
package config

import (
    "os"
    "strconv"
    "time"
)

type Config struct {
    // Network
    HTTPSPort       int           // default: 47291
    TCPPortMin      int           // default: 47300 (random port for TCP transfer, from this range)
    TCPPortMax      int           // default: 47400

    // mDNS
    MDNSService     string        // default: "_gestureshare._tcp"
    MDNSScanInterval time.Duration // default: 3s

    // Session
    SessionTTL      time.Duration // default: 1h
    PairingTimeout  time.Duration // default: 10s

    // Transfer
    TCPChunkSize    int           // default: 65536 (64KB)
    HTTPSChunkSize  int           // default: 1048576 (1MB — for browser encrypt chunks)
    MaxFileSize     int64         // default: 10GB
    ProgressInterval time.Duration // default: 250ms
    TransferTimeout time.Duration // default: 30s (no progress for this long = timeout)

    // Crypto
    TLSCertValidity time.Duration // default: 24h

    // Dev
    DevMode         bool          // if true: skip cert fingerprint check, verbose logging
}

func Load() Config {
    return Config{
        HTTPSPort:        envInt("GS_PORT", 47291),
        TCPPortMin:       envInt("GS_TCP_MIN", 47300),
        TCPPortMax:       envInt("GS_TCP_MAX", 47400),
        MDNSService:      envStr("GS_MDNS_SERVICE", "_gestureshare._tcp"),
        MDNSScanInterval: envDuration("GS_MDNS_SCAN_INTERVAL", 3*time.Second),
        SessionTTL:       envDuration("GS_SESSION_TTL", time.Hour),
        PairingTimeout:   envDuration("GS_PAIRING_TIMEOUT", 10*time.Second),
        TCPChunkSize:     envInt("GS_TCP_CHUNK", 65536),
        HTTPSChunkSize:   envInt("GS_HTTPS_CHUNK", 1048576),
        MaxFileSize:      int64(envInt("GS_MAX_FILE_SIZE", 10*1024*1024*1024)),
        ProgressInterval: envDuration("GS_PROGRESS_INTERVAL", 250*time.Millisecond),
        TransferTimeout:  envDuration("GS_TX_TIMEOUT", 30*time.Second),
        TLSCertValidity:  envDuration("GS_CERT_TTL", 24*time.Hour),
        DevMode:          envBool("GS_DEV", false),
    }
}

func envInt(key string, def int) int {
    if v := os.Getenv(key); v != "" {
        if n, err := strconv.Atoi(v); err == nil { return n }
    }
    return def
}
func envStr(key, def string) string {
    if v := os.Getenv(key); v != "" { return v }
    return def
}
func envDuration(key string, def time.Duration) time.Duration {
    if v := os.Getenv(key); v != "" {
        if d, err := time.ParseDuration(v); err == nil { return d }
    }
    return def
}
func envBool(key string, def bool) bool {
    if v := os.Getenv(key); v != "" {
        return v == "1" || v == "true" || v == "yes"
    }
    return def
}
```

The TypeScript equivalent — gesture thresholds are configurable via Vite env vars:

```typescript
// src/lib/gesture/config.ts
export const GESTURE_CONFIG = {
    grabThreshold:     Number(import.meta.env.VITE_GRAB_THRESHOLD    ?? 0.065),
    openThreshold:     Number(import.meta.env.VITE_OPEN_THRESHOLD    ?? 0.130),
    requiredHoldFrames:Number(import.meta.env.VITE_HOLD_FRAMES       ?? 8),
    targetFPS:         Number(import.meta.env.VITE_TARGET_FPS        ?? 30),
}
```
