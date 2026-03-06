# GestureShare — Implementation Plan

**Version:** 1.0  
**Status:** Active  
**Last Updated:** 2026-03-06  
**Total Duration:** 8 weeks to MVP · 14 weeks to v1.0

---

## How to Use This Document

This plan is written for an AI coding agent. Each phase has:
- **Context** — what exists before this phase starts
- **Goal** — what must be true when this phase ends
- **Tasks** — specific implementable units with file paths
- **Verification** — how to confirm the phase is done
- **Agent Instructions** — precise prompts to give the coding agent

---

## Phase 0 — Project Bootstrap (Day 1)

### Context
Empty repository.

### Goal
Running Tauri app with Go sidecar spawning and SvelteKit loading in the WebView. IPC messages flow in both directions.

### Tasks

**T0.1 — Initialize Tauri project**
```
cd apps/desktop
cargo tauri init
# App name: GestureShare
# Window title: GestureShare
# Dev server: http://localhost:5173
```

**T0.2 — Initialize SvelteKit**
```
cd apps/desktop/frontend
npm create svelte@latest . -- --template skeleton --types typescript
npm install
npm install three @mediapipe/tasks-vision @tauri-apps/api qrcode jsqr
npm install -D @types/three @sveltejs/adapter-static
```

**T0.3 — Initialize Go sidecar**
```
mkdir sidecar && cd sidecar
go mod init github.com/gestureshare/sidecar
go get github.com/hashicorp/mdns@v1.0.5
go get github.com/pion/webrtc/v3@v3.2.40
go get golang.org/x/crypto@latest
```

**T0.4 — Wire IPC**
Files to create:
- `apps/desktop/src-tauri/src/sidecar/mod.rs` — spawn Go, read stdout events, write stdin commands
- `apps/desktop/src-tauri/src/main.rs` — register all commands, call sidecar::spawn on setup
- `sidecar/main.go` — start IPC read loop
- `sidecar/ipc/protocol.go` — all message type constants and payload structs
- `sidecar/ipc/router.go` — dispatch incoming commands to handlers

**T0.5 — Smoke test IPC**
- Rust sends `CMD_GET_DEVICE_INFO` to Go stdin on startup
- Go responds with `EVT_DEVICE_INFO {name, os, version}`
- Rust emits to SvelteKit via `window.emit()`
- SvelteKit `+page.svelte` listens and logs device name to console

### Verification
```bash
cargo tauri dev
# Console shows: "[sidecar] Ready. Listening for IPC commands..."
# Browser console shows: "Device: MyMacBook"
```

### Agent Instructions
> "Implement a Tauri application that spawns a Go sidecar binary on startup. The Rust sidecar module should spawn Go using `std::process::Command`, read its stdout line by line (each line is a JSON object), and emit each parsed event to the Tauri frontend using `app.emit_all()`. Commands from the frontend should be serialized as JSON and written to Go's stdin. Create the Go main.go with a stdin reader loop that parses JSON commands and dispatches them. Start with a single round-trip: Rust sends CMD_GET_DEVICE_INFO, Go responds with EVT_DEVICE_INFO containing the hostname."

---

## Phase 1 — LAN Discovery (Days 2-3)

### Context
IPC pipeline working. Devices cannot yet find each other.

### Goal
Two devices running GestureShare see each other in the peer list within 5 seconds of launching.

### Tasks

**T1.1 — mDNS broadcaster**  
File: `sidecar/mdns/discovery.go`
- On `CMD_DISCOVER`: register `_gestureshare._tcp` service on port 47291
- Include TXT records: `version=1.0`, `name=<hostname>`, `os=<os>`
- Scan every 3 seconds, emit `EVT_PEER_FOUND` for new peers
- Emit `EVT_PEER_LOST` when peer disappears from scan results

**T1.2 — Rust command handler**  
File: `apps/desktop/src-tauri/src/commands/transfer.rs`
- `start_discovery()` → send `CMD_DISCOVER` to Go
- `stop_discovery()` → send `CMD_STOP_DISCOVER` to Go

**T1.3 — SvelteKit peer store**  
File: `apps/desktop/frontend/src/lib/stores/peerStore.ts`
- Listen for `EVT_PEER_FOUND`, `EVT_PEER_LOST`
- Svelte writable store: `Map<id, PeerInfo>`
- Derived store: `peers` (array), `hasPeers` (boolean)

**T1.4 — Peer list UI**  
File: `apps/desktop/frontend/src/routes/+page.svelte`
- Call `start_discovery()` on mount
- Render peer list from `$peers` store
- Show scanning indicator when `!$hasPeers`

### Verification
```bash
# Terminal 1: cargo tauri dev (machine A)
# Terminal 2: cargo tauri dev (machine B, same WiFi)
# Both UIs show each other's hostname in the peer list within 5 seconds
```

### Agent Instructions
> "Implement mDNS peer discovery in Go using the `github.com/hashicorp/mdns` package. Create `sidecar/mdns/discovery.go` with a `Discovery` struct that: (1) advertises the local device on `_gestureshare._tcp` using `mdns.NewMDNSService` and `mdns.NewServer`, (2) scans for peers every 3 seconds using `mdns.Query` in a goroutine loop, (3) sends `EVT_PEER_FOUND` JSON to stdout when a new peer appears, and `EVT_PEER_LOST` when a peer disappears. Wire this to the IPC router so `CMD_DISCOVER` starts it and `CMD_STOP_DISCOVER` stops it. In SvelteKit, create a peer store that listens for these events using `@tauri-apps/api/event` `listen()` and updates a Svelte writable store."

---

## Phase 2 — Zero-Trust Handshake (Days 4-7)

### Context
Peer discovery works. No security layer yet.

### Goal
Phone scans QR code, browser completes full P-521 ECDH handshake, both sides hold matching AES-256-CTR key. All 5 handshake steps shown in browser UI.

### Tasks

**T2.1 — Go: Cryptographic identity generation**  
File: `sidecar/crypto/session.go`
- Generate P-521 keypair using `crypto/ecdh` on launch
- Generate 32-byte random session ID, hex-encode
- Generate self-signed TLS certificate (ECDSA P-256, 24h expiry)
- Compute SHA-256 fingerprint of certificate DER bytes
- Expose: `PubKeyB64()`, `SessionID()`, `CertFingerprint()`, `TLSCert()`

**T2.2 — Go: HTTPS server**  
File: `sidecar/server/https.go`
- Start `net/http` server with TLS config: version min 1.3, cert from T2.1
- Mount routes defined in T2.3
- CORS middleware for browser cross-origin requests

**T2.3 — Go: API routes**  
File: `sidecar/server/routes.go`
```
GET  /join                   → serve browser-client/index.html
GET  /api/cert-ping          → return {fingerprint: string}
POST /api/session/register   → ECDH handshake, return {token, desktopName}
```
- Session register: decode browser's P-521 pub key, run ECDH, HKDF → AES key, store session

**T2.4 — Go: ECDH + HKDF**  
File: `sidecar/crypto/ecdh.go`
- `DeriveAESKey(sharedBits []byte, sessionIDHex string) ([]byte, error)`
  - HKDF-SHA512, info=`"gestureshare-v1-aes"`, output 32 bytes
- `DeriveHMACKey(sharedBits []byte, sessionIDHex string) ([]byte, error)`
  - HKDF-SHA512, info=`"gestureshare-v1-hmac"`, output 32 bytes

**T2.5 — Go: QR payload builder**  
File: `sidecar/server/qr.go`
- `BuildQRURL(localIP string) string`
- Format: `https://<ip>:<port>/join#key=<pubkey_urlsafe_b64>&fp=<cert_sha256>&sid=<session_id>`

**T2.6 — Tauri: QR display**  
File: `apps/desktop/frontend/src/routes/+page.svelte`
- Fetch local IP address via Go IPC `CMD_GET_DEVICE_INFO`
- Generate QR code image from URL using `qrcode` npm package
- Display QR in connect panel

**T2.7 — Browser client**  
File: `browser-client/index.html`
- Already built — verify it works with the Go server
- Test all 5 handshake steps complete without error

**T2.8 — 6-digit text code**  
- Go generates 6-digit numeric code on launch
- Broadcasts code in mDNS TXT record: `code=847291`
- Browser client: `POST /api/pair/code {code}` → Go looks up peer by code in mDNS scan

### Verification
```bash
# 1. Launch app on macOS
# 2. Open iPhone Safari, scan QR
# 3. Browser shows all 5 handshake steps green
# 4. Desktop shows "Phone connected"
# 5. In browser console: Crypto.hasSession === true
# 6. Wireshark: all traffic is TLS, no plaintext
```

### Agent Instructions
> "Implement the zero-trust HTTPS pairing server in Go. Create `sidecar/server/https.go` that starts a TLS 1.3 HTTPS server using a self-signed ECDSA certificate generated by `sidecar/crypto/session.go`. The `/api/cert-ping` endpoint returns the certificate's SHA-256 fingerprint as JSON. The `/api/session/register` endpoint receives the browser's base64url-encoded P-521 public key and session ID, performs ECDH using the server's private key, derives AES-256-CTR and HMAC-SHA256 keys via HKDF-SHA512 (with info strings 'gestureshare-v1-aes' and 'gestureshare-v1-hmac' respectively), stores the session keyed by a random 32-byte token, and returns the token and desktop hostname. Use `crypto/ecdh` for P-521. Use `golang.org/x/crypto/hkdf` for HKDF."

---

## Phase 3 — File Transfer: Phone ↔ Desktop (Days 8-12)

### Context
Secure session established. No file transfer yet.

### Goal
Full file transfer in both directions (phone→desktop, desktop→phone) with real-time progress. 100MB file completes in under 10 seconds on WiFi.

### Tasks

**T3.1 — Go: AES-256-CTR encryption**  
File: `sidecar/crypto/ctr.go`
- `EncryptCTR(plaintext, key []byte) ([]byte, error)` → `[16-byte counter || ciphertext]`
- `DecryptCTR(data, key []byte) ([]byte, error)` → read counter from first 16 bytes
- Counter: 16 random bytes per encryption call (not sequential)

**T3.2 — Go: HMAC-SHA256**  
File: `sidecar/crypto/hmac.go`
- `SignHMAC(data, key []byte) string` → base64 of HMAC-SHA256
- `VerifyHMAC(data []byte, sigB64 string, key []byte) bool`

**T3.3 — Go: Upload handler (phone → desktop)**  
File: `sidecar/server/routes.go`
```
POST /api/transfer/upload
  Headers: X-GS-Token, X-GS-FileName, X-GS-OrigSize, X-GS-HMAC
  Body: full encrypted payload (AES-256-CTR)
```
- Auth session via token
- Read full body (stream, 10GB limit)
- Verify HMAC — reject if fails
- DecryptCTR → save to `~/Downloads/<filename>`
- Emit `EVT_TX_COMPLETE` to Rust via IPC
- Emit progress events as bytes arrive

**T3.4 — Go: Download handler (desktop → phone)**  
File: `sidecar/server/routes.go`
```
GET /api/transfer/download?transfer_id=<id>
```
- Auth session via token
- Read file from path stored during offer
- EncryptCTR in 64KB streaming chunks
- Stream response with `Content-Type: application/octet-stream`

**T3.5 — Go: Transfer offer flow**  
File: `sidecar/server/routes.go`
```
POST /api/transfer/offer    → tell browser a file is ready to download
POST /api/transfer/accept   → browser accepts, server starts streaming
POST /api/transfer/reject   → browser declines
```

**T3.6 — Rust: File send command**  
File: `apps/desktop/src-tauri/src/commands/transfer.rs`
- `send_file(filePath, peerId)` → send `CMD_SEND_FILE` to Go
- Listen for `EVT_TX_PROGRESS`, emit to SvelteKit

**T3.7 — SvelteKit: Transfer UI**  
File: `apps/desktop/frontend/src/routes/+page.svelte`
- File picker button → `invoke('pick_file')`
- Send button → `invoke('send_file', {filePath, peerId})`
- Progress bar with percent, MB/s, ETA
- Incoming file toast with accept/reject

**T3.8 — Browser: Transfer flow**  
File: `browser-client/index.html`
- Already implemented — verify upload flow with real server
- Verify progress ring updates in real-time
- Test file integrity (sha256 match)

### Verification
```bash
# Test 1: Phone → Desktop
# Select 100MB file on iPhone, send, verify in ~/Downloads
# sha256sum ~/Downloads/file == sha256sum original

# Test 2: Desktop → Phone
# Select file in Tauri UI, phone shows incoming toast, accept, verify

# Test 3: Tampering detection
# Intercept upload, flip one byte in payload
# Server should return 400 Bad Request (HMAC fail)
```

### Agent Instructions
> "Implement the file transfer handlers in the Go HTTPS server. For phone-to-desktop: the `/api/transfer/upload` POST handler should read the entire request body, verify the HMAC-SHA256 (from X-GS-HMAC header) against the ciphertext using the session's HMAC key, then decrypt using AES-256-CTR with the session's AES key. The decrypted bytes should be written to the user's Downloads directory. Emit progress to the Rust sidecar IPC as bytes are read. For desktop-to-phone: implement a GET `/api/transfer/download` that reads a file, encrypts it in 64KB chunks with AES-256-CTR (each chunk gets a fresh random 16-byte counter), and streams the response."

---

## Phase 4 — Desktop-to-Desktop Raw TCP (Days 13-16)

### Context
Phone ↔ desktop transfer works. No fast desktop-to-desktop path yet.

### Goal
Two desktops transfer files at ≥ 40 MB/s over raw encrypted TCP. ECDH shared key derived from already-established session.

### Tasks

**T4.1 — Go: TCP sender**  
File: `sidecar/transfer/tcp_sender.go`
- Open TCP listener on random port
- Signal port to peer via existing IPC/API
- For each 64KB chunk: encrypt with AES-256-CTR, write `[4-byte len || counter || ciphertext]`
- After all chunks: write HMAC of full ciphertext as final 32-byte packet
- Track bytes sent, compute MB/s, emit progress via IPC

**T4.2 — Go: TCP receiver**  
File: `sidecar/transfer/tcp_receiver.go`
- Connect to sender's TCP address
- Read length-prefixed packets: extract counter + ciphertext, decrypt, append to output file
- After all chunks received: read HMAC, verify over full decrypted file
- If HMAC fails: delete file, emit error
- If OK: emit `EVT_TX_COMPLETE` with file path

**T4.3 — Go: Progress tracker**  
File: `sidecar/transfer/progress.go`
- Track: bytes sent, total bytes, start time
- Compute: MB/s (rolling 1-second window), ETA
- Emit `EVT_TX_PROGRESS` every 250ms

**T4.4 — Buffer optimization**
- TCP socket buffer: `SO_SNDBUF` / `SO_RCVBUF` set to 4MB
- Read/write in 64KB chunks (not per-byte)
- Benchmark and confirm ≥ 40 MB/s on gigabit LAN

### Verification
```bash
# On machine A: prepare 1GB test file
dd if=/dev/urandom of=/tmp/test1gb.bin bs=1M count=1024

# Transfer to machine B, measure time
# time: < 25 seconds on gigabit (≥ 40 MB/s)
# sha256: matches original
```

### Agent Instructions
> "Implement raw TCP file transfer in Go. The sender (`sidecar/transfer/tcp_sender.go`) should: open a TCP listener, send a length-prefixed framing protocol where each packet is `[4-byte uint32 length][16-byte AES-CTR counter][ciphertext]`, encrypt each 64KB chunk with AES-256-CTR using a fresh random 16-byte counter, compute HMAC-SHA256 over all ciphertext (concatenated), and send the HMAC as the final 32 bytes. Set TCP socket buffers to 4MB using `syscall.SetsockoptInt`. The receiver mirrors this: read length-prefixed packets, decrypt each chunk, accumulate the decrypted file, verify the final HMAC. Emit progress events every 250ms."

---

## Phase 5 — Gesture Engine (Days 17-21)

### Context
All transfer paths working. No gesture control yet.

### Goal
Closed fist opens file picker. Open palm sends the selected file. 8-frame debouncing prevents false triggers. Gesture detection works reliably in varied lighting.

### Tasks

**T5.1 — MediaPipe initialization**  
File: `apps/desktop/frontend/src/lib/gesture/HandTracker.ts`
- Initialize `HandLandmarker` with GPU delegate, fallback to CPU
- `startCamera(videoEl, callback)` → getUserMedia + detection loop
- Detection loop: `detectForVideo` per animation frame
- Output: `HandLandmarkerResult` with 21 landmarks per hand

**T5.2 — Gesture classifier**  
File: `apps/desktop/frontend/src/lib/gesture/GestureClassifier.ts`
- GRAB: all 5 fingertips within `0.065` normalized distance of wrist
- OPEN_PALM: all 5 fingertips beyond `0.13` of wrist
- POINT: index beyond threshold, middle/ring/pinky below
- IDLE: anything else

**T5.3 — Debouncer**  
Integrated into `GestureClassifier.ts`
- Gesture must persist for 8 consecutive frames to confirm
- `holdCount` increments on same gesture, resets on change
- `confidence` property: `holdCount / REQUIRED_HOLD_FRAMES` (0→1)

**T5.4 — Gesture store**  
File: `apps/desktop/frontend/src/lib/stores/gestureStore.ts`
- Writable store: `{ gesture, landmarks, confidence, handVisible }`
- Update every frame from HandTracker callback
- Derived: `currentGesture`, `handVisible`

**T5.5 — Gesture → action wiring**  
File: `apps/desktop/frontend/src/routes/+page.svelte`
- Subscribe to `currentGesture`
- `GRAB` confirmed → `invoke('pick_file')` → file selected → attach orb
- `OPEN_PALM` confirmed (file selected) → `invoke('send_file', {...})`
- `OPEN_PALM` confirmed (receiving) → `invoke('accept_transfer', {...})`

**T5.6 — Gesture accuracy testing**
- Test in: bright light, dim light, backlit, dark skin, fair skin, with glasses
- Target: < 2% false positive rate (GRAB/OPEN triggering when not intended)
- Tune `GRAB_THRESHOLD` and `OPEN_THRESHOLD` constants as needed

### Verification
```bash
# Manual test script:
# 1. Close fist — file picker opens (must hold 267ms+)
# 2. Select file — orb appears on palm
# 3. Open palm — transfer initiates
# 4. Random hand movements should NOT trigger (false positive test)
# Log false positive count over 5 minute session
```

### Agent Instructions
> "Implement the MediaPipe hand tracking and gesture classification system in TypeScript. Create `HandTracker.ts` that initializes `HandLandmarker` from `@mediapipe/tasks-vision` with GPU delegate enabled and falls back to CPU automatically. Start the camera via `getUserMedia` and run `detectForVideo` in a `requestAnimationFrame` loop. Create `GestureClassifier.ts` that receives the 21 normalized landmarks and classifies: GRAB (all fingertip-to-wrist distances < 0.065), OPEN_PALM (all > 0.13), POINT (only index extended). Include a debouncer that requires 8 consecutive frames of the same gesture before confirming it — expose a `confidence` value from 0 to 1."

---

## Phase 6 — Phantom UI (Days 22-28)

### Context
Gestures work. No visual feedback on gestures yet.

### Goal
Phantom hand skeleton and file orb animations fully implemented. Color-coded gesture states. Transfer animation plays on send.

### Tasks

**T6.1 — Phantom hand renderer**  
File: `apps/desktop/frontend/src/lib/ui/PhantomHand.ts`
- Three.js scene with orthographic camera (maps to normalized 0-1 coords)
- WebGLRenderer on transparent canvas overlaid on camera feed
- 20 bone connections (LineBasicMaterial, opacity 0.75)
- 21 joint spheres (MeshBasicMaterial, opacity 0.9)
- Palm glow: large CircleGeometry behind wrist with blur effect
- Color: cyan=IDLE, orange=GRAB, green=OPEN_PALM, gold=POINT
- Resize-aware: call `renderer.setSize` on window resize

**T6.2 — File orb**  
File: `apps/desktop/frontend/src/lib/ui/FileOrb.ts`
- Core sphere + outer glow shell (BackSide material)
- States: `hidden → attached → sending → complete`
- `attached`: lerp position toward palm centroid (wrist landmark) each frame
- `sending`: lerp toward `(1.1, -0.1)` — flies off screen top-right
- `complete`: scale burst animation, then `hidden` after 1.5 seconds
- Pulse: `Math.sin(Date.now() * 0.003)` scale oscillation when attached

**T6.3 — Canvas layering**  
File: `apps/desktop/frontend/src/routes/+page.svelte`
- Camera `<video>` at 60% opacity (mirrored)
- PhantomHand `<canvas>` absolutely positioned, pointer-events none
- FileOrb `<canvas>` absolutely positioned above phantom
- UI `<div>` on top of all canvases

**T6.4 — Transfer ring animation**  
File: `apps/desktop/frontend/src/lib/ui/TransferRing.svelte`
- SVG ring with `stroke-dashoffset` animated on progress update
- Gradient stroke: teal → violet
- Center: percent text + state label
- Stats grid: MB/s, MB done, ETA

**T6.5 — Glassmorphism design pass**  
File: `apps/desktop/frontend/src/routes/+page.svelte`
- All panels: `backdrop-filter: blur(32px)`, translucent background
- Border: `1px solid rgba(255,255,255,0.08)`
- Subtle inner gradient highlight: `linear-gradient(135deg, rgba(255,255,255,0.06), transparent)`
- Button hover: lift transform + glow shadow

**T6.6 — Mini-Mode**  
File: `apps/desktop/frontend/src/lib/ui/MiniMode.svelte`
- Compact 280×80px window showing: peer name, transfer status, MB/s
- Toggle via keyboard shortcut (Cmd/Ctrl+Shift+M)
- Always-on-top via Tauri `window.setAlwaysOnTop(true)`
- Drag to reposition

### Verification
```bash
# Visual check:
# 1. Hand visible: skeleton draws at 30fps, no lag
# 2. Close fist: skeleton turns orange, glow pulses
# 3. Select file: orb appears on palm, follows hand
# 4. Open palm: orb flies off screen, transfer ring shows progress
# 5. Glassmorphism: panels have visible blur and translucency
# 6. Mini-Mode: toggles and stays on top
```

### Agent Instructions
> "Implement the Three.js phantom hand overlay in TypeScript. Create a `PhantomHand` class with an orthographic camera mapping [0,1] space to the full canvas. Draw the 20 anatomical bone connections as `THREE.Line` objects using the MediaPipe landmark indices, and place `THREE.Mesh` spheres at each of the 21 joints. Change the material color based on the current gesture: `0x00d4ff` for IDLE, `0xff6b35` for GRAB, `0x39ff14` for OPEN_PALM. Add a large translucent circle behind the wrist landmark as a palm glow effect that intensifies during GRAB. The renderer should use `alpha: true` and `setClearColor(0x000000, 0)` so the camera feed shows through."

---

## Phase 7 — Clipboard + Polish (Days 29-33)

### Context
Full gesture + transfer + UI working. Clipboard not implemented. Known UX rough edges.

### Goal
Universal clipboard sync working. All error states handled. App ready for user testing.

### Tasks

**T7.1 — Go: WebSocket server**  
File: `sidecar/server/websocket.go`
- Upgrade HTTP to WebSocket using `nhooyr.io/websocket`
- Store connection per session
- `PushToClient(sessionToken, msgType, payload)` — send JSON to browser
- Used for: clipboard push from desktop, incoming file notification

**T7.2 — Go: Clipboard endpoint**  
File: `sidecar/clipboard/sync.go`
- `POST /api/clipboard/push` — receive encrypted clipboard from browser, decrypt, write to system clipboard
- Go system clipboard: `golang.design/x/clipboard` package
- On desktop clipboard change: detect, encrypt with session key, push via WebSocket

**T7.3 — Browser: Clipboard UI**  
File: `browser-client/index.html`
- Already implemented — wire to real WebSocket
- Test two-way clipboard sync

**T7.4 — Error handling audit**
- Connection lost during transfer: show error, offer retry
- HMAC verification fails: show "Integrity check failed, file rejected"
- Camera permission denied: show clear instruction
- mDNS blocked (enterprise WiFi): show fallback instructions for text code
- File collision: auto-rename with timestamp suffix

**T7.5 — Portable binary + installer**
- Tauri builds: `.exe` installer for Windows, `.dmg` for macOS, `.AppImage` for Linux
- Also produce portable `.exe` (no installer) for Windows
- Update `scripts/build.sh` with cross-platform targets
- GitHub Actions CI: build on all three OSes

**T7.6 — Final performance validation**
- Benchmark 1GB transfer desktop-to-desktop: target ≥ 40 MB/s
- Benchmark 100MB transfer phone-to-desktop: target ≥ 20 MB/s
- Profile MediaPipe: confirm < 33ms per frame on target hardware
- Memory leak check: run for 1 hour, confirm no growth

### Verification
```bash
# User testing with 3 non-technical users:
# 1. Connect phone in < 5 seconds (QR)
# 2. Send a photo from phone to desktop
# 3. Send a file from desktop to phone
# 4. Sync a URL from desktop clipboard to phone
# 5. All succeed on first attempt without instructions
```

---

## Phase 8 — MVP Ship (Days 34-40)

### Context
All features working. Needs final hardening, documentation, and packaging.

### Goal
Signed, notarized binaries on all three platforms. README that lets a developer set up in < 10 minutes.

### Tasks

**T8.1 — Security audit**
- Review all IPC message handlers: validate all inputs, no path traversal on filenames
- Review all HTTPS handlers: auth check on every protected endpoint
- Packet capture test: zero plaintext bytes in any transfer
- HMAC bypass attempt: flip bytes in upload, confirm rejection

**T8.2 — Code cleanup**
- Remove all `// TODO` stubs (implement or document as known limitation)
- Remove all `unwrap()` in Rust (replace with proper error handling)
- Remove all `log.Fatal` in Go hot paths (replace with error propagation)

**T8.3 — Documentation**
- `README.md`: quick start, feature list, screenshot
- `CONTRIBUTING.md`: dev setup, architecture overview link, PR process
- Inline code comments on all public functions

**T8.4 — Packaging**
```bash
./scripts/build.sh
# Produces:
# dist/GestureShare-0.1.0-setup.exe      (Windows installer)
# dist/GestureShare-0.1.0-portable.exe   (Windows portable)
# dist/GestureShare-0.1.0.dmg           (macOS)
# dist/GestureShare-0.1.0.AppImage       (Linux)
```

**T8.5 — Version 0.1.0 tag**
```bash
git tag v0.1.0
git push origin v0.1.0
```

---

## Post-MVP Roadmap

### Phase 9 — v1.0 (Weeks 9-14)

| Feature | Description |
|---------|-------------|
| Transfer resume | Checkpoint-based restart after disconnect |
| Folder transfer | Recursive directory with progress per-file |
| Multiple simultaneous | Queue multiple transfers to multiple peers |
| Peer trust memory | Remember trusted fingerprints across sessions |
| Transfer history | Persistent log with date, size, peer, direction |
| Mini-Mode gesture | Control transfers without switching windows |

### Phase 10 — v2.0

| Feature | Description |
|---------|-------------|
| WAN relay | Optional encrypted relay for cross-network transfers |
| Mobile native app | Flutter iOS/Android with gesture control |
| API mode | CLI / daemon for scripted transfers |
| Browser extension | Drag website images directly to GestureShare |

---

## Appendix: Agent Quick-Reference

### Starting a new Phase

When starting a new phase, give the agent this context block:

```
Project: GestureShare (Tauri + SvelteKit + Go sidecar)
Current phase: [PHASE NAME]
Completed: [list completed files]
Goal: [paste goal from this plan]
File to implement: [specific file path]
Dependencies already built: [list relevant existing modules]
```

### Running the project in dev

```bash
# Terminal 1: Start Go sidecar in watch mode
cd sidecar && air   # or: go run .

# Terminal 2: Start Tauri dev
cd apps/desktop && cargo tauri dev
# This also starts SvelteKit on :5173 automatically
```

### Test file generation

```bash
# 10MB test file
dd if=/dev/urandom of=/tmp/test10mb.bin bs=1M count=10

# 1GB test file
dd if=/dev/urandom of=/tmp/test1gb.bin bs=1M count=1024

# Verify integrity
sha256sum /tmp/test1gb.bin
sha256sum ~/Downloads/test1gb.bin
```

### Key file locations at end of each phase

| Phase | Key files added |
|-------|----------------|
| 0 | `sidecar/main.go`, `src/sidecar/mod.rs`, `src/main.rs` |
| 1 | `sidecar/mdns/discovery.go`, `src/lib/stores/peerStore.ts` |
| 2 | `sidecar/crypto/session.go`, `sidecar/server/https.go`, `browser-client/index.html` |
| 3 | `sidecar/crypto/ctr.go`, `sidecar/server/routes.go`, `src/lib/transfer/tauribridge.ts` |
| 4 | `sidecar/transfer/tcp_sender.go`, `sidecar/transfer/tcp_receiver.go` |
| 5 | `src/lib/gesture/HandTracker.ts`, `src/lib/gesture/GestureClassifier.ts` |
| 6 | `src/lib/ui/PhantomHand.ts`, `src/lib/ui/FileOrb.ts`, `src/routes/+page.svelte` |
| 7 | `sidecar/server/websocket.go`, `sidecar/clipboard/sync.go` |
| 8 | All docs, `scripts/build.sh`, packaged binaries |
