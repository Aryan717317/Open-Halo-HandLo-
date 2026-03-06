# GestureShare — Agent Rules & Coding Conventions

**Read this before writing any code. These rules are non-negotiable.**  
**Version:** 1.0 | **Scope:** All files in this repository

---

## 0. The Prime Directive

Every piece of code you write must satisfy one test:  
**"Could another agent pick this up tomorrow and know exactly what it does and why?"**

If not, rewrite it.

---

## 1. How to Start Every Task

Before writing code for any task:

1. Re-read the relevant section of `ARCHITECTURE.md`
2. Re-read the current Phase in `IMPLEMENTATION_PLAN.md`
3. Check `STATE_MACHINES.md` if your task touches any screen, transfer, or session state
4. Check `WIRE_PROTOCOL.md` if your task touches TCP packets or IPC messages
5. Check `PLATFORM_GUIDE.md` if your task touches: file paths, mDNS, sockets, TLS, camera, clipboard, or builds

Only then open an editor.

---

## 2. Language Rules

### 2.1 Go (sidecar/)

**Error handling — always wrap with context:**
```go
// WRONG
file, err := os.Open(path)
if err != nil { return err }

// CORRECT
file, err := os.Open(path)
if err != nil { return fmt.Errorf("open transfer file %q: %w", path, err) }
```

**Never use log.Fatal outside of main():**
```go
// WRONG — kills the whole process from inside a handler
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
    data, err := io.ReadAll(r.Body)
    if err != nil { log.Fatal(err) }  // ← NEVER
}

// CORRECT — return error to caller, log at appropriate level
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
    data, err := io.ReadAll(r.Body)
    if err != nil {
        log.Printf("[upload] read body: %v", err)
        http.Error(w, "read error", 500)
        return
    }
}
```

**Always use structured log prefixes:**
```go
log.Printf("[mdns] peer found: %s @ %s", peer.Name, peer.Address)
log.Printf("[crypto] ECDH key derived for session %s", sid[:8])
log.Printf("[transfer] sent %d/%d bytes to %s", sent, total, peerID)
```
Prefixes: `[mdns]`, `[server]`, `[crypto]`, `[transfer]`, `[ipc]`, `[clipboard]`, `[ws]`

**Goroutine ownership — every goroutine must have a stop mechanism:**
```go
// WRONG — goroutine leaks on shutdown
go func() {
    for { scan() }
}()

// CORRECT — select on stopCh
go func() {
    ticker := time.NewTicker(3 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-s.stopCh:
            return
        case <-ticker.C:
            s.scan()
        }
    }
}()
```

**Mutexes — always use RWMutex for read-heavy state:**
```go
type Store struct {
    mu   sync.RWMutex
    data map[string]*Item
}

func (s *Store) Get(key string) *Item {
    s.mu.RLock()          // read lock
    defer s.mu.RUnlock()
    return s.data[key]
}

func (s *Store) Set(key string, item *Item) {
    s.mu.Lock()           // write lock
    defer s.mu.Unlock()
    s.data[key] = item
}
```

**No naked panics in production code:**
```go
// WRONG
func mustParseKey(b []byte) *ecdh.PublicKey {
    k, err := ecdh.P521().NewPublicKey(b)
    if err != nil { panic(err) }   // ← NEVER outside of tests/init
    return k
}

// CORRECT — return the error
func parseKey(b []byte) (*ecdh.PublicKey, error) {
    return ecdh.P521().NewPublicKey(b)
}
```

**Cleanup on function exit — always use defer for resources:**
```go
func sendFile(conn net.Conn, path string) error {
    file, err := os.Open(path)
    if err != nil { return fmt.Errorf("open: %w", err) }
    defer file.Close()     // ← always defer immediately after acquiring

    ln, err := net.Listen("tcp", ":0")
    if err != nil { return fmt.Errorf("listen: %w", err) }
    defer ln.Close()       // ← and again

    // ...
}
```

---

### 2.2 Rust (src-tauri/)

**Use `?` operator — never `.unwrap()` in production paths:**
```rust
// WRONG
let content = fs::read_to_string(&path).unwrap();

// CORRECT
let content = fs::read_to_string(&path)
    .map_err(|e| format!("read config: {e}"))?;
```

**Tauri commands must return `Result<T, String>`:**
```rust
// WRONG
#[tauri::command]
pub fn pick_file() -> Option<String> { ... }

// CORRECT
#[tauri::command]
pub async fn pick_file() -> Result<Option<String>, String> {
    // map all errors to String with context
    Ok(some_value)
}
```

**Never block the async runtime — spawn blocking work:**
```rust
// WRONG — blocks Tauri's async executor
#[tauri::command]
pub async fn read_large_file(path: String) -> Result<Vec<u8>, String> {
    std::fs::read(&path).map_err(|e| e.to_string())  // ← blocking
}

// CORRECT
#[tauri::command]
pub async fn read_large_file(path: String) -> Result<Vec<u8>, String> {
    tokio::task::spawn_blocking(move || {
        std::fs::read(&path).map_err(|e| e.to_string())
    })
    .await
    .map_err(|e| e.to_string())?
}
```

**Always add context to errors before returning to frontend:**
```rust
fn spawn_sidecar(path: &Path) -> Result<Child, String> {
    Command::new(path)
        .spawn()
        .map_err(|e| format!("spawn sidecar at {}: {}", path.display(), e))
}
```

---

### 2.3 TypeScript / SvelteKit (frontend/)

**No `any` types — use the types defined in `src/lib/types.ts`:**
```typescript
// WRONG
function handleEvent(data: any) { ... }

// CORRECT
import type { PeerInfo, ProgressEvent } from '$lib/types';
function handlePeerFound(peer: PeerInfo) { ... }
```

**Always type Tauri event payloads:**
```typescript
// WRONG
await listen('EVT_PEER_FOUND', (event) => {
    peerStore.add(event.payload)  // payload is `unknown`
})

// CORRECT
await listen<PeerInfo>('EVT_PEER_FOUND', (event) => {
    peerStore.add(event.payload)  // payload is `PeerInfo`
})
```

**Svelte stores — never mutate state directly, always go through store methods:**
```typescript
// WRONG — bypasses reactivity
$peerStore.set('id', peer);

// CORRECT — use the store's own mutation methods
peerStore.add(peer);
```

**async/await in Svelte components — always handle errors:**
```typescript
// WRONG — silently swallows errors
onMount(async () => {
    await startDiscovery()
})

// CORRECT — show error state
onMount(async () => {
    try {
        await startDiscovery()
    } catch (e) {
        errorMessage = `Discovery failed: ${e}`
    }
})
```

**Always clean up listeners on component destroy:**
```typescript
import { onMount, onDestroy } from 'svelte'
import { listen } from '@tauri-apps/api/event'

let unlisten: (() => void) | null = null

onMount(async () => {
    unlisten = await listen<PeerInfo>('EVT_PEER_FOUND', handler)
})

onDestroy(() => {
    unlisten?.()     // ← REQUIRED — listener leaks without this
})
```

---

## 3. File and Module Rules

### 3.1 One responsibility per file

Each file does exactly one thing. If you find yourself writing `// Section 2` inside a file, split it.

| File | Contains | Does NOT contain |
|------|----------|-----------------|
| `mdns/discovery.go` | mDNS advertise + scan | Crypto, IPC, HTTP |
| `crypto/ecdh.go` | ECDH + HKDF | AES, HMAC, TLS |
| `crypto/ctr.go` | AES-256-CTR | ECDH, HMAC |
| `HandTracker.ts` | Camera + MediaPipe | Gesture classification |
| `GestureClassifier.ts` | Landmark → Gesture | Camera, UI, stores |
| `tauribridge.ts` | invoke/listen calls | Business logic |

### 3.2 Naming conventions

| Language | Type | Convention | Example |
|----------|------|------------|---------|
| Go | Types | PascalCase | `BrowserSession`, `PeerInfo` |
| Go | Functions | camelCase | `deriveAESKey`, `parseHashPayload` |
| Go | Constants | ALL_CAPS | `CHUNK_SIZE`, `CTR_COUNTER_SIZE` |
| Go | Files | snake_case | `tcp_sender.go`, `browser_crypto.go` |
| Rust | Types | PascalCase | `AppState`, `TransferArgs` |
| Rust | Functions | snake_case | `send_file`, `pick_file` |
| Rust | Constants | SCREAMING_SNAKE | `MAX_FILE_SIZE` |
| TypeScript | Types/Interfaces | PascalCase | `PeerInfo`, `TransferOffer` |
| TypeScript | Functions | camelCase | `startDiscovery`, `sendFile` |
| TypeScript | Stores | camelCase + Store | `peerStore`, `transferStore` |
| Svelte | Components | PascalCase | `MiniMode.svelte`, `GlassPanel.svelte` |
| CSS classes | kebab-case | `drop-zone`, `file-card` |

### 3.3 Import order

**Go:**
```go
import (
    // 1. Standard library
    "crypto/ecdh"
    "encoding/json"
    "fmt"

    // 2. External packages (blank line separator)
    "github.com/hashicorp/mdns"
    "golang.org/x/crypto/hkdf"

    // 3. Internal packages (blank line separator)
    "github.com/gestureshare/sidecar/crypto"
    "github.com/gestureshare/sidecar/ipc"
)
```

**TypeScript:**
```typescript
// 1. Svelte/framework
import { onMount, onDestroy } from 'svelte'
import { derived } from 'svelte/store'

// 2. Tauri
import { invoke } from '@tauri-apps/api/tauri'
import { listen } from '@tauri-apps/api/event'

// 3. Third-party
import * as THREE from 'three'

// 4. Internal $lib imports
import { peerStore } from '$lib/stores/peerStore'
import type { PeerInfo } from '$lib/types'
```

---

## 4. Security Rules

These are hard stops. Break them and the PR is rejected.

**Rule S1 — No plaintext secrets in code or logs:**
```go
// WRONG
log.Printf("[crypto] derived key: %x", aesKey)

// CORRECT — log that derivation happened, never the value
log.Printf("[crypto] AES key derived for session %s", sid[:8])
```

**Rule S2 — Never write key material to disk:**
```go
// WRONG
os.WriteFile("session.key", aesKey, 0600)

// CORRECT — keys live only in memory, wiped on process exit
```

**Rule S3 — Always validate filenames before writing:**
```go
// WRONG — path traversal vulnerability
os.WriteFile(filepath.Join(downloadDir, filename), data, 0644)

// CORRECT
safe := filepath.Base(filename)   // strips any ../
if safe == "" || safe == "." {
    return fmt.Errorf("invalid filename: %q", filename)
}
os.WriteFile(filepath.Join(downloadDir, safe), data, 0644)
```

**Rule S4 — Authenticate before any data operation:**
```go
// WRONG — serves data without checking session
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
    path := r.URL.Query().Get("path")
    http.ServeFile(w, r, path)
}

// CORRECT
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
    sess := s.authSession(r)
    if sess == nil {
        http.Error(w, "Unauthorized", 401)
        return
    }
    // only serve files the session is authorized for
}
```

**Rule S5 — Never trust user input for file paths:**
```go
// Reject any path containing: .. / \ : * ? " < > |
// Use filepath.Base() to sanitize, then validate result
```

---

## 5. IPC Rules

**Every command handler must emit a response or error:**
```go
// WRONG — command disappears silently if it fails
func (r *Router) handleSendFile(p SendFilePayload) {
    if err := transfer.Send(p); err != nil {
        log.Printf("send failed: %v", err)  // nobody knows
    }
}

// CORRECT
func (r *Router) handleSendFile(p SendFilePayload) {
    if err := transfer.Send(p); err != nil {
        ipc.Emit(ipc.EvtTxError, ipc.ErrorPayload{
            Code:    "TX_FAILED",
            Message: err.Error(),
        })
        return
    }
}
```

**IPC messages are fire-and-forget, not request-response:**
- Rust sends CMD_ → Go processes → Go emits EVT_ asynchronously
- Do not block waiting for a response in the Rust command handler
- SvelteKit listens for EVT_ independently of the invoke() that triggered CMD_

---

## 6. Git Commit Rules

Format: `<type>(<scope>): <description>`

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `perf`  
Scopes: `mdns`, `crypto`, `transfer`, `ipc`, `gesture`, `ui`, `tauri`, `browser`

```
feat(crypto): add HKDF-SHA512 key derivation for AES and HMAC keys
fix(transfer): prevent path traversal in received filename sanitization
perf(tcp): increase socket buffer to 4MB for gigabit throughput
test(crypto): add unit tests for AES-256-CTR encrypt/decrypt roundtrip
docs(arch): add state machine diagram for transfer lifecycle
```

**One logical change per commit. Never commit:**
- Multiple unrelated features
- Commented-out code
- Debug print statements (`fmt.Println`, `console.log` for debugging)
- `.unwrap()` or `panic!` without an explanation comment

---

## 7. What to Do When Stuck

In order:

1. **Re-read the relevant doc** — the answer is probably in ARCHITECTURE.md or WIRE_PROTOCOL.md
2. **Check existing code** — look for how a similar problem was solved in an adjacent file
3. **Write a TODO comment** with exact description of what's needed, and continue
4. **Do not guess at crypto** — if the cryptographic operation isn't documented, stop and ask

Never invent a new protocol, packet format, or crypto primitive. Everything is specified. If something seems unspecified, it's a documentation gap — flag it, don't improvise.
