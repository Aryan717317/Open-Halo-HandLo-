# GestureShare — Error Handling & Edge Cases Catalogue

**Version:** 1.0  
**Purpose:** Exhaustive catalogue of every failure mode, its correct handling, and the exact user-facing message. An agent writing any error path must match the behaviour defined here.

---

## 1. Error Handling Principles

1. **Never lose a file.** A partial or corrupt write must be deleted before returning an error.
2. **Never show a misleading error.** If the HMAC fails, say "integrity check failed" not "network error".
3. **Every error is logged.** All errors go to the audit log with timestamp + error code.
4. **Sensitive errors are sanitised.** Don't expose internal paths or crypto details in UI messages.
5. **Security errors are loud.** CERT_MISMATCH and HMAC_FAIL get prominent red UI, not subtle toasts.
6. **Retry only where safe.** Network errors: retry 3x. Crypto errors: never retry (abort session).

---

## 2. Connection Errors

### C-01: mDNS Port Conflict
**Trigger:** Port 5353 (mDNS) is already in use by another process  
**Go handling:**
```go
if err := startMDNS(); err != nil {
    log.Printf("[mdns] WARN: cannot bind port 5353: %v", err)
    ipc.Emit(EvtError, ErrorPayload{
        Code:    "MDNS_BIND_FAIL",
        Message: "mDNS unavailable — use QR or text code instead",
        Fatal:   false,  // not fatal: other connection methods still work
    })
}
```
**UI behaviour:** Show amber badge "LAN scan unavailable — use QR code"  
**Recovery:** User uses QR or text code pairing

### C-02: HTTPS Port In Use
**Trigger:** Port 47291 already bound  
**Go handling:**
```go
// Try ports 47291, 47292, 47293 before failing
for _, port := range []int{47291, 47292, 47293} {
    if err := startHTTPS(port); err == nil {
        break
    }
}
```
**UI behaviour:** App still launches, shows whichever port was bound in QR  
**Recovery:** Automatic — tries alternate ports

### C-03: Peer Unreachable After Discovery
**Trigger:** mDNS showed peer, but TCP/HTTPS connection times out  
**Handling:** 3 retries with 1s backoff, then emit `EVT_TX_ERROR { code: "PEER_UNREACHABLE" }`  
**UI message:** "Could not reach [peer name] — they may have moved networks"  
**Do NOT:** Remove peer from list (mDNS may still show them)

### C-04: Certificate Fingerprint Mismatch
**Trigger:** TLS cert served by server doesn't match fingerprint in QR code  
**Browser handling:**
```javascript
if (fingerprint !== payload.fp) {
    // Log to audit trail
    UI.log(`SECURITY: Cert mismatch! Expected ${payload.fp.slice(0,16)}... got ${fingerprint.slice(0,16)}...`, 'err');
    // Show prominent warning — never proceed
    throw new Error('CERT_MISMATCH: Possible man-in-the-middle attack detected. Do not proceed.');
}
```
**UI behaviour:** Full-screen red error screen with explanation. No dismiss button. Only "Scan QR again" option.  
**Recovery:** User must scan a fresh QR code from a trusted source  
**Do NOT:** Allow user to bypass this check

### C-05: Session ID Mismatch (register endpoint)
**Trigger:** Browser sends a session ID that doesn't match the server's current session  
**Go handling:** Return HTTP 403 immediately, no session created  
**Browser handling:** Show error screen "Session expired — scan a new QR code"  
**Recovery:** Scan QR again

### C-06: Text Code Not Found
**Trigger:** User enters 6-digit code but no peer on LAN is advertising that code  
**UI message:** "Code not found on your network. Check the code and try again."  
**Recovery:** User re-enters code or uses QR

---

## 3. Transfer Errors

### T-01: HMAC Verification Failure (file tampered)
**Trigger:** Received ciphertext HMAC doesn't match header value  
**Go handling:**
```go
if !crypto.VerifyHMAC(encData, hmacHeader, sess.SharedKey) {
    log.Printf("[transfer] SECURITY: HMAC FAIL for %s — rejecting file", fileName)
    // Never write any bytes to disk
    ipc.Emit(EvtTxError, ErrorPayload{
        TransferID: transferID,
        Code:       "HMAC_FAIL",
        Message:    "File integrity check failed — transfer rejected",
    })
    http.Error(w, "Integrity check failed", http.StatusBadRequest)
    return
}
```
**UI message:** "⚠ Transfer rejected — file integrity check failed. The file may have been tampered with in transit."  
**Do NOT:** Save any portion of the file  
**Do NOT:** Retry automatically

### T-02: Decryption Error
**Trigger:** AES-CTR decryption returns an error (malformed ciphertext)  
**Go handling:** Delete any partial output, emit `EVT_TX_ERROR { code: "DECRYPT_FAIL" }`  
**UI message:** "Decryption failed — the transfer could not be completed"  
**Recovery:** Re-pair and retry

### T-03: File Already Exists
**Trigger:** `~/Downloads/photo.jpg` already exists  
**Go handling:**
```go
func uniquePath(dir, name string) string {
    path := filepath.Join(dir, name)
    if _, err := os.Stat(path); os.IsNotExist(err) {
        return path
    }
    ext := filepath.Ext(name)
    base := name[:len(name)-len(ext)]
    ts := time.Now().Format("150405")
    return filepath.Join(dir, fmt.Sprintf("%s_%s%s", base, ts, ext))
}
```
**UI behaviour:** File saved as `photo_143022.jpg` — no error shown, just confirmation

### T-04: Disk Full
**Trigger:** Write to Downloads fails with "no space left on device"  
**Go handling:** Delete partial file, emit `EVT_TX_ERROR { code: "DISK_FULL" }`  
**UI message:** "Not enough disk space to save [filename] ([size]). Free up space and try again."  
**Recovery:** User frees space, resends

### T-05: Source File Deleted Before Send
**Trigger:** File selected in picker no longer exists when transfer starts  
**Rust handling:**
```rust
if !std::path::Path::new(&file_path).exists() {
    return Err("FILE_NOT_FOUND".to_string());
}
```
**UI message:** "File not found — it may have been moved or deleted"  
**Recovery:** User selects file again

### T-06: Transfer Cancelled by User
**Trigger:** User clicks Cancel during active transfer  
**Go handling:**
```go
// Set cancel flag, stop reading from file/network
// Delete any partial output file
// Emit EVT_TX_ERROR { code: "CANCELLED" }
```
**UI behaviour:** Progress ring disappears, return to Ready state. No error message — cancellation is intentional.

### T-07: Connection Lost Mid-Transfer
**Trigger:** Network drops during transfer (TCP RST or HTTPS disconnect)  
**Go handling:**
```go
// 3 reconnect attempts with exponential backoff
// If all fail: delete partial file, emit EVT_TX_ERROR { code: "PEER_UNREACHABLE" }
```
**UI message:** "Connection lost — transfer could not complete. Reconnect and try again."  
**Recovery (v1.0):** User must resend. Checkpoint resume is Phase 3 feature.

### T-08: File Too Large for Browser Upload
**Trigger:** Browser tries to upload > 10GB  
**Go handling:** Return HTTP 413 Request Entity Too Large  
**Browser handling:**
```javascript
if (file.size > 10 * 1024 ** 3) {
    UI.showError('File too large — maximum 10 GB per transfer');
    return;
}
```
**Recovery:** User splits file or uses desktop-to-desktop path (no limit)

### T-09: Transfer Timeout
**Trigger:** No progress for 60 seconds  
**Go handling:** Cancel transfer, delete partial file, emit error  
**UI message:** "Transfer timed out — no data received for 60 seconds"

---

## 4. Gesture Errors

### G-01: Camera Permission Denied
**Trigger:** `getUserMedia` throws `NotAllowedError`  
**Browser handling:**
```javascript
try {
    stream = await navigator.mediaDevices.getUserMedia({ video: true });
} catch (e) {
    if (e.name === 'NotAllowedError') {
        UI.setScreen('no-camera');
        // Show instructions for granting permission
    }
}
```
**UI behaviour:** Show persistent "Camera access needed for gesture control" banner with "Open Settings" button  
**Recovery:** Grant permission, reload app

### G-02: MediaPipe Model Download Fails (first run, offline)
**Trigger:** `HandLandmarker.createFromOptions` fails (CDN unreachable)  
**Handling:**
```typescript
try {
    await tracker.initialize();
} catch (e) {
    console.warn('[gesture] MediaPipe unavailable:', e);
    // Fall back to button-only UI — gestures optional
    gestureStore.set({ ...defaultState, available: false });
    UI.showBanner('Gesture control unavailable — use buttons instead', 'warn');
}
```
**UI behaviour:** Amber banner, all transfer actions available via buttons  
**Recovery:** Connect to internet once to cache the model, then works offline

### G-03: MediaPipe WASM Not Supported
**Trigger:** Browser doesn't support WebAssembly (very rare)  
**Handling:** Same as G-02 — gesture control disabled, button fallback active

### G-04: False Positive Gesture (accidental trigger)
**Trigger:** Hand movement accidentally confirms GRAB or OPEN_PALM  
**Prevention:** 8-frame debounce + require 267ms hold  
**Additional guard:** Only trigger GRAB when `AppScreen === 'ready'`. Only trigger OPEN_PALM → send when a file is already selected.  
**Recovery:** User closes file picker if opened accidentally

---

## 5. IPC Errors

### I-01: Go Sidecar Crashes
**Trigger:** Go sidecar process exits unexpectedly  
**Rust handling:**
```rust
// In sidecar/mod.rs — monitor child process
child.wait().await;
// Restart up to 3 times
// After 3 failures: emit fatal error to frontend
window.emit_all("EVT_ERROR", json!({
    "code": "SIDECAR_CRASH",
    "message": "Networking service crashed — restart the app",
    "fatal": true
}));
```
**UI behaviour:** Full-screen error with "Restart GestureShare" button

### I-02: Go Sidecar Not Found
**Trigger:** Binary not present in expected location  
**Rust handling:** Log error + show error screen before opening main window  
**UI message:** "Networking binary not found. Please reinstall GestureShare."

### I-03: Malformed IPC Message
**Trigger:** JSON parse error on a message from Go  
**Rust handling:**
```rust
if let Ok(msg) = serde_json::from_str::<Value>(&line) {
    // process
} else {
    log::warn!("[sidecar] malformed JSON: {}", &line[..50.min(line.len())]);
    // Skip message — don't crash
}
```
**Recovery:** Automatic — bad messages are skipped

---

## 6. Security Error UI Specifications

Security errors require special UI treatment — they must be impossible to miss.

### CERT_MISMATCH
```
┌─────────────────────────────────────────┐
│  ⊘  Security Alert                      │
│                                         │
│  Certificate fingerprint mismatch       │
│                                         │
│  The device you're connecting to is     │
│  NOT the one that generated the QR.     │
│  Someone may be intercepting your       │
│  connection.                            │
│                                         │
│  No data was transmitted.               │
│                                         │
│  Expected: a3f8b2c1...                  │
│  Received: deadbeef...                  │
│                                         │
│         [ Scan QR Again ]               │
└─────────────────────────────────────────┘
Background: red tint
Only action: scan QR again (no dismiss/ignore)
```

### HMAC_FAIL
```
Banner (red, persistent):
⚠ Transfer rejected — file may have been tampered with in transit.
The file was NOT saved. [Details ▾]

Details (expandable):
Transfer ID: <id>
File: <filename>
HMAC verification: FAILED
Action taken: file discarded, no data written to disk
```

---

## 7. Error Recovery Matrix

| Error | Auto-retry? | User action needed | Session survives? |
|-------|------------|-------------------|------------------|
| MDNS_BIND_FAIL | No | Use QR/text code | Yes |
| PEER_UNREACHABLE | Yes (3x) | Reconnect if persists | Yes |
| CERT_MISMATCH | No | Scan QR again | No — new session |
| SESSION_EXPIRED | No | Scan QR again | No — new session |
| HMAC_FAIL | No | Resend from sender | Yes |
| DECRYPT_FAIL | No | Re-pair | No — new session |
| CANCELLED | No | Nothing | Yes |
| FILE_NOT_FOUND | No | Select file again | Yes |
| DISK_FULL | No | Free space | Yes |
| SIDECAR_CRASH | Yes (3x) | Restart app | No |
| CAMERA_DENIED | No | Grant permission | Yes (buttons work) |
