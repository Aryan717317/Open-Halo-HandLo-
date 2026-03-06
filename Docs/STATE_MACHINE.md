# GestureShare — UI State Machine

**Version:** 1.0  
**Purpose:** Defines every screen, every valid transition, and what triggers each transition. An agent writing any UI code must implement exactly this state machine — no undocumented state changes.

---

## 1. Complete State Diagram

```
                         ┌─────────────────────────────────────────────┐
                         │                  APP LAUNCH                 │
                         │  - Generate P-521 keypair (Go)             │
                         │  - Start HTTPS server (Go)                 │
                         │  - Begin mDNS discovery (Go)               │
                         └─────────────────┬───────────────────────────┘
                                           │
                                           ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                              CONNECT                                         │
│  What shows: QR code, 6-digit code, peer list (auto-updating)               │
│  What's active: mDNS scan loop, QR display, gesture cam OFF                 │
└────────────┬────────────────┬──────────────────┬──────────────────────────┘
             │ User taps peer  │ QR scanned by    │ Text code
             │ from peer list  │ phone browser    │ entered & resolved
             ▼                ▼                  ▼
┌────────────────────────────────────────────────────────────┐
│                         PAIRING                            │
│  What shows: spinning indicator, handshake step progress   │
│  What's active: ECDH exchange in progress                  │
└────────────────────────────┬───────────────────────────────┘
          │ Handshake         │ Handshake
          │ success           │ fails
          ▼                  ▼
┌──────────────────┐    ┌──────────────────────────────────┐
│     READY        │    │            ERROR                 │
│ Camera ON        │    │ Shows: error code + message      │
│ Phantom hand ON  │◄───┤ Only exit: "Scan QR Again"      │
│ Gesture active   │    │ (for CERT_MISMATCH)             │
└──────────────────┘    │ or "Retry" (for other errors)   │
         │              └──────────────────────────────────┘
         │ GRAB gesture confirmed
         │ (8 frames, screen==='ready')
         ▼
┌──────────────────┐
│    SELECTING     │
│ File picker open │
│ Orb on palm      │
└────┬─────────────┘
     │ File chosen       │ Picker cancelled
     ▼                   ▼
┌─────────────┐      ┌──────────┐
│ SELECTING   │      │  READY   │ (return, no file selected)
│ (orb        │      └──────────┘
│  attached)  │
└──────┬──────┘
       │ OPEN_PALM confirmed
       │ (8 frames, file selected, peer connected)
       ▼
┌──────────────────┐
│    SENDING       │
│ Progress ring    │
│ Orb flies off    │◄─── Cancel button → READY
└────┬─────────────┘
     │ Transfer         │ Transfer
     │ complete         │ error
     ▼                  ▼
┌──────────┐      ┌──────────────────────────┐
│   DONE   │      │   ERROR                  │
│ Stats    │      │ (non-security error)     │
│ shown    │      │ "Try again" → READY      │
└──────────┘      └──────────────────────────┘
     │
     │ "Send another file" button
     ▼
  READY
```

---

## 2. Screen Definitions

### `connect`
**When active:** App just launched, or after session terminated  
**Visible elements:**
- QR code (generated from `EVT_DEVICE_INFO.qr_url`)
- 6-digit text code (shown below QR)
- Peer list (populated by `EVT_PEER_FOUND` events)
- "Scanning for nearby devices..." indicator when peer list empty
- Connection method tabs: QR / Code / Nearby

**Active background processes:**
- mDNS scan loop running
- HTTPS server accepting connections (phone may scan QR at any time)
- Gesture camera: **OFF** (no need, conserves resources)

**Entry triggers:**
- App launch
- `btn-send-another` clicked from `done` screen
- Session terminated / error dismissed (non-security)
- `EVT_PAIR_REJECTED` received

**Exit triggers:**
- User taps peer in list → transition to `pairing`
- User submits text code → transition to `pairing`  
- `EVT_PAIR_SUCCESS` received (QR path — phone initiated) → transition to `ready`

---

### `pairing`
**When active:** Handshake in progress between two devices  
**Visible elements:**
- Spinner / pulse animation
- Step indicators (QR parsed → Cert verified → ECDH → HKDF → E2EE)
- Peer name being connected to
- Cancel button

**Active background processes:**
- ECDH handshake running in Go sidecar
- Timeout: 15 seconds before auto-fail

**Entry triggers:**
- User taps peer from list in `connect` screen
- User submits valid text code

**Exit triggers:**
- `EVT_PAIR_SUCCESS` → transition to `ready`
- `EVT_PAIR_REJECTED` or timeout → transition to `connect`
- CERT_MISMATCH → transition to `error` (no back button)

---

### `ready`
**When active:** Paired with at least one device, waiting for gesture  
**Visible elements:**
- Camera feed (mirrored, 60% opacity)
- Phantom hand skeleton (Three.js overlay, gesture-colored)
- Status: "Connected to [peer name]"
- Gesture hint: "✊ Close fist to select a file"
- Peer indicator badge (green dot)
- Mini-Mode toggle button

**Active background processes:**
- Camera stream: **ON** at 30fps
- MediaPipe inference: **ON** every frame
- Gesture classifier: **ON**, debouncing active
- Tauri event listeners: active for `EVT_TX_OFFER` (incoming file)

**Entry triggers:**
- `EVT_PAIR_SUCCESS` from `pairing` or `connect`
- Transfer cancelled from `sending` screen
- Transfer errored (non-security) + "Try again"
- "Send another file" from `done`

**Exit triggers:**
- GRAB gesture confirmed → `selecting`
- `EVT_TX_OFFER` received → show incoming toast (screen stays `ready`)
- OPEN_PALM while incoming toast showing → `receiving`

---

### `selecting`
**When active:** GRAB confirmed, file picker open, file orb on palm  
**Visible elements:**
- Camera feed + phantom hand (orange skeleton — grab color)
- File orb attached to palm, pulsing
- File picker dialog (native OS dialog, above everything)
- Status: "Hold still — or open picker is open"

**Substates (internal, not separate screens):**
- `selecting/waiting`: picker open, no file chosen yet
- `selecting/file-ready`: file chosen, orb attached, waiting for OPEN_PALM

**Entry triggers:**
- GRAB gesture confirmed while `screen === 'ready'`

**Exit triggers:**
- File picker cancelled (no file selected) → `ready`
- File selected → remain in `selecting` (substate `file-ready`), hint changes to "🖐 Open palm to send"
- OPEN_PALM confirmed while file selected → `sending`
- No gesture / hand leaves frame for >5s while file selected → remain in `selecting` (don't auto-cancel)

---

### `sending`
**When active:** Transfer actively in progress  
**Visible elements:**
- Progress ring (0→100%, animated stroke)
- MB/s, MB done, ETA stats
- File name being sent
- Orb flying off screen (animation plays once)
- Cancel button

**Entry triggers:**
- OPEN_PALM confirmed while `screen === 'selecting'` and file selected

**Exit triggers:**
- `EVT_TX_COMPLETE` → `done`
- `EVT_TX_ERROR` → `error` (with error details)
- Cancel button clicked → `ready` (partial file deleted by Go)

---

### `receiving`
**When active:** Incoming file transfer in progress  
**Visible elements:**
- Progress ring (receiver side)
- "Receiving [filename] from [peer]"
- Sender name
- Cancel/Reject button

**Entry triggers:**
- User accepts incoming file toast (OPEN_PALM or tap "Accept")

**Exit triggers:**
- `EVT_TX_COMPLETE` → `done`
- `EVT_TX_ERROR` → `error`
- Cancel → `ready`

---

### `done`
**When active:** Transfer completed successfully  
**Visible elements:**
- ✦ icon (animated entrance)
- "Transfer complete"
- Stats: file size, speed, duration
- "Keys wiped from memory. Session remains active."
- "Send another file" button
- "View audit log" button

**Entry triggers:**
- `EVT_TX_COMPLETE` from `sending` or `receiving`

**Exit triggers:**
- "Send another file" → `ready`
- (session stays active — peer still connected)

---

### `error`
**When active:** Unrecoverable or security error  
**Two variants:**

**Variant A — Security Error (CERT_MISMATCH, HMAC_FAIL):**
- Red background tint
- ⊘ icon
- Error explanation (human-readable)
- Technical detail (fingerprint values, transfer ID)
- ONLY action: "Scan QR Again" (goes to `connect`, clears session)
- No dismiss, no back, no ignore

**Variant B — Recoverable Error:**
- Neutral error card
- Error message
- "Try Again" button → `ready`
- "Disconnect" button → `connect`

**Entry triggers:**
- CERT_MISMATCH during `pairing`
- HMAC_FAIL during `sending`/`receiving`
- SIDECAR_CRASH (fatal=true)
- Any EVT_TX_ERROR with non-cancellation code

**Exit triggers:**
- Variant A: "Scan QR Again" only → `connect`
- Variant B: "Try Again" → `ready` or "Disconnect" → `connect`

---

## 3. Incoming File Toast (Overlay, Not a Screen)

The incoming file toast appears **on top of the current screen** (always `ready` when it appears). It is not a screen transition.

```
┌──────────────────────────────────────────────┐
│  📁 photo_20240312.heic                      │
│  8.2 MB · from MacBook-Pro                   │
│                         [✕ Reject] [Accept ⬇] │
└──────────────────────────────────────────────┘
```

**Shows:** When `EVT_TX_OFFER` received and `screen === 'ready'`  
**Dismiss (reject):** `btn-reject` clicked → Go sidecar notified, toast hides  
**Accept:** `btn-accept` clicked → transition to `receiving` screen  
**Auto-dismiss:** After 30 seconds without response → auto-reject  
**Multiple simultaneous offers:** Queue them; show one at a time

---

## 4. State Invariants (Rules That Must Always Be True)

These invariants must never be violated by any code:

| Invariant | Description |
|-----------|-------------|
| INV-01 | Camera is ON if and only if `screen ∈ {ready, selecting, sending, receiving}` |
| INV-02 | File orb is visible if and only if `screen ∈ {selecting, sending}` |
| INV-03 | Gesture classifier runs if and only if camera is ON |
| INV-04 | GRAB action only fires when `screen === 'ready'` |
| INV-05 | OPEN_PALM → send only fires when `screen === 'selecting'` AND `selectedFile !== null` |
| INV-06 | OPEN_PALM → accept only fires when incoming toast is showing |
| INV-07 | `screen === 'error'` with CERT_MISMATCH has NO dismiss/back option |
| INV-08 | A file is never written to disk before HMAC verification passes |
| INV-09 | `selectedFile` is always `null` when `screen === 'ready'` |
| INV-10 | At most one active transfer at a time (v1.0) |

---

## 5. SvelteKit Implementation Pattern

```typescript
// +page.svelte — canonical state machine implementation

type Screen = 'connect' | 'pairing' | 'ready' | 'selecting' | 'sending' | 'receiving' | 'done' | 'error';

let screen: Screen = 'connect';
let selectedFile: string | null = null;
let selectedFileName = '';
let activePeerId = '';
let incomingOffer: TransferOffer | null = null;
let errorDetails: { code: string; message: string; isSecurity: boolean } | null = null;

// The ONLY function that changes screen — enforces all invariants
function transition(to: Screen, context?: Record<string, unknown>) {
    // Enforce INV-01: camera management
    const cameraScreens: Screen[] = ['ready', 'selecting', 'sending', 'receiving'];
    if (cameraScreens.includes(to) && !cameraScreens.includes(screen)) {
        startCamera();
    } else if (!cameraScreens.includes(to) && cameraScreens.includes(screen)) {
        stopCamera();  // conserve resources
    }

    // Enforce INV-09: clear selected file on returning to ready
    if (to === 'ready') {
        selectedFile = null;
        selectedFileName = '';
    }

    // Enforce INV-10: cannot enter sending/receiving if one is active
    if ((to === 'sending' || to === 'receiving') && 
        (screen === 'sending' || screen === 'receiving')) {
        console.error('Attempted to start transfer while one is active');
        return;
    }

    screen = to;
    if (context) Object.assign({ selectedFile, activePeerId, errorDetails }, context);
}

// Usage:
// transition('ready')
// transition('selecting')
// transition('sending')
// transition('error', { errorDetails: { code: 'CERT_MISMATCH', isSecurity: true } })
```

---

## 6. Mini-Mode State

Mini-Mode is a **window state**, not a screen state. It can be active during any screen.

```
Normal Window (1200×800)  ←──────────── Cmd/Ctrl+Shift+M ────────────►  Mini-Mode (280×80)

Mini-Mode shows:
  [peer name] · [screen label] · [speed if transferring]
  e.g.: "MacBook · Sending · 52 MB/s"

Mini-Mode transitions:
  Click anywhere on mini window → expand back to normal window
  Transfer completes → flash green, then return to normal window
```

Mini-Mode must not affect the underlying screen state. The camera continues running if `screen ∈ cameraScreens`.
