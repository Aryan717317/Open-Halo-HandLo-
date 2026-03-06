# GestureShare — Code Conventions & Agent Context

**Version:** 1.0  
**Purpose:** Rules an agent must follow when writing any code for this project. Covers naming, formatting, patterns, and what context to always carry between sessions.

---

## 1. The Most Important Thing to Know First

This project has **three separate codebases** in one repo that cannot share code directly:

| Codebase | Language | Location | Can import from |
|----------|----------|----------|----------------|
| Go sidecar | Go | `sidecar/` | Only Go packages in `sidecar/` |
| Rust/Tauri | Rust | `apps/desktop/src-tauri/` | Only Rust crates |
| SvelteKit frontend | TypeScript | `apps/desktop/frontend/` | Only npm packages + `$lib/` |
| Browser client | Vanilla JS | `browser-client/index.html` | Nothing (self-contained) |

Type definitions are **duplicated** across boundaries by design. If you change a type, you must update it in all three places. The canonical source of truth for shared types is `docs/API_CONTRACTS.md`.

---

## 2. Known Code Inconsistency to Fix in Phase 2

> ⚠️ **`sidecar/crypto/aes.go` uses AES-GCM but the entire system was redesigned to use AES-CTR.**

The file was generated in the Phase 0 scaffold before the security redesign. It must be **completely replaced** in Phase 2 with AES-CTR implementation. Do not build on top of `aes.go` as it stands.

The correct AES-CTR implementation to use as reference: `browser-client/crypto/browser_crypto.go`

---

## 3. Naming Conventions

### Go (sidecar/)
```
Packages:   lowercase, single word          → crypto, mdns, transfer, ipc
Files:      snake_case.go                   → tcp_sender.go, browser_crypto.go
Types:      PascalCase                      → BrowserSession, ChunkCipher
Functions:  camelCase (unexported), PascalCase (exported)
Constants:  SCREAMING_SNAKE for IPC types   → CmdDiscover, EvtPeerFound
Variables:  camelCase                       → sessionID, sharedBits
JSON tags:  snake_case                      → `json:"transfer_id"`
```

### Rust (src-tauri/)
```
Files:      snake_case.rs
Types:      PascalCase
Functions:  snake_case
Tauri commands: snake_case                  → send_file, start_discovery
Error type: String (simple, for now)
```

### TypeScript (frontend/)
```
Files:      PascalCase.ts for classes       → HandTracker.ts, GestureClassifier.ts
            camelCase.ts for stores/utils   → peerStore.ts, tauribridge.ts
            PascalCase.svelte for components → GlassPanel.svelte
Types/Interfaces: PascalCase               → PeerInfo, TransferOffer
Variables:  camelCase
Svelte stores: camelCase                   → peerStore, transferStore
Store values (derived): camelCase          → $peers, $hasPeers, $currentGesture
Tauri event names: SCREAMING_SNAKE         → EVT_PEER_FOUND, EVT_TX_PROGRESS
Tauri command names: snake_case            → 'start_discovery', 'send_file'
```

### Cross-boundary JSON field names
All JSON field names use **snake_case** everywhere:
```json
{ "transfer_id": "...", "peer_id": "...", "bytes_sent": 0, "speed_bps": 0 }
```
This is enforced by Go struct tags. TypeScript types must match exactly.

---

## 4. File Organization Rules

### Go: one responsibility per file
```
crypto/session.go   → keypair generation + session ID + TLS cert (ONLY these)
crypto/ecdh.go      → ECDH + HKDF key derivation (ONLY these)
crypto/ctr.go       → AES-256-CTR encrypt/decrypt (ONLY these)
crypto/hmac.go      → HMAC-SHA256 sign/verify (ONLY these)
```
Do NOT combine these into one file. Small focused files are easier for agents to reason about.

### TypeScript: co-locate related things
```
lib/gesture/HandTracker.ts         → camera + MediaPipe (no gesture logic)
lib/gesture/GestureClassifier.ts   → gesture logic + debouncer (no camera)
lib/gesture/gestureStore.ts        → Svelte store only (no logic)
```

### Svelte: keep pages thin
`routes/+page.svelte` should only:
1. Import from `$lib/`
2. Wire events to store updates
3. Call `transition()` for screen changes
4. Render `{#if screen === 'x'}` blocks

Logic belongs in `$lib/`, not in the page component.

---

## 5. Error Handling Patterns

### Go: always wrap errors with context
```go
// ✅ Correct
if err := doThing(); err != nil {
    return fmt.Errorf("doing thing for transfer %s: %w", transferID, err)
}

// ❌ Wrong — loses context
if err := doThing(); err != nil {
    return err
}
```

### Go: never use log.Fatal in library code
```go
// ✅ Only in main.go
log.Fatal("cannot start:", err)

// ✅ In library code: return error
func StartServer(port int) error {
    // ...
    return fmt.Errorf("bind port %d: %w", port, err)
}
```

### Go: emit IPC error before returning
```go
func handleUpload(...) {
    if !crypto.VerifyHMAC(...) {
        ipc.Emit(EvtTxError, ErrorPayload{Code: "HMAC_FAIL", ...})
        http.Error(w, "Integrity check failed", 400)
        return  // ← always return after emitting error
    }
}
```

### Rust: use String errors for Tauri commands
```rust
// ✅ Tauri commands return Result<T, String>
#[tauri::command]
pub async fn send_file(args: SendFileArgs) -> Result<(), String> {
    sidecar::send_command(...)
        .map_err(|e| e.to_string())
}
```

### TypeScript: never swallow errors silently
```typescript
// ✅ Log + rethrow or handle
try {
    await invoke('send_file', args);
} catch (e) {
    UI.log(`Send failed: ${e}`, 'err');
    transition('error', { errorDetails: { code: 'UNKNOWN', message: String(e) } });
}

// ❌ Never do this
try {
    await invoke('send_file', args);
} catch { /* silent */ }
```

---

## 6. Security Rules (Non-Negotiable)

An agent must never write code that violates these:

1. **No file bytes hit the disk before HMAC verification passes.** Buffer in memory or stream-verify.
2. **No plaintext file bytes transmitted over any network interface.** Encrypt before send.
3. **CERT_MISMATCH has no bypass.** The UI for this error has no dismiss button. Do not add one.
4. **Session ID must match before ECDH.** Reject mismatched session IDs with HTTP 403.
5. **Keys are never logged.** Do not log AES keys, HMAC keys, ECDH private keys, or session tokens.
6. **Filenames are sanitized before disk write.** Use `filepath.Base()` to strip any path components.
   ```go
   safeName := filepath.Base(untrustedName)  // strips ../../etc/passwd to passwd
   ```
7. **Session tokens are 32 random bytes minimum.** Not UUIDs — use `crypto/rand`.

---

## 7. IPC Communication Patterns

### Sending a command from Rust to Go
```rust
// In a Tauri command handler:
sidecar::send_command(&app, "CMD_SEND_FILE", serde_json::json!({
    "transfer_id": transfer_id,
    "peer_id": peer_id,
    "file_path": file_path,
    "file_name": file_name,
    "file_size": file_size,
    "mime_type": mime_type,
}));
```

### Listening for an event from Go in SvelteKit
```typescript
// In onMount() or a store initializer — not in reactive statements
import { listen } from '@tauri-apps/api/event';

onMount(async () => {
    const unlisten = await listen<ProgressEvent>('EVT_TX_PROGRESS', (event) => {
        transferStore.updateProgress(event.payload);
    });
    
    return () => unlisten();  // cleanup on destroy
});
```

### Emitting an event from Go to Rust
```go
// In any Go package — import ipc
ipc.Emit(ipc.EvtTxProgress, ipc.ProgressPayload{
    TransferID: id,
    BytesSent:  sent,
    TotalBytes: total,
    Percent:    float64(sent) / float64(total) * 100,
    SpeedBPS:   speedBPS,
    ETASeconds: eta,
})
```

---

## 8. Tauri Configuration Rules

The `tauri.conf.json` allowlist must be **minimal** — only enable what's actually used:

```json
"allowlist": {
    "all": false,
    "shell": { "open": false },  // No shell.open unless explicitly needed
    "dialog": {
        "open": true,            // File picker
        "save": false            // Not needed
    },
    "fs": {
        "readFile": false,       // Go handles file reading
        "writeFile": false,      // Go handles file writing  
        "readDir": false,
        "scope": ["$DOWNLOAD/*"] // Minimum scope
    },
    "path": { "all": true }
}
```

Never use `"all": true` — this grants all permissions to the WebView.

---

## 9. Agent Session Context Block

When starting a new coding session for this project, paste this block as context:

```
PROJECT: GestureShare
Stack: Tauri (Rust) + SvelteKit (TypeScript) + Go sidecar
Key docs: docs/API_CONTRACTS.md, docs/DECISIONS.md, docs/STATE_MACHINE.md

CRITICAL FACTS:
1. sidecar/crypto/aes.go uses AES-GCM but MUST be AES-CTR — fix in Phase 2
   Reference: browser-client/crypto/browser_crypto.go::EncryptCTR/DecryptCTR
2. All JSON field names are snake_case across all boundaries
3. The browser client (browser-client/index.html) is a single file, zero deps
4. P-521 is the only ECDH curve — Curve25519 is NOT available in WebCrypto
5. HKDF info strings are FIXED: "gestureshare-v1-aes" and "gestureshare-v1-hmac"
6. Hash fragment in QR URL is never sent to server (intentional security property)
7. AES key and HMAC key MUST be derived separately (domain separation)
8. Filename from browser must be sanitized with filepath.Base() before disk write
9. Screen state machine is defined in docs/STATE_MACHINE.md — follow it exactly
10. Camera turns OFF when screen is 'connect', 'pairing', 'done', or 'error'

CURRENT PHASE: [FILL IN BEFORE PASTING]
FILES ALREADY IMPLEMENTED: [FILL IN BEFORE PASTING]
TASK: [FILL IN BEFORE PASTING]
```

---

## 10. Dependency Version Lock

These versions are tested and known to work together. Do not upgrade without testing:

### Go
```
github.com/hashicorp/mdns    v1.0.5     # mDNS discovery
github.com/pion/webrtc/v3    v3.2.40    # WebRTC signaling only
golang.org/x/crypto          v0.21.0    # HKDF
nhooyr.io/websocket          v1.8.10    # WebSocket server
golang.design/x/clipboard    v0.7.0     # System clipboard
```

### Rust
```
tauri         = "1.6"    # NOT 2.x — migration is future work
serde         = "1.0"
serde_json    = "1.0"
tokio         = "1"
rcgen         = "0.12"
uuid          = "1.6"
```

### npm
```
@mediapipe/tasks-vision   ^0.10.9   # Hands WASM model
@tauri-apps/api           ^1.6.0    # Must match Tauri 1.6
three                     ^0.160.0
qrcode                    ^1.5.3
```

---

## 11. What the Agent Should NOT Change Without an ADR Update

These are locked decisions. If an agent is tempted to change them, it must first update DECISIONS.md with a new ADR:

- The cryptographic algorithms (P-521, AES-256-CTR, HMAC-SHA256, HKDF-SHA512)
- The HKDF info strings
- The IPC protocol format (JSON newline-delimited)
- The hash fragment approach for QR key transport
- The single-file browser client approach
- The Go sidecar as a separate process (not embedded in Rust)
- The mDNS service name and port
