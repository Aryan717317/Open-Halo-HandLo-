# GestureShare — System Architecture

**Version:** 1.0  
**Status:** Active  
**Last Updated:** 2026-03-06

---

## 1. System Overview

GestureShare is a three-process application with a browser-based universal client.

```
┌──────────────────────────────────────────────────────────────────────┐
│                        GESTURESHARE DESKTOP                          │
│                                                                      │
│  ┌─────────────────────────────┐   ┌──────────────────────────────┐ │
│  │    PROCESS 1: Tauri Shell   │   │    PROCESS 2: Go Sidecar     │ │
│  │    (Rust + WebView)         │   │    (Networking Core)         │ │
│  │                             │   │                              │ │
│  │  SvelteKit Frontend         │   │  HTTPS Server (TLS 1.3)      │ │
│  │  ├── Gesture Engine         │   │  mDNS Discovery              │ │
│  │  │   ├── HandTracker.ts     │   │  WebRTC / Raw TCP Transfer   │ │
│  │  │   └── GestureClassifier  │   │  P-521 ECDH Crypto           │ │
│  │  ├── Three.js Overlay       │   │  HMAC-SHA256 Integrity       │ │
│  │  │   ├── PhantomHand.ts     │   │  Clipboard Sync API          │ │
│  │  │   └── FileOrb.ts         │   │  WebSocket Push              │ │
│  │  ├── Svelte Stores          │   │                              │ │
│  │  └── Tauri Bridge           │   │                              │ │
│  │      (invoke / listen)      │   │                              │ │
│  └──────────────┬──────────────┘   └──────────────┬───────────────┘ │
│                 │  Tauri IPC                       │                 │
│                 │  (Rust commands)                 │                 │
│  ┌──────────────▼──────────────────────────────────▼───────────────┐ │
│  │                    PROCESS 1b: Rust Core                        │ │
│  │   File system access · Sidecar spawn · Command routing          │ │
│  └─────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────┬───────────────────────────────────┘
                                   │
                    Local Network (LAN / WiFi)
                    Assumed hostile — all traffic E2EE
                                   │
          ┌────────────────────────┼────────────────────────┐
          │                        │                        │
┌─────────▼──────────┐  ┌──────────▼──────────┐  ┌────────▼──────────┐
│   Phone / Tablet   │  │   Another Desktop   │  │   Any Browser     │
│   (Browser only)   │  │   (Tauri app)       │  │   (Linux, etc.)   │
│                    │  │                     │  │                   │
│  index.html        │  │  Same architecture  │  │  index.html       │
│  WebCrypto API     │  │  as above           │  │  WebCrypto API    │
│  P-521 ECDH        │  │  Raw TCP transfer   │  │  P-521 ECDH       │
│  AES-256-CTR       │  │  40-100 MB/s        │  │  AES-256-CTR      │
│  No install        │  │                     │  │  No install       │
└────────────────────┘  └─────────────────────┘  └───────────────────┘
```

---

## 2. Process Architecture

### 2.1 Process 1: Tauri Shell (Rust + SvelteKit WebView)

The user-facing desktop process. Tauri embeds a WebView that runs SvelteKit. Rust handles OS-level operations.

**Rust layer responsibilities:**
- Spawn and manage Go sidecar process lifecycle
- Read Go sidecar stdout line-by-line (JSON events) → emit to SvelteKit via `window.emit()`
- Write commands to Go sidecar stdin (JSON)
- File system: open file picker, read file bytes, write received files
- System tray integration
- Window management (normal mode ↔ Mini-Mode)

**SvelteKit (WebView) layer responsibilities:**
- Camera access via `getUserMedia`
- MediaPipe Hands WASM — 21-landmark hand tracking at 30fps
- Gesture classification and debouncing
- Three.js phantom hand skeleton + file orb animation
- Reactive UI (Svelte stores for peer list, transfer state, gesture state)
- Tauri `invoke()` calls — bridge to Rust commands

**IPC: SvelteKit ↔ Rust**
```
SvelteKit → Rust:  invoke('send_file', { filePath, peerId })
Rust → SvelteKit:  window.emit('EVT_TX_PROGRESS', { percent, speedMBs })
```

### 2.2 Process 2: Go Sidecar (Networking Core)

A compiled Go binary shipped with the Tauri app. Spawned by Rust on startup. Owns all networking.

**Responsibilities:**
- mDNS: broadcast own presence, scan for peers every 3 seconds
- HTTPS server on port 47291: serves browser client, handles pairing and file upload APIs
- P-521 keypair + session ID + self-signed TLS cert generation on startup
- ECDH key exchange with browser clients
- AES-256-CTR encryption/decryption
- HMAC-SHA256 signing and verification
- Raw TCP sender/receiver for desktop-to-desktop transfers
- WebSocket server for push notifications (clipboard, incoming file alerts)
- Clipboard sync endpoint

**IPC: Go ↔ Rust**
```
Protocol:    Newline-delimited JSON over stdin/stdout
Rust → Go:   {"type":"CMD_SEND_FILE","payload":{...}}
Go → Rust:   {"type":"EVT_TX_PROGRESS","payload":{...}}
```

Full message type catalogue:

| Direction | Type | Meaning |
|-----------|------|---------|
| Rust → Go | `CMD_DISCOVER` | Start mDNS scanning |
| Rust → Go | `CMD_STOP_DISCOVER` | Stop scanning |
| Rust → Go | `CMD_PAIR_REQUEST` | Initiate pairing with peer |
| Rust → Go | `CMD_SEND_FILE` | Send file to peer |
| Rust → Go | `CMD_CANCEL_TX` | Cancel active transfer |
| Go → Rust | `EVT_PEER_FOUND` | New peer discovered on LAN |
| Go → Rust | `EVT_PEER_LOST` | Peer no longer visible |
| Go → Rust | `EVT_PAIR_SUCCESS` | Session established |
| Go → Rust | `EVT_TX_OFFER` | Incoming file from browser |
| Go → Rust | `EVT_TX_PROGRESS` | Transfer progress update |
| Go → Rust | `EVT_TX_COMPLETE` | Transfer finished |
| Go → Rust | `EVT_TX_ERROR` | Transfer failed |
| Go → Rust | `EVT_CLIPBOARD_RX` | Clipboard received from phone |

### 2.3 Browser Client (index.html)

A single self-contained HTML file served by the Go HTTPS server. Zero external dependencies. Runs on any modern browser without installation.

**Responsibilities:**
- Parse QR hash fragment client-side (key, cert fingerprint, session ID)
- Verify TLS cert fingerprint via `/api/cert-ping`
- Generate P-521 keypair via WebCrypto (non-extractable)
- Complete ECDH session registration via POST
- Encrypt files using AES-256-CTR before upload
- Sign transfers with HMAC-SHA256
- Upload encrypted payload via XHR with progress events
- WebSocket connection for receiving push notifications
- Clipboard sync (send and receive)
- Wipe keys on page unload

---

## 3. Data Flow Diagrams

### 3.1 Zero-Trust Handshake

```
Desktop (Go)                    Network              Browser (Phone)
────────────                    ───────              ───────────────
Generate P-521 keypair
Generate session ID (32B random)
Generate self-signed TLS cert
Compute cert fingerprint (SHA-256)
Build QR URL:
  https://192.168.1.x:47291/join
  #key=<pub_b64>
  &fp=<cert_sha256>
  &sid=<session_id>

Display QR ──────────────────── [optical] ──────────► Scan QR
                                                       Parse hash fragment
                                                       (never sent over net)

                                ◄── HTTPS GET /join ──
Serve index.html ───────────────────────────────────►
                                                       Load browser client
                                                       Fetch /api/cert-ping
                                ◄── GET /cert-ping ───
Return {fingerprint} ──────────────────────────────►
                                                       Compare fp to QR value
                                                       ✓ Match → continue
                                                       ✗ Mismatch → ABORT

                                                       Generate P-521 keypair
                                                       POST /session/register
                                                         {browserPubKey, sid}
                                ◄── POST /register ───
Decode browserPubKey
ECDH(desktopPriv × browserPub)
HKDF → AES-256-CTR key
HKDF → HMAC key
Store in session map
Return {token, desktopName} ───────────────────────►
                                                       ECDH(browserPriv × desktopPub)
                                                       HKDF → same AES-256-CTR key
                                                       HKDF → same HMAC key
                                                       ✓ E2EE session active
```

### 3.2 Phone → Desktop File Transfer

```
Browser (Phone)                                        Desktop (Go)
───────────────                                        ────────────
User selects file
Read as ArrayBuffer
Encrypt chunks (AES-256-CTR):
  for each 1MB chunk:
    counter = random 16 bytes
    encrypted = AES-CTR(chunk, key, counter)
    packet = [counter || encrypted]
Compute HMAC-SHA256(full_ciphertext)

POST /api/transfer/upload
  Header: X-GS-Token: <session-token>
  Header: X-GS-FileName: photo.jpg
  Header: X-GS-HMAC: <hmac_b64>
  Body: <full encrypted payload>
──────────────────────────────────────────────────────►
                                                        Auth session via token
                                                        Read body bytes
                                                        Verify HMAC-SHA256
                                                          ✓ → proceed
                                                          ✗ → 400 reject
                                                        DecryptCTR(payload, key)
                                                        Write to ~/Downloads/photo.jpg
                                                        Emit EVT_TX_COMPLETE to Rust
                                                        Return 200 {status: "ok"}
◄──────────────────────────────────────────────────────
Show "Transfer complete"
                                                        Rust emits to SvelteKit
                                                        Desktop shows notification
```

### 3.3 Desktop → Desktop (Raw TCP)

```
Desktop A (Sender)                                     Desktop B (Receiver)
──────────────────                                     ──────────────────────
Select file via gesture/UI

Create TCP listener on random port
Broadcast port via signaling ──────────────────────►
                                                        Connect to TCP address

Derive AES-256-CTR key (ECDH shared)                   Derive same key (ECDH shared)

Open file → read stream
for each 64KB chunk:
  counter = random 16 bytes
  encrypted = AES-CTR(chunk, key, counter)
  write [counter || len(encrypted) || encrypted]
──────────── raw TCP socket ───────────────────────►
                                                        Read packet
                                                        Extract counter + ciphertext
                                                        DecryptCTR → plaintext chunk
                                                        Append to output file

Compute HMAC → send as EOF marker ─────────────────►
                                                        Verify HMAC over full file
                                                        ✓ keep file
                                                        ✗ delete file, notify error
```

---

## 4. Security Architecture

### 4.1 Cryptographic Primitives

| Primitive | Algorithm | Parameters | Purpose |
|-----------|-----------|------------|---------|
| Key exchange | ECDH | P-521 curve | Establish shared secret without transmitting it |
| Key derivation | HKDF | SHA-512, 32-byte output | Derive AES and HMAC keys from shared secret |
| Encryption | AES-CTR | 256-bit key, 128-bit counter | File encryption (stream cipher, max throughput) |
| Integrity | HMAC | SHA-256, 32-byte key | Detect tampering of encrypted payload |
| Transport | TLS | 1.3 only, ECDSA cert | Encrypt HTTP signaling channel |
| Cert auth | SHA-256 | fingerprint in QR | Pin server certificate via optical channel |
| Randomness | CSPRNG | OS entropy | All key generation, nonces, session IDs |

### 4.2 Trust Model

```
WHAT WE TRUST:
  ✓ The optical channel (QR code scan) — attacker cannot intercept light
  ✓ The device that generated the QR — verified by cert fingerprint
  ✓ The WebCrypto subsystem — browser secure context, non-extractable keys

WHAT WE DO NOT TRUST:
  ✗ The local network — assume ARP spoofing, packet capture, MITM
  ✗ DNS/mDNS responses — only used for peer discovery, not security
  ✗ The TLS certificate chain — no CA, TOFU pinned via QR instead
  ✗ Disk storage — no key material ever written to disk
  ✗ Memory after session — keys explicitly wiped on exit
```

### 4.3 Forward Secrecy

- New P-521 keypair generated on every application launch
- New session ID (32 random bytes) per launch
- Compromising stored data from a previous session reveals nothing
- Separate HKDF derivation for AES key vs HMAC key (domain separation)

### 4.4 AES-CTR vs AES-GCM

AES-CTR is chosen over GCM for the transfer cipher for one reason: throughput.

GCM computes an authentication tag per encryption call, which introduces a pause and limits parallelism. CTR is a pure XOR stream cipher — no authentication overhead. At 40-100 MB/s file transfer speeds, this matters. Authentication is provided by HMAC-SHA256 over the full ciphertext, which is computed once, not per-chunk.

---

## 5. Module Dependency Map

```
SvelteKit Frontend
  ├── HandTracker.ts          (depends on: @mediapipe/tasks-vision)
  ├── GestureClassifier.ts    (depends on: HandTracker types)
  ├── gestureStore.ts         (depends on: GestureClassifier)
  ├── PhantomHand.ts          (depends on: three, gestureStore)
  ├── FileOrb.ts              (depends on: three)
  ├── tauribridge.ts          (depends on: @tauri-apps/api)
  ├── peerStore.ts            (depends on: tauribridge)
  ├── transferStore.ts        (depends on: tauribridge)
  └── +page.svelte            (depends on: all above)

Rust / Tauri
  ├── main.rs                 (depends on: sidecar, commands)
  ├── sidecar/mod.rs          (depends on: std::process)
  ├── commands/file.rs        (depends on: tauri dialog, fs)
  ├── commands/transfer.rs    (depends on: sidecar)
  └── commands/device.rs      (standalone)

Go Sidecar
  ├── main.go                 (depends on: all packages)
  ├── ipc/protocol.go         (standalone)
  ├── ipc/router.go           (depends on: mdns, webrtc, crypto, transfer)
  ├── mdns/discovery.go       (depends on: hashicorp/mdns)
  ├── server/https.go         (depends on: crypto, transfer)
  ├── server/routes.go        (depends on: crypto)
  ├── crypto/session.go       (depends on: crypto/ecdh, rcgen equivalent)
  ├── crypto/ctr.go           (depends on: crypto/aes, crypto/cipher)
  ├── crypto/ecdh.go          (depends on: crypto/ecdh, x/crypto/hkdf)
  ├── crypto/hmac.go          (depends on: crypto/hmac, crypto/sha256)
  ├── transfer/tcp_sender.go  (depends on: crypto)
  ├── transfer/tcp_receiver.go(depends on: crypto)
  └── clipboard/sync.go       (standalone)

Browser Client (index.html)
  ├── Crypto module           (depends on: window.crypto.subtle)
  ├── Session module          (depends on: Crypto, fetch)
  ├── Transfer module         (depends on: Crypto, Session, XHR)
  └── UI module               (depends on: Transfer, Session, Canvas)
```

---

## 6. Directory Structure

```
gestureshare/
│
├── README.md
│
├── apps/
│   └── desktop/
│       ├── src-tauri/                    # Rust / Tauri
│       │   ├── Cargo.toml
│       │   ├── tauri.conf.json
│       │   └── src/
│       │       ├── main.rs
│       │       ├── sidecar/mod.rs        # Go sidecar spawner
│       │       └── commands/
│       │           ├── mod.rs
│       │           ├── file.rs           # File system ops
│       │           ├── transfer.rs       # Bridge to Go
│       │           └── device.rs         # Device info
│       │
│       └── frontend/                     # SvelteKit
│           ├── package.json
│           ├── svelte.config.js
│           ├── vite.config.ts
│           └── src/
│               ├── app.html
│               ├── routes/+page.svelte
│               └── lib/
│                   ├── gesture/
│                   │   ├── HandTracker.ts
│                   │   ├── GestureClassifier.ts
│                   │   └── gestureStore.ts
│                   ├── ui/
│                   │   ├── PhantomHand.ts
│                   │   ├── FileOrb.ts
│                   │   ├── MiniMode.svelte
│                   │   └── GlassPanel.svelte
│                   ├── transfer/
│                   │   └── tauribridge.ts
│                   ├── stores/
│                   │   ├── peerStore.ts
│                   │   └── transferStore.ts
│                   └── types.ts
│
├── sidecar/                              # Go networking core
│   ├── go.mod
│   ├── main.go
│   ├── ipc/
│   │   ├── protocol.go                  # Message type definitions
│   │   └── router.go                    # Command dispatch
│   ├── server/
│   │   ├── https.go                     # TLS 1.3 HTTPS server
│   │   ├── routes.go                    # REST API handlers
│   │   ├── websocket.go                 # WS push notifications
│   │   └── static.go                    # Serve browser client HTML
│   ├── mdns/
│   │   └── discovery.go                 # mDNS broadcast + scan
│   ├── crypto/
│   │   ├── session.go                   # P-521 + session ID + TLS cert
│   │   ├── ecdh.go                      # ECDH + HKDF-SHA512
│   │   ├── ctr.go                       # AES-256-CTR encrypt/decrypt
│   │   └── hmac.go                      # HMAC-SHA256 sign/verify
│   ├── transfer/
│   │   ├── tcp_sender.go                # Raw TCP encrypted file send
│   │   ├── tcp_receiver.go              # Raw TCP encrypted file receive
│   │   └── progress.go                  # Speed + ETA tracking
│   └── clipboard/
│       └── sync.go                      # Clipboard push/pull API
│
├── browser-client/
│   ├── index.html                       # Complete browser client (single file)
│   ├── server_companion.go              # Go handler that serves index.html
│   └── crypto/
│       └── browser_crypto.go            # Server-side mirror of WebCrypto ops
│
├── docs/
│   ├── PRD.md
│   ├── MVP.md
│   ├── ARCHITECTURE.md                  # This file
│   └── IMPLEMENTATION_PLAN.md
│
└── scripts/
    ├── setup.sh
    ├── dev.sh
    └── build.sh
```

---

## 7. API Reference

### 7.1 REST API (Go HTTPS Server)

All endpoints require `X-GS-Token` header except `/api/cert-ping` and `/join`.

```
GET  /join                      Serve browser client HTML
GET  /api/cert-ping             Return TLS cert SHA-256 fingerprint
POST /api/session/register      Register browser session (ECDH pubkey exchange)
POST /api/transfer/upload       Receive encrypted file from browser
POST /api/clipboard/push        Receive encrypted clipboard text from browser
WS   /ws?sid=<session_id>       WebSocket for push notifications to browser
```

### 7.2 Tauri Commands (Rust)

```rust
pick_file()                     → Option<String>  // native file picker
get_downloads_dir()             → String
get_device_info()               → DeviceInfo
start_discovery()               → ()
stop_discovery()                → ()
pair_request(args)              → ()
pair_accept(peer_id)            → ()
send_file(args)                 → ()
cancel_transfer(transfer_id)    → ()
```

### 7.3 Tauri Events (Go → Rust → SvelteKit)

```
EVT_PEER_FOUND      { id, name, address, port, os }
EVT_PEER_LOST       { id }
EVT_PAIR_INCOMING   { peer_id, peer_name, public_key }
EVT_PAIR_SUCCESS    { peer_id }
EVT_TX_OFFER        { transfer_id, peer_id, file_name, file_size }
EVT_TX_PROGRESS     { transfer_id, bytes_sent, total_bytes, percent, speed_bps }
EVT_TX_COMPLETE     { transfer_id }
EVT_TX_ERROR        { transfer_id, code, message }
EVT_CLIPBOARD_RX    { text }
```

---

## 8. Performance Design

### 8.1 Transfer Speed Targets

| Transfer Mode | Target | Bottleneck |
|---------------|--------|-----------|
| Desktop → Desktop (raw TCP) | 40-100 MB/s | AES-CTR throughput, TCP buffer |
| Desktop → Phone (HTTPS) | 20-60 MB/s | HTTPS overhead, WiFi bandwidth |
| Phone → Desktop (HTTPS upload) | 15-40 MB/s | WebCrypto encrypt speed, upload |

### 8.2 Throughput Optimizations

- **AES-256-CTR** over GCM: no per-chunk auth tag pauses — 2-3x faster for large files
- **Raw TCP** over WebRTC: no DTLS + ICE + SCTP overhead — ~3x faster on LAN
- **64KB chunk size** for TCP desktop transfer: matches typical kernel socket buffer
- **1MB chunk size** for browser XHR: amortizes WebCrypto call overhead
- **Zero-copy reads**: `os.File` → direct write to TCP socket where possible
- **Parallel HMAC**: computed concurrently with TCP send, not blocking

### 8.3 Gesture Performance

- MediaPipe Hands: ~8ms inference on modern CPU, ~3ms with GPU delegate
- Three.js render: < 2ms per frame (21 line segments + spheres)
- Gesture store updates: Svelte reactive — only rerenders affected components
- Debouncer: 8 frame hold before confirmation — prevents thrashing without perceptible delay
