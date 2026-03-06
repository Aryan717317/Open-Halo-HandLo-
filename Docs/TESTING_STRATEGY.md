# GestureShare — Testing Strategy

**How to verify the project is correct at every layer.**  
**Version:** 1.0 | **Referenced by:** `IMPLEMENTATION_PLAN.md`

---

## 1. Testing Philosophy

This project has three kinds of correctness that cannot be verified the same way:

| Kind | Verification Method | Why |
|------|---------------------|-----|
| Crypto correctness | Unit tests with known vectors | One wrong bit = silent data corruption |
| Protocol correctness | Integration tests with two real processes | IPC mismatches only appear end-to-end |
| Security properties | Packet capture + deliberate attack tests | Cannot be unit tested |

**Rule:** The crypto layer must have 100% unit test coverage. The transfer layer must have integration tests. The gesture layer uses manual testing with a checklist.

---

## 2. Unit Tests

### 2.1 Go — Crypto Package Tests

Every function in `sidecar/crypto/` must have a test. Run with: `go test ./crypto/...`

**File: `sidecar/crypto/ctr_test.go`**

```go
package crypto_test

import (
    "bytes"
    "testing"
    "github.com/gestureshare/sidecar/crypto"
)

// Test round-trip: encrypt then decrypt produces original plaintext
func TestCTRRoundTrip(t *testing.T) {
    key := make([]byte, 32)  // all-zero test key
    plaintext := []byte("the quick brown fox jumps over the lazy dog")
    
    encrypted, err := crypto.EncryptCTR(plaintext, key)
    if err != nil { t.Fatalf("encrypt: %v", err) }
    
    if len(encrypted) != 16+len(plaintext) {
        t.Fatalf("expected %d bytes, got %d", 16+len(plaintext), len(encrypted))
    }
    
    decrypted, err := crypto.DecryptCTR(encrypted, key)
    if err != nil { t.Fatalf("decrypt: %v", err) }
    
    if !bytes.Equal(plaintext, decrypted) {
        t.Fatalf("decrypted != original: got %q", decrypted)
    }
}

// Two encryptions of same plaintext must produce different ciphertexts (different counters)
func TestCTRCounterUniqueness(t *testing.T) {
    key := make([]byte, 32)
    plain := bytes.Repeat([]byte("A"), 1000)
    
    enc1, _ := crypto.EncryptCTR(plain, key)
    enc2, _ := crypto.EncryptCTR(plain, key)
    
    if bytes.Equal(enc1, enc2) {
        t.Fatal("two encryptions produced identical output — counter not random")
    }
}

// Tampered ciphertext should decrypt to wrong bytes (CTR doesn't auth — HMAC does)
func TestCTRTamperDetection(t *testing.T) {
    key := make([]byte, 32)
    plain := []byte("original data")
    enc, _ := crypto.EncryptCTR(plain, key)
    
    enc[20] ^= 0xFF  // flip a byte in the ciphertext
    dec, err := crypto.DecryptCTR(enc, key)
    
    // CTR decryption always succeeds — tamper shows as wrong plaintext
    if err != nil { t.Fatalf("unexpected error: %v", err) }
    if bytes.Equal(dec, plain) {
        t.Fatal("tampered ciphertext decrypted to original — CTR should not auth")
    }
}

// Empty plaintext
func TestCTREmptyPlaintext(t *testing.T) {
    key := make([]byte, 32)
    enc, err := crypto.EncryptCTR([]byte{}, key)
    if err != nil { t.Fatalf("encrypt empty: %v", err) }
    dec, err := crypto.DecryptCTR(enc, key)
    if err != nil { t.Fatalf("decrypt empty: %v", err) }
    if len(dec) != 0 { t.Fatalf("expected empty, got %d bytes", len(dec)) }
}

// Large file (10MB) — verify no truncation
func TestCTRLargePlaintext(t *testing.T) {
    key := make([]byte, 32)
    plain := make([]byte, 10*1024*1024)
    for i := range plain { plain[i] = byte(i % 251) }
    
    enc, _ := crypto.EncryptCTR(plain, key)
    dec, err := crypto.DecryptCTR(enc, key)
    if err != nil { t.Fatalf("%v", err) }
    if !bytes.Equal(plain, dec) { t.Fatal("10MB round trip failed") }
}

// Wrong key produces wrong plaintext
func TestCTRWrongKey(t *testing.T) {
    key1 := make([]byte, 32)
    key2 := make([]byte, 32)
    key2[0] = 0xFF
    plain := []byte("secret")
    
    enc, _ := crypto.EncryptCTR(plain, key1)
    dec, err := crypto.DecryptCTR(enc, key2)
    if err != nil { t.Fatalf("%v", err) }
    if bytes.Equal(dec, plain) { t.Fatal("wrong key decrypted to original plaintext") }
}
```

**File: `sidecar/crypto/hmac_test.go`**

```go
package crypto_test

import (
    "testing"
    "github.com/gestureshare/sidecar/crypto"
)

func TestHMACSignVerify(t *testing.T) {
    key  := make([]byte, 32)
    data := []byte("transfer data here")
    
    sig := crypto.SignHMAC(data, key)
    if !crypto.VerifyHMAC(data, sig, key) {
        t.Fatal("HMAC verify failed on valid signature")
    }
}

func TestHMACTamperDetection(t *testing.T) {
    key  := make([]byte, 32)
    data := []byte("authentic data")
    sig  := crypto.SignHMAC(data, key)
    
    tampered := append([]byte{}, data...)
    tampered[0] ^= 0x01
    
    if crypto.VerifyHMAC(tampered, sig, key) {
        t.Fatal("HMAC accepted tampered data")
    }
}

func TestHMACWrongKey(t *testing.T) {
    key1 := make([]byte, 32)
    key2 := make([]byte, 32)
    key2[0] = 0x01
    data := []byte("data")
    
    sig := crypto.SignHMAC(data, key1)
    if crypto.VerifyHMAC(data, sig, key2) {
        t.Fatal("HMAC accepted wrong key")
    }
}
```

**File: `sidecar/crypto/ecdh_test.go`**

```go
package crypto_test

import (
    "bytes"
    "testing"
    "github.com/gestureshare/sidecar/crypto"
)

// Both sides of ECDH must derive the same key
func TestECDHSharedSecret(t *testing.T) {
    sessionID := "deadbeefcafe" + string(make([]byte, 52)) // 64 hex chars
    
    // Simulate desktop side
    desktopPriv, desktopPub, _ := crypto.GenerateP521KeyPair()
    
    // Simulate browser side  
    browserPriv, browserPub, _ := crypto.GenerateP521KeyPair()
    
    // Desktop: ECDH with browser's pubkey
    desktopShared, _ := crypto.ECDHSharedBits(desktopPriv, browserPub)
    desktopKey, _   := crypto.DeriveAESKey(desktopShared, sessionID)
    
    // Browser: ECDH with desktop's pubkey
    browserShared, _ := crypto.ECDHSharedBits(browserPriv, desktopPub)
    browserKey, _   := crypto.DeriveAESKey(browserShared, sessionID)
    
    // Both must produce identical keys
    if !bytes.Equal(desktopKey, browserKey) {
        t.Fatal("ECDH key derivation mismatch — desktop and browser would use different keys")
    }
}

// AES and HMAC keys must be different (domain separation)
func TestHKDFDomainSeparation(t *testing.T) {
    shared := make([]byte, 66)
    sid    := "aabbccdd" + string(make([]byte, 56))
    
    aesKey, _  := crypto.DeriveAESKey(shared, sid)
    hmacKey, _ := crypto.DeriveHMACKey(shared, sid)
    
    if bytes.Equal(aesKey, hmacKey) {
        t.Fatal("AES and HMAC keys are identical — HKDF domain separation failed")
    }
}
```

### 2.2 Go — Transfer Package Tests

**File: `sidecar/transfer/chunker_test.go`**

```go
// Test that SendFile produces valid packets receivable by a mock receiver
func TestChunkRoundTrip(t *testing.T) {
    // Create temp file with known content
    // Start mock TCP listener
    // Call SendFile with mock listener address
    // Read all packets from listener
    // Verify decrypted content matches original
    // Verify HMAC passes
}
```

### 2.3 TypeScript — Crypto Module Tests

Use Vitest (included with SvelteKit). Run with: `npm test`

**File: `src/lib/gesture/__tests__/GestureClassifier.test.ts`**

```typescript
import { describe, it, expect } from 'vitest'
import { GestureClassifier, Gesture } from '../GestureClassifier'

// Helper: generate 21 landmarks all at origin
function makeLandmarks(overrides: Record<number, {x:number, y:number, z:number}> = {}) {
    return Array.from({ length: 21 }, (_, i) => ({
        x: overrides[i]?.x ?? 0,
        y: overrides[i]?.y ?? 0,
        z: overrides[i]?.z ?? 0,
    }))
}

describe('GestureClassifier', () => {
    it('classifies closed fist as IDLE before hold frames complete', () => {
        const gc = new GestureClassifier()
        const fist = makeLandmarks() // all tips at origin = near palm = GRAB raw
        
        for (let i = 0; i < 7; i++) {
            expect(gc.classify(fist)).toBe(Gesture.IDLE)
        }
    })

    it('confirms GRAB after 8 frames', () => {
        const gc = new GestureClassifier()
        const fist = makeLandmarks()
        
        for (let i = 0; i < 8; i++) gc.classify(fist)
        expect(gc.classify(fist)).toBe(Gesture.GRAB)
    })

    it('resets hold count on gesture change', () => {
        const gc = new GestureClassifier()
        const fist = makeLandmarks()
        
        for (let i = 0; i < 6; i++) gc.classify(fist)
        
        // Switch to open palm
        const palm = makeLandmarks({
            4: {x: 0.2, y: 0.2, z: 0}, 8: {x: 0.2, y: -0.2, z: 0},
            12: {x: 0.3, y: 0, z: 0}, 16: {x: 0.3, y: 0.2, z: 0},
            20: {x: 0.3, y: -0.2, z: 0}
        })
        gc.classify(palm)
        
        // Fist again — should need 8 more frames
        for (let i = 0; i < 7; i++) {
            expect(gc.classify(fist)).not.toBe(Gesture.GRAB)
        }
    })

    it('returns IDLE for empty landmarks', () => {
        const gc = new GestureClassifier()
        expect(gc.classify([])).toBe(Gesture.IDLE)
    })
})
```

---

## 3. Integration Tests

### 3.1 IPC Round-Trip Test

**Purpose:** Verify Rust↔Go IPC works end-to-end before building any feature on top.

```bash
# scripts/test-ipc.sh
#!/bin/bash
set -e

echo "Building Go sidecar..."
cd sidecar && go build -o /tmp/gs-sidecar . && cd ..

echo "Running IPC smoke test..."
# Pipe a CMD_GET_DEVICE_INFO and expect EVT_DEVICE_INFO back
echo '{"type":"CMD_GET_DEVICE_INFO","payload":{}}' | /tmp/gs-sidecar | grep -q "EVT_DEVICE_INFO"
echo "IPC: PASS"

echo '{"type":"CMD_DISCOVER","payload":{}}' | timeout 5 /tmp/gs-sidecar | head -1 | grep -q "EVT_PEER"
echo "Discovery: PASS (or no peers found — check WiFi)"
```

### 3.2 Crypto Cross-Language Test

**Purpose:** Verify that Go's `EncryptCTR` produces output that the browser's WebCrypto can decrypt, and vice versa.

```bash
# scripts/test-crypto-compat.sh
# 1. Go generates a test key + encrypts a known string
# 2. Outputs: key (hex), counter (hex), ciphertext (hex), expected_plaintext
# 3. Browser test page imports these and decrypts with WebCrypto
# 4. Compares decrypted output to expected_plaintext
```

```go
// cmd/crypto-test/main.go — helper binary for cross-language test
package main

import (
    "encoding/hex"
    "fmt"
    "github.com/gestureshare/sidecar/crypto"
)

func main() {
    key := make([]byte, 32)
    plain := []byte("cross-language crypto test vector 12345")
    enc, _ := crypto.EncryptCTR(plain, key)
    
    fmt.Printf("KEY:        %s\n", hex.EncodeToString(key))
    fmt.Printf("COUNTER:    %s\n", hex.EncodeToString(enc[:16]))
    fmt.Printf("CIPHERTEXT: %s\n", hex.EncodeToString(enc[16:]))
    fmt.Printf("PLAINTEXT:  %s\n", string(plain))
}
```

```javascript
// scripts/verify-crypto.mjs — run with: node scripts/verify-crypto.mjs
const KEY_HEX        = "0000000000000000000000000000000000000000000000000000000000000000"
const COUNTER_HEX    = "<output from Go binary>"
const CIPHERTEXT_HEX = "<output from Go binary>"
const EXPECTED       = "cross-language crypto test vector 12345"

const key = await crypto.subtle.importKey(
    "raw",
    hexToBytes(KEY_HEX),
    { name: "AES-CTR", length: 256 },
    false,
    ["decrypt"]
)

const decrypted = await crypto.subtle.decrypt(
    { name: "AES-CTR", counter: hexToBytes(COUNTER_HEX), length: 64 },
    key,
    hexToBytes(CIPHERTEXT_HEX)
)

const result = new TextDecoder().decode(decrypted)
console.assert(result === EXPECTED, `FAIL: got "${result}"`)
console.log("Crypto cross-language: PASS")

function hexToBytes(hex) {
    return Uint8Array.from(hex.match(/.{2}/g), b => parseInt(b, 16))
}
```

### 3.3 Transfer Integration Test

**Purpose:** Full file transfer between two local Go processes.

```bash
# scripts/test-transfer.sh
#!/bin/bash
set -e

# Generate 100MB test file
dd if=/dev/urandom of=/tmp/gs-test-100mb.bin bs=1M count=100 2>/dev/null
EXPECTED=$(sha256sum /tmp/gs-test-100mb.bin | cut -d' ' -f1)

# Start receiver
/tmp/gs-sidecar --test-receive --port 47400 --out /tmp/gs-received.bin &
RECV_PID=$!
sleep 0.5

# Send file
/tmp/gs-sidecar --test-send --host 127.0.0.1 --port 47400 --file /tmp/gs-test-100mb.bin
wait $RECV_PID

# Verify
ACTUAL=$(sha256sum /tmp/gs-received.bin | cut -d' ' -f1)
if [ "$EXPECTED" = "$ACTUAL" ]; then
    echo "Transfer integrity: PASS (SHA-256 match)"
else
    echo "Transfer integrity: FAIL"
    exit 1
fi
```

---

## 4. Security Tests

These must be run before any release. They verify the security model actually holds.

### 4.1 Plaintext Sniff Test

**Verify:** Zero plaintext file bytes appear on the wire.

```bash
# Setup: Wireshark or tcpdump on loopback
sudo tcpdump -i lo -w /tmp/capture.pcap port 47291 &
TCPDUMP_PID=$!

# Run a transfer
./scripts/test-transfer.sh

kill $TCPDUMP_PID

# Search for known plaintext in capture
# Use a file with distinctive content
echo "SECRET_TEST_CONTENT_12345" > /tmp/test-secret.txt

# ... run transfer of test-secret.txt ...

# Check capture for plaintext
strings /tmp/capture.pcap | grep "SECRET_TEST_CONTENT"
# Expected output: (empty — nothing found)
```

### 4.2 HMAC Bypass Test

**Verify:** Tampered upload is rejected.

```bash
# Transfer a file from browser to desktop
# Intercept with mitmproxy, flip one byte in body
# Expect: HTTP 400 response
# Expect: File NOT saved to Downloads

# Using mitmproxy:
mitmproxy --mode transparent --listen-port 8080     --script scripts/tamper-test.py

# tamper-test.py
def response(flow):
    if "/api/transfer/upload" in flow.request.url:
        body = bytearray(flow.request.content)
        body[100] ^= 0xFF  # flip a byte
        flow.request.content = bytes(body)
```

### 4.3 Cert Fingerprint Mismatch Test

**Verify:** Connection fails if TLS cert doesn't match QR fingerprint.

```bash
# Modify the browser client's cert ping response
# (mock the /api/cert-ping endpoint to return wrong fingerprint)
# Open URL in browser
# Expected: browser client shows "Security verification failed"
# Expected: connection does NOT proceed to session registration
```

### 4.4 Expired Session Test

**Verify:** Expired token returns 401.

```go
// In a test: create a session, manually set ExpiresAt to past, try upload
sess.ExpiresAt = time.Now().Add(-time.Minute)
// POST /api/transfer/upload with this session's token
// Expected: 401 Unauthorized
```

---

## 5. Manual Test Checklists

### 5.1 Per-Phase Checklist

Run these after completing each implementation phase.

**Phase 0 — IPC:**
- [ ] App launches without crash
- [ ] Go sidecar starts (check stderr for "[sidecar] Ready")
- [ ] IPC round-trip: CMD_GET_DEVICE_INFO → EVT_DEVICE_INFO
- [ ] Device name appears in SvelteKit UI

**Phase 1 — Discovery:**
- [ ] Two instances on same WiFi see each other within 5s
- [ ] Peer disappears from list within 6s of other app closing
- [ ] Works on WiFi (not just localhost)

**Phase 2 — Handshake:**
- [ ] QR code displays in desktop app
- [ ] iPhone Safari opens URL from QR (taps through cert warning)
- [ ] All 5 handshake steps show green in browser
- [ ] Desktop shows "Phone connected"
- [ ] Wireshark: all traffic is TLS encrypted

**Phase 3 — Transfer:**
- [ ] Send 100MB file from phone to desktop — verify SHA-256 match
- [ ] Send 100MB file from desktop to phone — verify SHA-256 match
- [ ] Progress ring shows real-time percent + MB/s
- [ ] Tampered upload rejected (HMAC fail test)

**Phase 4 — TCP:**
- [ ] Desktop-to-desktop: 1GB file, verify SHA-256
- [ ] Speed ≥ 40 MB/s on gigabit LAN
- [ ] Transfer cancelled mid-flight cleanly (no partial file)

**Phase 5 — Gestures:**
- [ ] GRAB: fist for 267ms → file picker opens
- [ ] OPEN_PALM: after file selected, open palm → transfer starts
- [ ] False positive test: 5 minutes of normal typing — no accidental triggers

**Phase 6 — UI:**
- [ ] Phantom hand visible at 30fps, no visible lag
- [ ] Gesture color changes work (cyan/orange/green)
- [ ] File orb follows palm, flies on send
- [ ] Glassmorphism visible (blur through panel)
- [ ] Mini-Mode toggles and stays on top

### 5.2 Cross-Platform Smoke Test

Run before any release tag.

| Test | macOS | Windows | Linux |
|------|-------|---------|-------|
| App launches | | | |
| Go sidecar spawns | | | |
| mDNS discovery works | | | |
| QR code displays | | | |
| Phone connects via QR | | | |
| File transfer: phone → desktop | | | |
| File transfer: desktop → desktop | | | |
| Speed ≥ 40 MB/s | | | |
| Clipboard sync | | | |
| App closes cleanly | | | |

---

## 6. CI Test Commands

```bash
# All Go tests
cd sidecar && go test ./... -v -race

# Go tests with coverage
cd sidecar && go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html

# TypeScript tests
cd apps/desktop/frontend && npm test

# IPC smoke test
./scripts/test-ipc.sh

# Crypto cross-language test
go run sidecar/cmd/crypto-test/main.go | node scripts/verify-crypto.mjs

# Full transfer test (requires two terminals or single loopback)
./scripts/test-transfer.sh

# Security tests (manual — require Wireshark/mitmproxy)
# See Section 4
```

---

## 7. Benchmark Targets

These must pass before the MVP ships:

```bash
# Go transfer benchmarks
cd sidecar && go test ./transfer/... -bench=. -benchtime=30s

# Expected results (gigabit LAN or loopback):
# BenchmarkTCPSend1GB-8    1    24500000000 ns/op    43.7 MB/s
# BenchmarkEncryptCTR-8  100     12500000 ns/op     82.2 MB/s  (1GB / 12.5s)
# BenchmarkHMACSHA256-8  100      5800000 ns/op    172.4 MB/s  (1GB / 5.8s)

# If BenchmarkTCPSend1GB shows < 40 MB/s:
# → Check TCP socket buffer sizes (should be 4MB)
# → Check that CGO_ENABLED=0 doesn't affect syscall performance
# → Profile with: go test -bench=. -cpuprofile=cpu.prof && go tool pprof cpu.prof
```
