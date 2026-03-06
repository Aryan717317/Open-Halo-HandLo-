# GestureShare — State Machines

**Every screen transition, transfer lifecycle, and session state is defined here.**  
**Version:** 1.0 | **Referenced by:** `ARCHITECTURE.md`, `IMPLEMENTATION_PLAN.md`

When the agent implements any component that transitions state, it must follow these exact definitions. Diverging from these causes bugs where Go emits an event the frontend doesn't expect, or the UI tries to render a state that doesn't exist.

---

## 1. App Screen State Machine (SvelteKit)

Defined in: `src/lib/types.ts` as `AppScreen`  
Managed in: `src/routes/+page.svelte`

```
                    ┌─────────┐
              start │         │
                    ▼         │
              ┌──────────┐    │
              │ connect  │◄───┘ (btn-disconnect, session expired)
              └──────────┘
                    │
         peer selected OR
         QR scanned + session active
                    │
                    ▼
              ┌──────────┐
              │  pairing │  (handshake in progress)
              └──────────┘
                    │
         EVT_PAIR_SUCCESS
                    │
                    ▼
              ┌──────────┐
              │  ready   │◄─────────────────────────────────┐
              └──────────┘                                  │
              /          \                                  │
  GRAB gesture            EVT_TX_OFFER                      │
  confirmed               (incoming)                        │
       │                       │                            │
       ▼                       ▼                            │
 ┌──────────┐           ┌──────────┐                        │
 │selecting │           │receiving │                        │
 └──────────┘           └──────────┘                        │
       │                   │    │                           │
  file chosen          accept  reject                       │
       │                   │    │                           │
       │                   │    └──────────────────────────►│
       ▼                   ▼                                │
 ┌──────────┐        ┌──────────┐                          │
 │  ready   │        │receiving │                          │
 │(has file)│        │ active   │                          │
 └──────────┘        └──────────┘                          │
       │                   │                               │
  OPEN_PALM           EVT_TX_COMPLETE                      │
  confirmed           EVT_TX_ERROR                         │
       │                   │                               │
       ▼                   ▼                               │
 ┌──────────┐         ┌──────────┐                         │
 │ sending  │         │  done    │─────────────────────────┘
 └──────────┘         └──────────┘     (btn-send-another)
       │
  EVT_TX_COMPLETE
  EVT_TX_ERROR
  btn-cancel
       │
       ▼
  ┌──────────┐
  │  done    │─────────────────────────────────────────────►ready
  └──────────┘
       │
  EVT_TX_ERROR only
       ▼
  ┌──────────┐
  │  error   │─────────────────────────────────────────────►connect
  └──────────┘     (btn-retry / page reload)
```

### AppScreen Type Definition

```typescript
// src/lib/types.ts
export type AppScreen =
  | 'connect'       // default — choose connection method
  | 'pairing'       // handshake in progress (QR scanned, awaiting EVT_PAIR_SUCCESS)
  | 'ready'         // paired, no file selected, camera active
  | 'selecting'     // GRAB confirmed, file picker open or file selected
  | 'sending'       // OPEN_PALM confirmed, transfer in progress (sender)
  | 'receiving'     // incoming file offer shown, awaiting user accept/reject
  | 'receiving-active' // user accepted, transfer in progress (receiver)
  | 'done'          // transfer complete — shown for 3s then → ready
  | 'error'         // unrecoverable error — requires reconnect
```

### Transition Rules

| From | To | Trigger | Guard |
|------|----|---------|-------|
| connect | pairing | peer selected in list OR QR scan detected | peer must exist |
| connect | pairing | text code submitted | code must be 6 digits |
| pairing | ready | `EVT_PAIR_SUCCESS` received | — |
| pairing | error | `EVT_PAIR_REJECTED` or timeout (10s) | — |
| ready | selecting | GRAB gesture confirmed (8 frames) | camera must be active |
| selecting | ready | ESC key or picker cancelled | — |
| selecting | ready (has file) | file chosen from picker | file.size > 0 |
| ready (has file) | sending | OPEN_PALM gesture confirmed | selectedPeerId must be set |
| ready | receiving | `EVT_TX_OFFER` received | session must be active |
| receiving | ready | reject button pressed | — |
| receiving | receiving-active | accept button pressed | — |
| receiving-active | done | `EVT_TX_COMPLETE` | — |
| sending | done | `EVT_TX_COMPLETE` | — |
| sending | error | `EVT_TX_ERROR` | — |
| receiving-active | error | `EVT_TX_ERROR` | — |
| done | ready | 3-second auto-advance OR btn-send-another | — |
| error | connect | btn-retry | — |
| any | connect | session expired or connection lost | — |

---

## 2. Transfer State Machine (Go + SvelteKit)

Defined in: `src/lib/stores/transferStore.ts` as `TransferStatus`  
Also tracked in Go via: `transfer.Status` field in session map

```
              ┌──────────┐
              │ pending  │  (offer sent/received, awaiting accept)
              └──────────┘
                    │
              accept()
                    │
                    ▼
              ┌──────────┐
              │  active  │  (bytes flowing)
              └──────────┘
              /     |     \
      cancel()  progress  complete
          │        │          │
          ▼        │          ▼
    ┌──────────┐   │    ┌──────────┐
    │cancelled │   │    │ complete │
    └──────────┘   │    └──────────┘
                   ▼
            (stays active,
             emit EVT_TX_PROGRESS)
                   
              ┌──────────┐
              │  error   │  (reachable from active only)
              └──────────┘
```

### TransferStatus Type Definition

```typescript
// src/lib/stores/transferStore.ts
export type TransferStatus =
  | 'pending'      // offer made, not yet accepted
  | 'active'       // transfer in progress
  | 'complete'     // finished successfully
  | 'cancelled'    // user cancelled
  | 'error'        // failed (HMAC mismatch, connection drop, etc.)
```

### Transfer Object Shape

```typescript
export interface Transfer {
  id:          string           // UUID v4
  fileName:    string
  fileSize:    number           // bytes
  peerId:      string
  peerName:    string
  direction:   'send' | 'receive'
  status:      TransferStatus
  progress:    number           // 0–100
  speedBps:    number           // bytes per second, rolling 1s window
  startedAt:   number           // Date.now()
  completedAt: number | null
  errorMsg:    string | null
  savedPath:   string | null    // set on receive-complete only
}
```

### Go EVT Emissions per Transfer Status

| Status | Go must emit | Payload fields required |
|--------|-------------|------------------------|
| pending | `EVT_TX_OFFER` | transfer_id, peer_id, peer_name, file_name, file_size, mime_type |
| active (progress) | `EVT_TX_PROGRESS` | transfer_id, bytes_sent, total_bytes, percent, speed_bps |
| complete | `EVT_TX_COMPLETE` | transfer_id, saved_path, elapsed_ms, avg_speed_bps |
| cancelled | `EVT_TX_CANCELLED` | transfer_id |
| error | `EVT_TX_ERROR` | transfer_id, code, message |

---

## 3. Session State Machine (Go server)

Managed in: `sidecar/server/https.go` in the `sessions map[string]*BrowserSession`

```
              ┌──────────────┐
              │  no_session  │  (server just started)
              └──────────────┘
                     │
            POST /api/session/register
            (valid pubkey + matching SID)
                     │
                     ▼
              ┌──────────────┐
              │    active    │  (ECDH complete, token issued)
              └──────────────┘
              /              \
      token valid             token invalid/missing
        (req OK)               (→ 401 Unauthorized)
                              
              │  session expires (1hr)
              │  OR client disconnects WS
              │  OR CMD_DISCONNECT received
              ▼
              ┌──────────────┐
              │   expired    │  (token revoked, keys wiped)
              └──────────────┘
                     │
              app exit
                     │
                     ▼
              ┌──────────────┐
              │    wiped     │  (keys zeroed, struct removed)
              └──────────────┘
```

### Session Struct Fields

```go
// sidecar/server/https.go
type SessionStatus string

const (
    SessionActive  SessionStatus = "active"
    SessionExpired SessionStatus = "expired"
    SessionWiped   SessionStatus = "wiped"
)

type BrowserSession struct {
    ID          string
    Token       string        // 32-byte hex, used as map key
    PubKey      []byte        // browser's P-521 public key
    AESKey      []byte        // 32-byte AES-256-CTR key
    HMACKey     []byte        // 32-byte HMAC-SHA256 key
    DesktopName string
    Status      SessionStatus
    CreatedAt   time.Time
    ExpiresAt   time.Time     // CreatedAt + 1 hour
    ClipCh      chan []byte    // buffered(8), for WS push
}
```

### Session Lifecycle Rules

1. Session is created only after successful ECDH — never before
2. Session expires after 1 hour (even if WebSocket is still open)
3. On expiry: zero out AESKey and HMACKey bytes before removing from map
4. Token is validated on every protected endpoint — no exceptions (see AGENT_RULES Rule S4)
5. After expiry, browser must re-scan QR to establish new session (no silent renewal)

---

## 4. Gesture State Machine (TypeScript)

Managed in: `src/lib/gesture/GestureClassifier.ts`

```
              ┌──────────┐
              │   IDLE   │◄─────────────────────────────────┐
              └──────────┘                                  │
              /     |     \                                 │
     fist     |    index   open                             │
   forming    |  extending  opening                         │
              |                                             │
  (holdCount) ▼ (holdCount)                                 │
              ┌─────────────────────────────────────────┐   │
              │         RAW GESTURE DETECTED            │   │
              │   (not yet confirmed — hold required)   │   │
              └─────────────────────────────────────────┘   │
                              │                             │
                    8 frames same gesture                   │
                              │                             │
              ┌───────────────┼───────────────┐             │
              ▼               ▼               ▼             │
         ┌────────┐      ┌────────┐      ┌────────┐         │
         │  GRAB  │      │  OPEN  │      │ POINT  │         │
         └────────┘      │  PALM  │      └────────┘         │
              │          └────────┘           │             │
         gesture         gesture           gesture          │
         changes         changes           changes          │
              └──────────────┴───────────────┘             │
                             │                              │
                     holdCount resets                       │
                             │                              │
                             └──────────────────────────────┘
```

### Gesture Thresholds (tunable in config)

```typescript
// src/lib/gesture/GestureClassifier.ts
const GRAB_THRESHOLD       = 0.065   // normalized landmark distance
const OPEN_THRESHOLD       = 0.130   // normalized landmark distance
const REQUIRED_HOLD_FRAMES = 8       // ~267ms at 30fps
const CONFIDENCE_SCALE     = REQUIRED_HOLD_FRAMES
```

### Gesture → App Action Mapping

```typescript
// Only applied when screen === 'ready' or 'selecting'
// Other screens ignore gesture events

const GESTURE_ACTIONS: Record<AppScreen, Partial<Record<Gesture, () => void>>> = {
  'ready': {
    GRAB: () => openFilePicker(),         // → screen: 'selecting'
  },
  'selecting': {
    OPEN_PALM: () => initiateTransfer(),  // only if file is selected
  },
  'receiving': {
    OPEN_PALM: () => acceptTransfer(),    // accept incoming file
  },
  // All other screens: gestures are ignored
}
```

---

## 5. Error Code Catalogue

All errors emitted by Go sidecar use these codes. SvelteKit maps codes to user-visible messages.

### Transfer Errors

| Code | Meaning | User Message |
|------|---------|-------------|
| `TX_HMAC_FAIL` | HMAC verification failed on received file | "File integrity check failed — transfer rejected" |
| `TX_DECRYPT_FAIL` | AES decryption failed | "Decryption error — possible data corruption" |
| `TX_DISK_FULL` | Cannot write to Downloads | "Not enough disk space to save file" |
| `TX_FILE_NOT_FOUND` | Source file disappeared during send | "Source file was moved or deleted" |
| `TX_CONNECTION_LOST` | TCP/HTTPS connection dropped mid-transfer | "Connection lost — transfer incomplete" |
| `TX_CANCELLED` | User cancelled | *(no error shown — clean cancel)* |
| `TX_PEER_REJECTED` | Receiving peer rejected the offer | "File was declined by the other device" |
| `TX_TIMEOUT` | No progress for 30 seconds | "Transfer timed out" |

### Session Errors

| Code | Meaning | User Message |
|------|---------|-------------|
| `SESSION_CERT_MISMATCH` | TLS fingerprint didn't match QR value | "Security verification failed — possible network attack" |
| `SESSION_INVALID_KEY` | Browser sent malformed P-521 key | "Invalid cryptographic key from connecting device" |
| `SESSION_ID_MISMATCH` | Session ID in register doesn't match QR | "Session ID mismatch — rescan QR code" |
| `SESSION_EXPIRED` | 1-hour session lifetime exceeded | "Session expired — please scan QR again" |
| `SESSION_UNAUTHORIZED` | Request missing or invalid token | *(log only, return 401)* |

### Connection Errors

| Code | Meaning | User Message |
|------|---------|-------------|
| `CONN_MDNS_FAIL` | mDNS service failed to start | "Local network discovery unavailable — try text code" |
| `CONN_PORT_IN_USE` | Port 47291 already bound | "Port 47291 in use — close other GestureShare instances" |
| `CONN_NO_NETWORK` | No network interface found | "No network connection detected" |
| `CONN_PAIR_TIMEOUT` | Pairing attempt timed out (10s) | "Connection timed out — try again" |
