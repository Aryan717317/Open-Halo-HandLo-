# GestureShare — Architecture Decision Records (ADR)

**Version:** 1.0  
**Purpose:** Documents every major technical decision, the alternatives considered, and why we chose what we chose. An agent must read this before suggesting changes to any core technology choice — the rationale here explains constraints that aren't obvious from the code alone.

---

## ADR-001: Tauri over Electron for Desktop Shell

**Status:** Accepted  
**Date:** 2026-03-06

### Decision
Use Tauri (Rust) as the desktop shell instead of Electron.

### Context
The app needs a native desktop window that can:
- Access the local filesystem (file picker, save to Downloads)
- Spawn a child process (Go sidecar)
- Run a WebView with camera access
- Ship as a small, fast binary

### Options Considered

| Option | Bundle size | Memory | Native API access | Cold start |
|--------|------------|--------|------------------|-----------|
| Tauri | ~10 MB | ~30 MB | Full via commands | <1s |
| Electron | ~150 MB | ~100 MB | Full via Node | ~2s |
| Flutter Desktop | ~25 MB | ~50 MB | Plugin-based | <1s |

### Decision Rationale
Tauri wins on binary size (10MB vs 150MB) and memory usage. The Rust layer gives direct OS access for spawning the Go sidecar via `std::process::Command`. SvelteKit in the WebView gives us the same frontend stack we'd use anyway for the browser client — code sharing between desktop UI and browser client is real.

### Consequences
- Rust knowledge required for the Tauri command layer
- Tauri v1 (not v2) — migration to v2 is a known future task
- WebView rendering varies slightly across OSes (uses system WebView)

---

## ADR-002: Go Sidecar over Pure Rust Networking

**Status:** Accepted  
**Date:** 2026-03-06

### Decision
Implement all networking (mDNS, HTTPS server, TCP transfer) in a separate Go binary (sidecar), communicating with Rust via stdin/stdout IPC.

### Context
The app needs:
- mDNS discovery (`hashicorp/mdns`)
- WebRTC signaling (`pion/webrtc`)
- HTTPS server with TLS
- Raw TCP file streaming

### Options Considered

**Option A: Pure Rust (everything in Tauri)**
- `mdns-sd` crate for mDNS
- `webrtc-rs` for WebRTC
- `axum` + `rustls` for HTTPS

**Option B: Go sidecar**
- `hashicorp/mdns` — battle-tested, used in production at scale
- `pion/webrtc` — most mature WebRTC implementation in any language
- `net/http` + `crypto/tls` — Go stdlib, no deps

**Option C: Node.js sidecar**

### Decision Rationale
`pion/webrtc` in Go is far more mature than `webrtc-rs`. `hashicorp/mdns` has years of production use. Go's `crypto` stdlib is comprehensive and well-audited. The developer is already comfortable in both Go and Rust. The IPC overhead (stdin/stdout JSON) is negligible for the message volumes involved (<100 messages/minute for control, zero messages during file transfer which goes direct TCP).

### Consequences
- Two language toolchains (Go + Rust) to maintain
- IPC adds one serialization layer for control messages
- Binary distribution: must ship compiled Go binary for each target platform
- **Go sidecar binary naming convention for Tauri:** `sidecar-{arch}-{os}` matching the format Tauri expects in `src-tauri/binaries/`

---

## ADR-003: AES-256-CTR over AES-256-GCM for File Transfer Cipher

**Status:** Accepted  
**Date:** 2026-03-06

### Decision
Use AES-256-CTR as the stream cipher for file transfer encryption instead of AES-256-GCM.

### Context
The original scaffold used AES-256-GCM. The redesign switched to CTR. This was a deliberate performance decision.

> ⚠️ **KNOWN INCONSISTENCY:** `sidecar/crypto/aes.go` in the Phase 0 scaffold still uses AES-GCM. This file must be replaced with an AES-CTR implementation in Phase 2. The correct implementation is in `browser-client/crypto/browser_crypto.go`.

### Options Considered

| Cipher | Auth per chunk? | Throughput | Parallelizable | Notes |
|--------|----------------|-----------|----------------|-------|
| AES-256-GCM | Yes (16B tag) | ~800 MB/s | Partially | Auth tag pauses |
| AES-256-CTR | No | ~2 GB/s | Yes | Needs separate HMAC |
| ChaCha20-Poly1305 | Yes | ~1.2 GB/s | No | Good alternative |

### Decision Rationale
AES-CTR is a pure XOR stream cipher. No authentication overhead per chunk means throughput is limited only by AES block operations, which are hardware-accelerated (AES-NI) on modern CPUs. On a machine with AES-NI, AES-CTR hits ~2 GB/s — far beyond our 40-100 MB/s network target. Authentication is provided by HMAC-SHA256 computed over the full ciphertext — this is actually stronger than per-chunk GCM for our use case because it detects reordering attacks across chunks, not just within-chunk tampering.

### Consequences
- Authentication is deferred to HMAC verification at end of transfer — receiver holds undecrypted data in memory/disk until HMAC passes before writing plaintext
- Actually: receiver must buffer OR verify streaming HMAC per-chunk to avoid holding full encrypted file
- **Implementation note:** Use separate HMAC-SHA256 key (domain-separated via HKDF) from AES key
- **Correct implementation reference:** `browser-client/crypto/browser_crypto.go::EncryptCTR/DecryptCTR`
- **File to fix in Phase 2:** `sidecar/crypto/aes.go` — replace GCM with CTR

---

## ADR-004: P-521 over P-256 or Curve25519 for Key Exchange

**Status:** Accepted  
**Date:** 2026-03-06

### Decision
Use NIST P-521 curve for ECDH key exchange.

### Context
Key exchange needs to work identically in the Go sidecar (crypto/ecdh stdlib) and the browser (WebCrypto API). All three must use the same curve.

### Options Considered

| Curve | Security bits | WebCrypto support | Go stdlib | Notes |
|-------|--------------|-------------------|-----------|-------|
| P-256 | 128 | ✅ All browsers | ✅ | Standard, ubiquitous |
| P-384 | 192 | ✅ Most browsers | ✅ | |
| P-521 | 260 | ✅ Safari 15+, Chrome 90+ | ✅ | Highest standard security |
| Curve25519 | 128 | ❌ Not in WebCrypto | ✅ Go | Cannot use for browser |

### Decision Rationale
Curve25519 would be ideal but **it is not available in the WebCrypto API** (`crypto.subtle`). This is a hard browser constraint. Among NIST curves, P-521 offers the highest security margin. The performance difference between P-256 and P-521 is negligible for a key exchange that happens once per session (not per chunk). P-521 is supported in Safari iOS 15+, Chrome 90+, Firefox 90+ — all our targets.

### Consequences
- P-521 public key is 133 bytes (uncompressed) vs 65 bytes for P-256
- QR code payload is slightly larger — still well within QR capacity
- If a target browser doesn't support P-521, fall back to P-256 (detected at runtime)

---

## ADR-005: Raw TCP over WebRTC for Desktop-to-Desktop Transfer

**Status:** Accepted  
**Date:** 2026-03-06

### Decision
Use raw TCP sockets for desktop-to-desktop file transfer instead of WebRTC data channels.

### Context
Desktop-to-desktop transfers are always on the same LAN. There is no NAT traversal needed. WebRTC was designed for peer-to-peer across NATs and the internet — its complexity is unnecessary here.

### Options Considered

| Method | Max LAN speed | Protocol overhead | Complexity |
|--------|-------------|------------------|-----------|
| WebRTC data channels | ~20-30 MB/s | High (DTLS + ICE + SCTP) | High |
| Raw TCP | 40-100+ MB/s | Zero | Low |
| UDP with custom ARQ | ~60-80 MB/s | Low | Very high |
| QUIC | ~50-80 MB/s | Medium | Medium |

### Decision Rationale
WebRTC's DTLS + ICE + SCTP stack adds substantial overhead that caps throughput at ~20-30 MB/s in practice, even on gigabit LAN. Raw TCP with a 4MB socket buffer and 64KB chunk sizes reaches 40-100 MB/s — matching or exceeding AirDrop performance. QUIC would be a good alternative but adds complexity with no advantage on LAN.

WebRTC is still used for signaling (SDP exchange) in the desktop-to-desktop pairing flow. Only the actual data transfer uses raw TCP.

### Consequences  
- Sender must open a TCP listener and communicate the port to receiver (via existing IPC/API)
- Both devices must be on the same subnet (always true for LAN)
- Firewalls may block arbitrary TCP ports — document this, mDNS code already handles this gracefully

---

## ADR-006: Single HTML File for Browser Client (No Build Step)

**Status:** Accepted  
**Date:** 2026-03-06

### Decision
The browser client is a single self-contained `index.html` with no external dependencies, no build step, no framework.

### Context
The browser client is served by the Go HTTPS server to any phone that scans the QR code. It must:
- Load fast over LAN (< 1 second)
- Work on any modern mobile browser
- Require no internet (can't load from CDN)
- Have no install requirement

### Options Considered

**Option A: SvelteKit / React app (bundled)**
- Build step required
- ~100-500KB bundle
- Still loads fast over LAN

**Option B: Single HTML file, vanilla JS, WebCrypto only**
- No build step
- ~50KB total
- Works in any browser
- Served directly from Go as static bytes

**Option C: Progressive Web App (PWA)**
- Could be installed
- But defeats "no install" goal

### Decision Rationale
The single-file approach means the Go server can embed the HTML as a string literal (or serve it from a relative path) with zero build tooling. Updates to the browser client just change one file. The WebCrypto API provides everything needed for P-521 ECDH and AES-256-CTR natively — no crypto library needed. Vanilla JS with no framework is perfectly adequate for a 5-screen app.

### Consequences
- No TypeScript in browser client (type safety via JSDoc or manual discipline)
- No hot reload during browser client development
- Must embed in Go binary for production distribution

---

## ADR-007: Hash Fragment for Key Transport in QR Code

**Status:** Accepted  
**Date:** 2026-03-06

### Decision
The P-521 public key, TLS certificate fingerprint, and session ID are encoded in the **URL hash fragment** (`#key=...&fp=...&sid=...`), not in query parameters.

### Context
The QR code encodes a URL. The browser opens that URL to the Go HTTPS server. The question is: how do we pass the key material to the browser securely?

### Options Considered

**Option A: Query parameters**  
`https://192.168.1.10:47291/join?key=...&fp=...&sid=...`  
- **Fatal flaw:** Query parameters are sent to the server in the GET request. The server (or an attacker observing the connection before TLS) could see the key.

**Option B: Hash fragment (chosen)**  
`https://192.168.1.10:47291/join#key=...&fp=...&sid=...`  
- **Key property:** Hash fragments are NEVER sent to the server — the browser parses them entirely client-side. The server only sees `GET /join`.

**Option C: POST body after page load**  
- Awkward UX, requires JavaScript to receive QR URL as parameter

### Decision Rationale
Hash fragments are the only mechanism available in a browser URL where data is guaranteed to never be transmitted over the network. This is not a workaround — it's the correct use of the URL hash. Combined with TLS, it means even a passive observer who can see the HTTP request cannot learn the key material.

### Consequences
- Server cannot log or inspect key material (intentional)
- JavaScript must parse `window.location.hash` on load
- Hash fragment is NOT included in browser history (varies by browser — treat as implementation detail)
- If user refreshes the page, the hash is still there — session may or may not still be valid

---

## ADR-008: HKDF-SHA512 with Domain-Separated Outputs

**Status:** Accepted  
**Date:** 2026-03-06

### Decision
Use HKDF-SHA512 with separate `info` strings to derive the AES key and HMAC key from the same ECDH shared secret.

### Context
ECDH produces one shared secret. We need two keys: one for AES-CTR encryption, one for HMAC-SHA256 integrity.

### Derivation Specification
```
shared_secret = ECDH(our_private, their_public)   // P-521, 66 bytes

aes_key  = HKDF-SHA512(
    IKM  = shared_secret,
    Salt = session_id_bytes,       // 32 random bytes from QR
    Info = "gestureshare-v1-aes",  // ASCII
    L    = 32                      // 256-bit AES key
)

hmac_key = HKDF-SHA512(
    IKM  = shared_secret,
    Salt = session_id_bytes,
    Info = "gestureshare-v1-hmac", // DIFFERENT info string
    L    = 32                      // 256-bit HMAC key
)
```

### Decision Rationale
Domain separation via distinct `info` strings ensures the AES key and HMAC key are cryptographically independent — neither can be derived from the other even with knowledge of the shared secret. SHA-512 (not SHA-256) provides a wider internal state, reducing any theoretical correlation between derived keys.

### Consequences
- Both sides MUST use identical `info` strings — any difference produces mismatched keys and a failed session
- `info` strings are fixed in spec and must not be changed without a version bump
- The `session_id` as salt means each session produces unique keys even if the same device pair connects multiple times

---

## ADR-009: mDNS Service Name `_gestureshare._tcp`

**Status:** Accepted  
**Date:** 2026-03-06

### Decision
Register the mDNS service type as `_gestureshare._tcp` with port 47291.

### Rationale
- `_gestureshare` is a unique application identifier, avoiding conflicts with system services
- `.local` domain is the standard mDNS/Bonjour domain
- Port 47291 is in the private range (49152–65535 would be more standard, but 47291 is memorable and unlikely to conflict)
- If 47291 is taken, try 47292 and 47293 before failing (see ADR for error handling)

### Port Registry
| Port | Service |
|------|---------|
| 47291 | GestureShare HTTPS server (primary) |
| 47292 | GestureShare HTTPS server (fallback 1) |
| 47293 | GestureShare HTTPS server (fallback 2) |
| 47294 | GestureShare TCP transfer (dynamic allocation, used internally) |

---

## ADR-010: No Persistent Key Storage

**Status:** Accepted  
**Date:** 2026-03-06

### Decision
No cryptographic key material is ever written to disk. All keys exist only in memory for the duration of the session.

### Rationale
- Keys written to disk persist after the user believes the session ended
- Disk encryption (FileVault, BitLocker) doesn't help if the OS is running
- Memory-only keys are wiped on app exit, OS crash, or power loss
- The threat model assumes the LAN is hostile — a compromised local machine is out of scope, but disk persistence makes it worse

### Consequences
- Every app launch generates fresh keys (new P-521 keypair, new session ID)
- No "remember this device" feature in v1.0 (fingerprint memory is Phase 9)
- Browser client keys are non-extractable WebCrypto keys — cannot be read even by the browser's own JavaScript
- The session ID in the QR code is one-time use — re-scanning generates a new session

### What IS stored on disk (non-sensitive)
- Transfer history (filenames, sizes, timestamps — no file contents, no keys)
- App preferences (window size, mini-mode preference)
- Peer display names (not fingerprints — fingerprint memory is Phase 9)
