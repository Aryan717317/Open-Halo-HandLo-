# GestureShare — Wire Protocol Specification

**Exact byte layouts for all network and IPC messages.**  
**Version:** 1.0 | **Referenced by:** `ARCHITECTURE.md`, `IMPLEMENTATION_PLAN.md`

This document is the ground truth. If Go and TypeScript disagree about a byte layout, this document wins. Do not guess byte offsets — look them up here.

---

## 1. IPC Protocol: Tauri (Rust) ↔ Go Sidecar

**Transport:** stdin/stdout pipe between parent (Rust) and child (Go) processes  
**Encoding:** Newline-delimited JSON (NDJSON) — one JSON object per line, terminated by `\n`  
**Direction:** bidirectional — both sides read and write

### Envelope Format

Every message, in both directions:

```json
{"type":"CMD_OR_EVT_NAME","payload":{...}}
```

Rules:
- `type` is always a string constant from the catalogue below
- `payload` is always an object (never null, never a primitive)
- One complete JSON object per line — no multi-line messages
- No trailing comma, no comments
- If payload has no fields: `"payload":{}` (not null, not omitted)

### Full Message Catalogue

**Rust → Go (Commands):**

```
CMD_DISCOVER          {}
CMD_STOP_DISCOVER     {}
CMD_GET_DEVICE_INFO   {}
CMD_PAIR_REQUEST      { peer_id: string, peer_name: string, address: string, port: number }
CMD_PAIR_ACCEPT       { peer_id: string }
CMD_PAIR_REJECT       { peer_id: string }
CMD_SEND_FILE         { transfer_id: string, peer_id: string, file_path: string,
                        file_name: string, file_size: number, mime_type: string }
CMD_CANCEL_TX         { transfer_id: string }
CMD_DISCONNECT        { peer_id: string }
CMD_PUSH_CLIPBOARD    { text: string }
```

**Go → Rust (Events):**

```
EVT_DEVICE_INFO       { name: string, os: string, version: string, local_ip: string }
EVT_PEER_FOUND        { id: string, name: string, address: string, port: number, os: string }
EVT_PEER_LOST         { id: string }
EVT_PAIR_INCOMING     { peer_id: string, peer_name: string, address: string }
EVT_PAIR_SUCCESS      { peer_id: string, peer_name: string }
EVT_PAIR_REJECTED     { peer_id: string }
EVT_TX_OFFER          { transfer_id: string, peer_id: string, peer_name: string,
                        file_name: string, file_size: number, mime_type: string }
EVT_TX_PROGRESS       { transfer_id: string, bytes_sent: number, total_bytes: number,
                        percent: number, speed_bps: number, eta_seconds: number }
EVT_TX_COMPLETE       { transfer_id: string, saved_path: string,
                        elapsed_ms: number, avg_speed_bps: number }
EVT_TX_CANCELLED      { transfer_id: string }
EVT_TX_ERROR          { transfer_id: string, code: string, message: string }
EVT_CLIPBOARD_RX      { text: string }
EVT_ERROR             { code: string, message: string }
```

### IPC Example Exchange

```
[Rust → Go stdin]
{"type":"CMD_DISCOVER","payload":{}}

[Go → Rust stdout]
{"type":"EVT_PEER_FOUND","payload":{"id":"GestureShare-Rajs-MacBook._gestureshare._tcp.","name":"Rajs-MacBook.local.","address":"192.168.1.42","port":47291,"os":"darwin"}}

[Rust → Go stdin]
{"type":"CMD_SEND_FILE","payload":{"transfer_id":"550e8400-e29b-41d4-a716-446655440000","peer_id":"GestureShare-Rajs-MacBook...","file_path":"/Users/user/Desktop/photo.jpg","file_name":"photo.jpg","file_size":4234567,"mime_type":"image/jpeg"}}

[Go → Rust stdout]
{"type":"EVT_TX_PROGRESS","payload":{"transfer_id":"550e8400-e29b-41d4-a716-446655440000","bytes_sent":1048576,"total_bytes":4234567,"percent":24.76,"speed_bps":52428800,"eta_seconds":6}}

[Go → Rust stdout]
{"type":"EVT_TX_COMPLETE","payload":{"transfer_id":"550e8400-e29b-41d4-a716-446655440000","saved_path":"/Users/user/Downloads/photo.jpg","elapsed_ms":81,"avg_speed_bps":52264037}}
```

---

## 2. TCP Transfer Protocol: Desktop-to-Desktop

**Transport:** Raw TCP socket  
**Initiator:** Sender opens listener, receiver connects  
**Byte order:** Big-endian for all multi-byte integers  
**Encryption:** AES-256-CTR applied to each chunk payload independently

### Session Negotiation (pre-transfer)

Before any file bytes flow, sender signals receiver via the existing HTTPS REST API:

```
Sender calls:
  POST https://<receiver-ip>:47291/api/transfer/offer
  Body: { transfer_id, file_name, file_size, mime_type, tcp_port }
  
Receiver responds:
  200 { status: "accepted" }   → sender begins streaming to tcp_port
  200 { status: "rejected" }   → sender aborts
```

### Packet Structure

Every packet in the TCP stream:

```
 0         1         2         3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5
┌─────────────────────────────────┐
│         PAYLOAD_LEN (4B)        │  uint32 big-endian: byte count of [COUNTER + CIPHERTEXT]
├─────────────────────────────────┤
│                                 │
│         COUNTER (16B)           │  random 16 bytes, unique per packet
│                                 │
├─────────────────────────────────┤
│                                 │
│      CIPHERTEXT (variable)      │  AES-256-CTR(plaintext_chunk, key, counter)
│      max 65536 bytes            │
│                                 │
└─────────────────────────────────┘

Total packet overhead: 4 + 16 = 20 bytes per chunk
Plaintext chunk size: 65536 bytes (64KB) except last chunk (may be smaller)
```

### EOF Marker (Final Packet)

After all data chunks, sender sends one final packet:

```
 0         1         2         3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5
┌─────────────────────────────────┐
│     0x00 0x00 0x00 0x24 (4B)    │  PAYLOAD_LEN = 36 (0x24)
├─────────────────────────────────┤
│     0xFF × 16 (16B)             │  COUNTER = all 0xFF bytes (EOF sentinel)
├─────────────────────────────────┤
│     HMAC-SHA256 (32B)           │  HMAC over ALL ciphertext bytes concatenated
└─────────────────────────────────┘
```

Receiver detects EOF when COUNTER is all 0xFF bytes. It then:
1. Extracts the 32-byte HMAC
2. Computes HMAC-SHA256 over every ciphertext byte received (in order)
3. Compares — if mismatch: delete file, emit `EVT_TX_ERROR` code `TX_HMAC_FAIL`
4. If match: emit `EVT_TX_COMPLETE`

### TCP Reading Algorithm (Go pseudocode)

```go
func receive(conn net.Conn, key []byte, outPath string) error {
    file, _ := os.Create(outPath)
    defer file.Close()
    
    var allCiphertext []byte  // accumulate for HMAC verification
    
    for {
        // Read 4-byte length prefix
        lenBuf := make([]byte, 4)
        io.ReadFull(conn, lenBuf)
        payloadLen := binary.BigEndian.Uint32(lenBuf)
        
        // Read full payload
        payload := make([]byte, payloadLen)
        io.ReadFull(conn, payload)
        
        counter    := payload[:16]
        ciphertext := payload[16:]
        
        // Check EOF sentinel
        if isAllFF(counter) {
            hmacSig := ciphertext  // last packet has HMAC, not data
            if !verifyHMAC(allCiphertext, hmacSig, key) {
                os.Remove(outPath)
                return fmt.Errorf("HMAC verification failed")
            }
            return nil  // success
        }
        
        // Decrypt and write
        plaintext := decryptCTR(ciphertext, key, counter)
        file.Write(plaintext)
        allCiphertext = append(allCiphertext, ciphertext...)
    }
}
```

---

## 3. HTTPS API Protocol: Browser ↔ Go Server

**Transport:** HTTPS (TLS 1.3)  
**Encoding:** JSON for control messages, binary for file payloads  
**Auth:** Session token in `X-GS-Token` header on all protected endpoints

### 3.1 Cert Ping

```
GET /api/cert-ping
Headers: (none required)

Response 200:
{
  "fingerprint": "a1b2c3d4e5f6..."   // SHA-256 of TLS cert DER, lowercase hex, no colons
}
```

### 3.2 Session Register

```
POST /api/session/register
Content-Type: application/json
Body:
{
  "pubKey": "<base64url-no-padding>",   // browser's P-521 raw public key bytes
  "sessionId": "<64-hex-chars>"          // from QR hash fragment
}

Response 200:
{
  "token": "<64-hex-chars>",             // session token for subsequent requests
  "desktopName": "Rajs-MacBook"
}

Response 403:
{
  "error": "SESSION_ID_MISMATCH"
}
```

### 3.3 File Upload (Phone → Desktop)

```
POST /api/transfer/upload
X-GS-Token: <session-token>
X-GS-FileName: <percent-encoded filename>
X-GS-OrigSize: <original plaintext size in bytes, decimal string>
X-GS-HMAC: <base64-standard-encoded HMAC-SHA256 of full ciphertext>
Content-Type: application/octet-stream
Body: <binary: AES-256-CTR encrypted file>

Encrypted body format:
  [counter:16B][ciphertext:variable]
  The entire file is one single AES-CTR operation with one counter.
  (No chunking in the HTTPS upload path — browser sends full file in one request)

Response 200:
{
  "status": "ok",
  "saved_path": "/Users/user/Downloads/photo.jpg"
}

Response 400:
{
  "error": "TX_HMAC_FAIL",
  "message": "HMAC verification failed"
}

Response 401:
{
  "error": "SESSION_UNAUTHORIZED"
}
```

### 3.4 Transfer Offer (Desktop → Browser)

```
POST /api/transfer/offer
X-GS-Token: <session-token>
Body:
{
  "transfer_id": "<uuid>",
  "file_name": "video.mp4",
  "file_size": 104857600,
  "mime_type": "video/mp4"
}

Response 200:
{
  "status": "pending"    // browser will show accept/reject UI
}
```

### 3.5 Transfer Accept / Reject

```
POST /api/transfer/accept
X-GS-Token: <session-token>
Body: { "transfer_id": "<uuid>" }

POST /api/transfer/reject
X-GS-Token: <session-token>
Body: { "transfer_id": "<uuid>" }
```

### 3.6 File Download (Desktop → Browser)

```
GET /api/transfer/download?transfer_id=<uuid>
X-GS-Token: <session-token>

Response 200:
Content-Type: application/octet-stream
X-GS-OrigSize: <plaintext size>
X-GS-HMAC: <base64 HMAC of full ciphertext>
Body: <binary: AES-256-CTR encrypted file>

Same format as upload: [counter:16B][ciphertext:variable]
```

### 3.7 Clipboard Push (Browser → Desktop)

```
POST /api/clipboard/push
X-GS-Token: <session-token>
Content-Type: application/json
Body:
{
  "data": "<base64-standard: [counter:16B][encrypted-text]>"
}

Response 200:
{ "status": "ok" }
```

### 3.8 WebSocket Push (Desktop → Browser)

```
WS /ws?sid=<session-id>

Messages are JSON objects sent from server to browser:

{ "type": "CLIPBOARD_PUSH", "data": "<base64: [counter:16B][encrypted-text]>" }
{ "type": "INCOMING_FILE",  "transfer_id": "...", "name": "...", "size": 12345 }
{ "type": "PING",           "ts": 1741267200000 }
{ "type": "SESSION_EXPIRING", "expires_in_seconds": 300 }
```

---

## 4. QR Code Payload Format

The QR code encodes a URL. The security-sensitive data is in the **hash fragment** — never transmitted over the network.

```
https://<local-ip>:<port>/join#key=<pubkey>&fp=<fingerprint>&sid=<sessionid>

Components:
  scheme:   always "https" (never http)
  host:     local IPv4 address of desktop (e.g., 192.168.1.42)
            NOT hostname — IP avoids DNS resolution entirely
  port:     47291 (default) or configured port
  path:     always "/join"
  fragment: NEVER sent to server (browser-only)
    key:    base64url (no padding) of P-521 raw public key bytes (133 bytes → ~178 chars)
    fp:     lowercase hex, no colons, SHA-256 of TLS cert DER (64 hex chars)
    sid:    lowercase hex, 32 random bytes (64 hex chars)

Example:
  https://192.168.1.42:47291/join#key=BAFf7K...Xq2A&fp=a1b2c3d4...&sid=deadbeef...
```

### Local IP Selection Algorithm (Go)

```go
func getLocalIP() (string, error) {
    // Enumerate all network interfaces
    // Skip: loopback (127.x), link-local (169.254.x), IPv6
    // Prefer: interface with default route
    // Return: first non-loopback IPv4 address on an active interface
    
    ifaces, _ := net.Interfaces()
    for _, iface := range ifaces {
        if iface.Flags&net.FlagUp == 0 { continue }
        if iface.Flags&net.FlagLoopback != 0 { continue }
        addrs, _ := iface.Addrs()
        for _, addr := range addrs {
            if ipnet, ok := addr.(*net.IPNet); ok {
                if ip4 := ipnet.IP.To4(); ip4 != nil {
                    if !ip4.IsLinkLocalUnicast() {
                        return ip4.String(), nil
                    }
                }
            }
        }
    }
    return "", fmt.Errorf("no suitable network interface found")
}
```

---

## 5. Crypto Primitive Byte Layouts

### AES-256-CTR Packet (both TCP and HTTPS)

```
Bytes  0-15:  Counter (16 bytes, random, unique per encryption call)
Bytes 16-end: Ciphertext (AES-CTR output, same length as plaintext)

Decryption:
  counter    = data[0:16]
  ciphertext = data[16:]
  plaintext  = AES_CTR_decrypt(ciphertext, key=session_aes_key, counter=counter)
```

### HMAC-SHA256 Encoding

```
Raw:     32 bytes (256-bit HMAC output)
Encoded: base64 standard encoding (with padding = signs)
         Length: 44 characters

In X-GS-HMAC header: base64 standard
In IPC JSON:         base64 standard
In TCP EOF packet:   raw 32 bytes (not base64)
```

### P-521 Public Key Encoding

```
Raw:     133 bytes (uncompressed point: 0x04 || X || Y, each 66 bytes)
In QR:   base64url (no padding), ~178 characters
In POST: base64url (no padding)
In Go:   ecdh.PublicKey.Bytes() → 133 bytes
In JS:   crypto.subtle.exportKey("raw", pubKey) → 133-byte ArrayBuffer
```

### Session ID Format

```
Raw:     32 random bytes
Encoded: lowercase hex string, 64 characters
Used as: HKDF salt (hex decoded back to 32 bytes before use)
```
