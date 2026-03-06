# GestureShare — Complete Type Definitions & API Contracts

**Version:** 1.0  
**Purpose:** Single source of truth for all data structures shared across Go, Rust, TypeScript, and the browser client. An agent must consult this before writing any code that crosses a process boundary.

---

## 1. IPC Protocol — Go Sidecar ↔ Rust/Tauri

All messages are newline-delimited JSON over stdin/stdout.

### Envelope (all messages)
```typescript
interface IPCMessage {
  type: string;       // one of the CMD_* or EVT_* constants below
  payload: object;    // depends on type — see payloads below
}
```

### Inbound Commands (Rust → Go stdin)

#### CMD_DISCOVER
```json
{ "type": "CMD_DISCOVER", "payload": {} }
```
Start mDNS broadcast + scanning loop. Idempotent.

#### CMD_STOP_DISCOVER
```json
{ "type": "CMD_STOP_DISCOVER", "payload": {} }
```

#### CMD_GET_DEVICE_INFO
```json
{ "type": "CMD_GET_DEVICE_INFO", "payload": {} }
```

#### CMD_PAIR_REQUEST
```json
{
  "type": "CMD_PAIR_REQUEST",
  "payload": {
    "peer_id": "GestureShare-MacBook._gestureshare._tcp.local.",
    "peer_name": "MacBook",
    "peer_address": "192.168.1.42",
    "peer_port": 47291,
    "code": "847291"           // optional — only for text-code pairing
  }
}
```

#### CMD_PAIR_ACCEPT
```json
{
  "type": "CMD_PAIR_ACCEPT",
  "payload": { "peer_id": "string" }
}
```

#### CMD_PAIR_REJECT
```json
{
  "type": "CMD_PAIR_REJECT",
  "payload": { "peer_id": "string" }
}
```

#### CMD_SEND_FILE
```json
{
  "type": "CMD_SEND_FILE",
  "payload": {
    "transfer_id": "550e8400-e29b-41d4-a716-446655440000",  // UUID v4
    "peer_id": "string",
    "file_path": "/Users/user/Documents/report.pdf",
    "file_name": "report.pdf",
    "file_size": 104857600,    // bytes
    "mime_type": "application/pdf"
  }
}
```

#### CMD_CANCEL_TX
```json
{
  "type": "CMD_CANCEL_TX",
  "payload": { "transfer_id": "string" }
}
```

#### CMD_PUSH_CLIPBOARD
```json
{
  "type": "CMD_PUSH_CLIPBOARD",
  "payload": {
    "text": "https://example.com/article",
    "session_token": "string"  // which browser session to push to
  }
}
```

---

### Outbound Events (Go stdout → Rust)

#### EVT_DEVICE_INFO
```json
{
  "type": "EVT_DEVICE_INFO",
  "payload": {
    "name": "MacBook-Pro",
    "os": "darwin",
    "version": "0.1.0",
    "local_ip": "192.168.1.10",
    "port": 47291,
    "session_id": "a3f8...",    // 64-char hex
    "qr_url": "https://192.168.1.10:47291/join#key=...&fp=...&sid=..."
  }
}
```

#### EVT_PEER_FOUND
```json
{
  "type": "EVT_PEER_FOUND",
  "payload": {
    "id": "GestureShare-MacBook._gestureshare._tcp.local.",
    "name": "MacBook",
    "address": "192.168.1.42",
    "port": 47291,
    "os": "darwin",
    "code": "847291"           // 6-digit text code, may be empty
  }
}
```

#### EVT_PEER_LOST
```json
{
  "type": "EVT_PEER_LOST",
  "payload": { "id": "string" }
}
```

#### EVT_PAIR_INCOMING
```json
{
  "type": "EVT_PAIR_INCOMING",
  "payload": {
    "peer_id": "string",
    "peer_name": "string",
    "public_key": "base64url-encoded P-521 public key"
  }
}
```

#### EVT_PAIR_SUCCESS
```json
{
  "type": "EVT_PAIR_SUCCESS",
  "payload": {
    "peer_id": "string",
    "peer_name": "string",
    "session_token": "string"
  }
}
```

#### EVT_PAIR_REJECTED
```json
{
  "type": "EVT_PAIR_REJECTED",
  "payload": { "peer_id": "string" }
}
```

#### EVT_TX_OFFER
```json
{
  "type": "EVT_TX_OFFER",
  "payload": {
    "transfer_id": "string",
    "peer_id": "string",
    "peer_name": "string",
    "file_name": "photo.heic",
    "file_size": 8388608,
    "mime_type": "image/heic"
  }
}
```

#### EVT_TX_PROGRESS
```json
{
  "type": "EVT_TX_PROGRESS",
  "payload": {
    "transfer_id": "string",
    "bytes_sent": 52428800,
    "total_bytes": 104857600,
    "percent": 50.0,
    "speed_bps": 52428800,     // bytes per second
    "eta_seconds": 1
  }
}
```
Emitted every 250ms during active transfer.

#### EVT_TX_COMPLETE
```json
{
  "type": "EVT_TX_COMPLETE",
  "payload": {
    "transfer_id": "string",
    "saved_path": "/Users/user/Downloads/photo.heic",
    "bytes_received": 8388608,
    "duration_ms": 210,
    "avg_speed_bps": 39941904
  }
}
```

#### EVT_TX_ERROR
```json
{
  "type": "EVT_TX_ERROR",
  "payload": {
    "transfer_id": "string",
    "code": "HMAC_FAIL",       // see Error Codes below
    "message": "Integrity check failed — file rejected"
  }
}
```

#### EVT_CLIPBOARD_RX
```json
{
  "type": "EVT_CLIPBOARD_RX",
  "payload": {
    "text": "https://example.com",
    "is_url": true,
    "source_peer": "iPhone"
  }
}
```

#### EVT_ERROR
```json
{
  "type": "EVT_ERROR",
  "payload": {
    "code": "MDNS_BIND_FAIL",
    "message": "Cannot bind mDNS socket — port in use",
    "fatal": false
  }
}
```

---

## 2. REST API — Go HTTPS Server

Base URL: `https://<local-ip>:47291`  
TLS: 1.3 minimum, self-signed cert, fingerprint verified by client via QR.  
Auth: `X-GS-Token: <session-token>` header on all protected routes.

### Public Endpoints (no auth required)

#### GET /join
Serves `browser-client/index.html`.  
Response: `text/html`

#### GET /api/cert-ping
Returns the server's TLS certificate SHA-256 fingerprint for client-side verification.  
Response:
```json
{ "fingerprint": "a3f8b2c1d4e5f6..." }   // 64-char lowercase hex
```

### Session Endpoints

#### POST /api/session/register
Completes ECDH handshake with browser client.

Request headers: none (before auth)  
Request body:
```json
{
  "pub_key": "BM...",          // URL-safe base64 of browser's P-521 uncompressed public key (133 bytes raw)
  "session_id": "a3f8..."      // 64-char hex — must match server's session ID from QR
}
```
Response `200`:
```json
{
  "token": "deadbeef...",      // 64-char hex session token
  "desktop_name": "MacBook-Pro"
}
```
Response `403`: session ID mismatch  
Response `400`: invalid public key format

#### DELETE /api/session
Terminate session, wipe server-side keys.  
Auth: required  
Response `200`: `{ "status": "ok" }`

### Transfer Endpoints

#### POST /api/transfer/offer
Desktop notifies browser that a file is ready to download.  
Auth: required  
Request body:
```json
{
  "transfer_id": "uuid-v4",
  "file_name": "report.pdf",
  "file_size": 104857600,
  "mime_type": "application/pdf"
}
```
Response `200`: `{ "status": "queued" }`  
Side effect: server pushes `INCOMING_FILE` event to browser via WebSocket.

#### GET /api/transfer/download?transfer_id=<id>
Browser downloads a file that desktop has offered.  
Auth: required  
Response: `application/octet-stream` — AES-256-CTR encrypted byte stream  
Format: stream of packets, each `[16-byte counter][ciphertext]` for each 64KB chunk  
Header: `X-GS-HMAC: <base64-hmac>` — HMAC-SHA256 of full ciphertext

#### POST /api/transfer/upload
Browser uploads an encrypted file to desktop.  
Auth: required  
Request headers:
```
X-GS-Token:     <session-token>
X-GS-FileName:  <percent-encoded filename>
X-GS-OrigSize:  <original plaintext byte count>
X-GS-HMAC:      <base64-encoded HMAC-SHA256 of full ciphertext>
Content-Type:   application/octet-stream
```
Request body: full encrypted payload — all chunks concatenated, each `[16-byte counter][ciphertext]`  
Response `200`: `{ "status": "ok", "saved_path": "..." }`  
Response `400`: HMAC verification failed — file rejected  
Response `401`: invalid session token  
Response `413`: file exceeds 10GB limit

#### POST /api/transfer/reject
Browser rejects an incoming file offer.  
Auth: required  
Request body: `{ "transfer_id": "string" }`

### Clipboard Endpoints

#### POST /api/clipboard/push
Browser sends encrypted clipboard text to desktop.  
Auth: required  
Request body:
```json
{
  "data": "<base64-encoded AES-256-CTR encrypted text>"
}
```
Response `200`: `{ "status": "ok" }`

### WebSocket

#### WS /ws?sid=<session-id>
Persistent push channel from desktop to browser.  
Upgrade: standard WebSocket handshake  
Messages are JSON:
```json
{ "type": "CLIPBOARD_PUSH", "data": "<base64-encrypted-text>" }
{ "type": "INCOMING_FILE",  "name": "photo.jpg", "size": 8388608, "transfer_id": "..." }
{ "type": "TX_COMPLETE",    "transfer_id": "..." }
{ "type": "PING",           "ts": 1234567890 }
```
Server sends `PING` every 30 seconds. Client must respond with `PONG`.

---

## 3. Tauri Commands — Rust Exposed to SvelteKit

Called via `invoke('command_name', { args })` from SvelteKit.

```typescript
// File system
invoke('pick_file'): Promise<string | null>
  // Opens native file picker, returns absolute path or null if cancelled

invoke('pick_files'): Promise<string[]>
  // Multi-file picker

invoke('get_downloads_dir'): Promise<string>
  // Returns ~/Downloads equivalent on all OSes

invoke('get_device_info'): Promise<DeviceInfo>

// Discovery
invoke('start_discovery'): Promise<void>
invoke('stop_discovery'): Promise<void>

// Pairing
invoke('pair_request', { peerId: string, peerAddress: string, peerPort: number }): Promise<void>
invoke('pair_accept', { peerId: string }): Promise<void>
invoke('pair_reject', { peerId: string }): Promise<void>

// Transfer
invoke('send_file', {
  transferId: string,
  peerId: string,
  filePath: string,
  fileName: string,
  fileSize: number,
  mimeType: string
}): Promise<void>

invoke('cancel_transfer', { transferId: string }): Promise<void>

// Clipboard
invoke('push_clipboard', { text: string, sessionToken: string }): Promise<void>

// Window
invoke('set_mini_mode', { enabled: boolean }): Promise<void>
invoke('set_always_on_top', { enabled: boolean }): Promise<void>
```

---

## 4. Tauri Events — Go → Rust → SvelteKit

Listened via `listen('EVT_NAME', handler)` from SvelteKit.

```typescript
listen('EVT_DEVICE_INFO',   (e: { payload: DeviceInfo }) => void)
listen('EVT_PEER_FOUND',    (e: { payload: PeerInfo }) => void)
listen('EVT_PEER_LOST',     (e: { payload: { id: string } }) => void)
listen('EVT_PAIR_INCOMING', (e: { payload: PairIncoming }) => void)
listen('EVT_PAIR_SUCCESS',  (e: { payload: PairSuccess }) => void)
listen('EVT_PAIR_REJECTED', (e: { payload: { peer_id: string } }) => void)
listen('EVT_TX_OFFER',      (e: { payload: TransferOffer }) => void)
listen('EVT_TX_PROGRESS',   (e: { payload: ProgressEvent }) => void)
listen('EVT_TX_COMPLETE',   (e: { payload: TransferComplete }) => void)
listen('EVT_TX_ERROR',      (e: { payload: TransferError }) => void)
listen('EVT_CLIPBOARD_RX',  (e: { payload: ClipboardEvent }) => void)
```

---

## 5. TypeScript Type Definitions

These are the canonical types. All SvelteKit code must import from `$lib/types.ts`.

```typescript
// $lib/types.ts

export interface DeviceInfo {
  name: string;
  os: 'darwin' | 'windows' | 'linux';
  version: string;
  local_ip: string;
  port: number;
  session_id: string;
  qr_url: string;
}

export interface PeerInfo {
  id: string;           // mDNS full service name — used as stable identifier
  name: string;         // human-readable hostname
  address: string;      // IPv4 address
  port: number;
  os: string;
  code: string;         // 6-digit pairing code, may be empty
}

export interface PairIncoming {
  peer_id: string;
  peer_name: string;
  public_key: string;
}

export interface PairSuccess {
  peer_id: string;
  peer_name: string;
  session_token: string;
}

export interface TransferOffer {
  transfer_id: string;
  peer_id: string;
  peer_name: string;
  file_name: string;
  file_size: number;
  mime_type: string;
}

export interface ProgressEvent {
  transfer_id: string;
  bytes_sent: number;
  total_bytes: number;
  percent: number;       // 0.0 – 100.0
  speed_bps: number;
  eta_seconds: number;
}

export interface TransferComplete {
  transfer_id: string;
  saved_path: string;
  bytes_received: number;
  duration_ms: number;
  avg_speed_bps: number;
}

export interface TransferError {
  transfer_id: string;
  code: ErrorCode;
  message: string;
}

export interface ClipboardEvent {
  text: string;
  is_url: boolean;
  source_peer: string;
}

export type ErrorCode =
  | 'HMAC_FAIL'           // integrity check failed
  | 'DECRYPT_FAIL'        // AES decryption error
  | 'CERT_MISMATCH'       // TLS cert fingerprint doesn't match QR
  | 'SESSION_EXPIRED'     // token not found or timed out
  | 'PEER_UNREACHABLE'    // TCP/HTTPS connection failed
  | 'CANCELLED'           // user cancelled
  | 'FILE_NOT_FOUND'      // source file missing
  | 'DISK_FULL'           // cannot write to Downloads
  | 'MDNS_BIND_FAIL'      // port conflict
  | 'UNKNOWN';

export type AppScreen =
  | 'connect'     // choose connection method, show peer list
  | 'pairing'     // mid-pairing handshake in progress
  | 'ready'       // paired, camera active, waiting for gesture
  | 'selecting'   // GRAB confirmed, file picker open
  | 'sending'     // transfer in progress (sender side)
  | 'receiving'   // transfer in progress (receiver side)
  | 'done'        // transfer complete
  | 'error';      // unrecoverable error

export type ConnectionMethod = 'qr' | 'code' | 'lan';

export type Gesture = 'IDLE' | 'GRAB' | 'OPEN_PALM' | 'POINT';

export interface GestureState {
  gesture: Gesture;
  landmarks: NormalizedLandmark[];
  confidence: number;    // 0.0 – 1.0
  handVisible: boolean;
}

export interface NormalizedLandmark {
  x: number;   // 0.0 – 1.0 normalized to frame width
  y: number;   // 0.0 – 1.0 normalized to frame height
  z: number;   // depth relative to wrist (not used for gesture classification)
}

export interface Transfer {
  id: string;
  file_name: string;
  file_size: number;
  peer_id: string;
  peer_name: string;
  direction: 'send' | 'receive';
  status: 'pending' | 'active' | 'complete' | 'error' | 'cancelled';
  progress: number;      // 0 – 100
  speed_bps: number;
  started_at: number;    // Date.now()
  completed_at?: number;
  saved_path?: string;
  error?: TransferError;
}
```

---

## 6. Go Struct Definitions

Canonical Go types for all IPC payloads and API request/response bodies.

```go
// sidecar/ipc/types.go

package ipc

// ── IPC envelopes ─────────────────────────────────────────────────────────────

type MsgType = string

const (
    // Inbound
    CmdDiscover      MsgType = "CMD_DISCOVER"
    CmdStopDiscover  MsgType = "CMD_STOP_DISCOVER"
    CmdGetDeviceInfo MsgType = "CMD_GET_DEVICE_INFO"
    CmdPairRequest   MsgType = "CMD_PAIR_REQUEST"
    CmdPairAccept    MsgType = "CMD_PAIR_ACCEPT"
    CmdPairReject    MsgType = "CMD_PAIR_REJECT"
    CmdSendFile      MsgType = "CMD_SEND_FILE"
    CmdCancelTx      MsgType = "CMD_CANCEL_TX"
    CmdPushClipboard MsgType = "CMD_PUSH_CLIPBOARD"

    // Outbound
    EvtDeviceInfo   MsgType = "EVT_DEVICE_INFO"
    EvtPeerFound    MsgType = "EVT_PEER_FOUND"
    EvtPeerLost     MsgType = "EVT_PEER_LOST"
    EvtPairIncoming MsgType = "EVT_PAIR_INCOMING"
    EvtPairSuccess  MsgType = "EVT_PAIR_SUCCESS"
    EvtPairRejected MsgType = "EVT_PAIR_REJECTED"
    EvtTxOffer      MsgType = "EVT_TX_OFFER"
    EvtTxProgress   MsgType = "EVT_TX_PROGRESS"
    EvtTxComplete   MsgType = "EVT_TX_COMPLETE"
    EvtTxError      MsgType = "EVT_TX_ERROR"
    EvtClipboardRx  MsgType = "EVT_CLIPBOARD_RX"
    EvtError        MsgType = "EVT_ERROR"
)

type IPCMessage struct {
    Type    MsgType         `json:"type"`
    Payload json.RawMessage `json:"payload"`
}

// ── Payload structs ───────────────────────────────────────────────────────────

type DeviceInfoPayload struct {
    Name      string `json:"name"`
    OS        string `json:"os"`
    Version   string `json:"version"`
    LocalIP   string `json:"local_ip"`
    Port      int    `json:"port"`
    SessionID string `json:"session_id"`
    QRURL     string `json:"qr_url"`
}

type PeerPayload struct {
    ID      string `json:"id"`
    Name    string `json:"name"`
    Address string `json:"address"`
    Port    int    `json:"port"`
    OS      string `json:"os"`
    Code    string `json:"code"`
}

type PairRequestPayload struct {
    PeerID      string `json:"peer_id"`
    PeerName    string `json:"peer_name"`
    PeerAddress string `json:"peer_address"`
    PeerPort    int    `json:"peer_port"`
    Code        string `json:"code,omitempty"`
}

type SendFilePayload struct {
    TransferID string `json:"transfer_id"`
    PeerID     string `json:"peer_id"`
    FilePath   string `json:"file_path"`
    FileName   string `json:"file_name"`
    FileSize   int64  `json:"file_size"`
    MimeType   string `json:"mime_type"`
}

type ProgressPayload struct {
    TransferID  string  `json:"transfer_id"`
    BytesSent   int64   `json:"bytes_sent"`
    TotalBytes  int64   `json:"total_bytes"`
    Percent     float64 `json:"percent"`
    SpeedBPS    int64   `json:"speed_bps"`
    ETASeconds  int     `json:"eta_seconds"`
}

type TransferCompletePayload struct {
    TransferID    string `json:"transfer_id"`
    SavedPath     string `json:"saved_path"`
    BytesReceived int64  `json:"bytes_received"`
    DurationMS    int64  `json:"duration_ms"`
    AvgSpeedBPS   int64  `json:"avg_speed_bps"`
}

type ErrorPayload struct {
    TransferID string `json:"transfer_id,omitempty"`
    Code       string `json:"code"`
    Message    string `json:"message"`
    Fatal      bool   `json:"fatal"`
}
```

---

## 7. Error Codes Reference

| Code | Layer | Meaning | Recovery |
|------|-------|---------|---------|
| `HMAC_FAIL` | Transfer | Ciphertext tampered in transit | Reject file, notify user, log |
| `DECRYPT_FAIL` | Transfer | AES-CTR decryption error | Reject file, notify user |
| `CERT_MISMATCH` | Handshake | TLS cert fingerprint ≠ QR value | Abort session, show warning |
| `SESSION_EXPIRED` | Auth | Token not found or timed out | Re-pair device |
| `PEER_UNREACHABLE` | Connection | TCP/HTTPS connection failed | Retry 3x with backoff, then error |
| `CANCELLED` | Transfer | User cancelled | Clean up partial file |
| `FILE_NOT_FOUND` | Transfer | Source file deleted before send | Show error, cancel transfer |
| `DISK_FULL` | Transfer | Cannot write to Downloads | Notify user, clean up |
| `MDNS_BIND_FAIL` | Discovery | Port conflict | Log warning, try alternate port |
| `UNKNOWN` | Any | Unclassified error | Log full error, show generic message |

---

## 8. Cryptographic Wire Formats

### P-521 Public Key Encoding
- Format: **uncompressed** point, 133 bytes: `04 || X (66 bytes) || Y (66 bytes)`
- Wire encoding: **URL-safe base64** (no padding) for QR/JSON, raw bytes for ECDH
- QR hash fragment: `key=<urlsafe-base64-of-133-bytes>`

### AES-256-CTR Packet Format
```
Desktop ↔ Desktop (TCP):
  [4 bytes: uint32 big-endian packet length]
  [16 bytes: random AES-CTR counter block]
  [N bytes: ciphertext]

Browser ↔ Desktop (HTTPS body):
  [16 bytes: random AES-CTR counter block]
  [N bytes: ciphertext]
  (packets concatenated, no length prefix — full body is one encrypted stream)
```

### HMAC-SHA256 Encoding
- Computed over: **full concatenated ciphertext** (all packets combined, without counter bytes)
- Wait — correction: HMAC is computed over `[counter || ciphertext]` per packet, then over all packets concatenated
- Wire encoding: **standard base64** in HTTP header `X-GS-HMAC`

### TLS Certificate Fingerprint
- Algorithm: SHA-256 of raw DER certificate bytes
- Encoding: lowercase hex, 64 characters
- QR hash fragment: `fp=<64-char-hex>`

### Session ID
- Generation: 32 cryptographically random bytes
- Encoding: lowercase hex, 64 characters
- Usage: HKDF salt, QR hash fragment, WebSocket URL parameter
