# GestureShare — Testing Strategy & Test Cases

**Version:** 1.0  
**Purpose:** Complete testing guide for agents and developers. Covers unit tests, integration tests, security validation tests, and performance benchmarks.

---

## 1. Testing Philosophy

Each layer has its own testing strategy:

| Layer | Strategy | Tools |
|-------|----------|-------|
| Go crypto functions | Unit tests — pure functions, deterministic | `go test` |
| Go IPC router | Unit tests with mock stdin/stdout | `go test` |
| Go transfer engine | Integration tests with real TCP sockets | `go test -tags integration` |
| Go mDNS | Manual test (requires real network) | Shell scripts |
| Rust Tauri commands | Unit tests on pure logic | `cargo test` |
| SvelteKit components | Unit tests for stores + classifiers | Vitest |
| Gesture classifier | Accuracy tests with recorded landmark data | Vitest |
| Browser crypto | Unit tests in Node.js WebCrypto | Vitest |
| Full E2E | Manual test protocol per phase | Test scripts |
| Security | Adversarial tests (MITM, tamper, replay) | Manual + Wireshark |
| Performance | Benchmark scripts | Shell + Go benchmarks |

---

## 2. Go Unit Tests

### 2.1 Crypto Tests

File: `sidecar/crypto/ctr_test.go`
```go
package crypto_test

import (
    "bytes"
    "crypto/rand"
    "testing"
    "github.com/gestureshare/sidecar/crypto"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
    key := make([]byte, 32)
    rand.Read(key)

    plaintext := []byte("Hello, GestureShare! This is a test payload.")

    encrypted, err := crypto.EncryptCTR(plaintext, key)
    if err != nil { t.Fatalf("encrypt: %v", err) }

    // Encrypted must be longer than plaintext (counter prepended)
    if len(encrypted) <= len(plaintext) {
        t.Fatal("encrypted output should be longer than plaintext")
    }

    decrypted, err := crypto.DecryptCTR(encrypted, key)
    if err != nil { t.Fatalf("decrypt: %v", err) }

    if !bytes.Equal(plaintext, decrypted) {
        t.Fatalf("roundtrip failed: got %q, want %q", decrypted, plaintext)
    }
}

func TestEncryptProducesUniqueCiphertexts(t *testing.T) {
    key := make([]byte, 32)
    rand.Read(key)
    plaintext := bytes.Repeat([]byte("A"), 1024)

    enc1, _ := crypto.EncryptCTR(plaintext, key)
    enc2, _ := crypto.EncryptCTR(plaintext, key)

    // Same plaintext + same key must produce different ciphertext (random counter)
    if bytes.Equal(enc1, enc2) {
        t.Fatal("identical ciphertexts for same plaintext — counter not random!")
    }
}

func TestDecryptWrongKeyFails(t *testing.T) {
    key1 := make([]byte, 32); rand.Read(key1)
    key2 := make([]byte, 32); rand.Read(key2)
    plaintext := []byte("secret data")

    encrypted, _ := crypto.EncryptCTR(plaintext, key1)
    decrypted, err := crypto.DecryptCTR(encrypted, key2)
    if err != nil { return } // some impls return error — fine

    // If no error, output must not match plaintext
    if bytes.Equal(decrypted, plaintext) {
        t.Fatal("wrong key produced correct plaintext — critical bug!")
    }
}

func TestEncryptShortData(t *testing.T) {
    key := make([]byte, 32); rand.Read(key)
    for _, size := range []int{0, 1, 15, 16, 17, 63, 64, 65} {
        plain := make([]byte, size)
        rand.Read(plain)
        enc, err := crypto.EncryptCTR(plain, key)
        if err != nil { t.Fatalf("size %d: encrypt error: %v", size, err) }
        dec, err := crypto.DecryptCTR(enc, key)
        if err != nil { t.Fatalf("size %d: decrypt error: %v", size, err) }
        if !bytes.Equal(plain, dec) { t.Fatalf("size %d: roundtrip mismatch", size) }
    }
}

func TestHMACSignVerify(t *testing.T) {
    key := make([]byte, 32); rand.Read(key)
    data := []byte("file contents here")

    sig := crypto.SignHMAC(data, key)
    if !crypto.VerifyHMAC(data, sig, key) {
        t.Fatal("valid HMAC failed verification")
    }

    // Tamper with data
    data[0] ^= 0xFF
    if crypto.VerifyHMAC(data, sig, key) {
        t.Fatal("tampered data passed HMAC verification — critical bug!")
    }
}

func TestHKDFDeterministic(t *testing.T) {
    sharedBits := make([]byte, 66); rand.Read(sharedBits)
    sessionID := "a3f8b2c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1"

    key1, err := crypto.DeriveAESKey(sharedBits, sessionID)
    if err != nil { t.Fatal(err) }
    key2, err := crypto.DeriveAESKey(sharedBits, sessionID)
    if err != nil { t.Fatal(err) }

    if !bytes.Equal(key1, key2) {
        t.Fatal("HKDF not deterministic for same inputs")
    }
    if len(key1) != 32 {
        t.Fatalf("expected 32-byte key, got %d", len(key1))
    }
}

func TestAESAndHMACKeysDifferent(t *testing.T) {
    sharedBits := make([]byte, 66); rand.Read(sharedBits)
    sid := "0000000000000000000000000000000000000000000000000000000000000000"

    aesKey, _  := crypto.DeriveAESKey(sharedBits, sid)
    hmacKey, _ := crypto.DeriveHMACKey(sharedBits, sid)

    if bytes.Equal(aesKey, hmacKey) {
        t.Fatal("AES and HMAC keys are identical — domain separation failed!")
    }
}

// Benchmark: encryption throughput
func BenchmarkEncryptCTR_64KB(b *testing.B) {
    key := make([]byte, 32); rand.Read(key)
    chunk := make([]byte, 64*1024); rand.Read(chunk)
    b.SetBytes(int64(len(chunk)))
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        crypto.EncryptCTR(chunk, key)
    }
}
```

### 2.2 ECDH Tests

File: `sidecar/crypto/ecdh_test.go`
```go
func TestECDHProducesMatchingKeys(t *testing.T) {
    // Simulate two sides doing ECDH
    // Side A
    curveA := ecdh.P521()
    privA, _ := curveA.GenerateKey(rand.Reader)

    // Side B
    curveB := ecdh.P521()
    privB, _ := curveB.GenerateKey(rand.Reader)

    // A computes shared secret using B's public key
    sharedA, _ := privA.ECDH(privB.PublicKey())

    // B computes shared secret using A's public key
    sharedB, _ := privB.ECDH(privA.PublicKey())

    if !bytes.Equal(sharedA, sharedB) {
        t.Fatal("ECDH: sides derived different shared secrets")
    }

    // Derive AES keys from both sides
    sid := "deadbeef00000000000000000000000000000000000000000000000000000000"
    keyA, _ := crypto.DeriveAESKey(sharedA, sid)
    keyB, _ := crypto.DeriveAESKey(sharedB, sid)

    if !bytes.Equal(keyA, keyB) {
        t.Fatal("HKDF: sides derived different AES keys")
    }
}
```

### 2.3 TCP Transfer Tests

File: `sidecar/transfer/transfer_test.go`
```go
// +build integration

func TestTCPTransferRoundtrip(t *testing.T) {
    key := make([]byte, 32); rand.Read(key)

    // Create 10MB test payload
    original := make([]byte, 10*1024*1024)
    rand.Read(original)
    originalHash := sha256.Sum256(original)

    // Write to temp file
    srcFile, _ := os.CreateTemp("", "gs-test-*.bin")
    srcFile.Write(original)
    srcFile.Close()

    dstFile, _ := os.CreateTemp("", "gs-recv-*.bin")
    dstPath := dstFile.Name()
    dstFile.Close()
    os.Remove(dstPath) // sender will create it

    // Start receiver
    received := make(chan error, 1)
    go func() {
        received <- transfer.ReceiveFile(":47399", key, dstPath)
    }()

    time.Sleep(50 * time.Millisecond) // let receiver start

    // Send
    err := transfer.SendFile("127.0.0.1:47399", key, srcFile.Name(), nil)
    if err != nil { t.Fatalf("send: %v", err) }

    if err := <-received; err != nil { t.Fatalf("receive: %v", err) }

    // Verify integrity
    dstData, _ := os.ReadFile(dstPath)
    dstHash := sha256.Sum256(dstData)
    if originalHash != dstHash {
        t.Fatal("SHA-256 mismatch — data corruption!")
    }

    // Cleanup
    os.Remove(srcFile.Name())
    os.Remove(dstPath)
}
```

---

## 3. TypeScript Unit Tests (Vitest)

### 3.1 Gesture Classifier Tests

File: `apps/desktop/frontend/src/lib/gesture/GestureClassifier.test.ts`
```typescript
import { describe, it, expect, beforeEach } from 'vitest';
import { GestureClassifier, Gesture } from './GestureClassifier';

// Helper: create 21 landmarks all at origin
function flatLandmarks(): NormalizedLandmark[] {
    return Array.from({ length: 21 }, () => ({ x: 0.5, y: 0.5, z: 0 }));
}

// Helper: simulate open palm — fingertips far from wrist
function openPalmLandmarks(): NormalizedLandmark[] {
    const lm = flatLandmarks();
    lm[0] = { x: 0.5, y: 0.8, z: 0 };  // wrist low
    // Fingertips high — far from wrist
    [4, 8, 12, 16, 20].forEach(i => { lm[i] = { x: 0.5, y: 0.4, z: 0 }; });
    return lm;
}

// Helper: simulate closed fist — fingertips close to wrist
function closedFistLandmarks(): NormalizedLandmark[] {
    const lm = flatLandmarks();
    lm[0] = { x: 0.5, y: 0.5, z: 0 };  // wrist center
    // Fingertips very close to wrist
    [4, 8, 12, 16, 20].forEach(i => { lm[i] = { x: 0.5, y: 0.52, z: 0 }; });
    return lm;
}

describe('GestureClassifier', () => {
    let classifier: GestureClassifier;

    beforeEach(() => { classifier = new GestureClassifier(); });

    it('returns IDLE for neutral hand position', () => {
        const result = classifier.classify(flatLandmarks());
        // May not confirm immediately due to debounce
        expect([Gesture.IDLE, Gesture.OPEN_PALM]).toContain(result);
    });

    it('returns GRAB after 8 frames of closed fist', () => {
        const fist = closedFistLandmarks();
        let result = Gesture.IDLE;
        for (let i = 0; i < 10; i++) {
            result = classifier.classify(fist);
        }
        expect(result).toBe(Gesture.GRAB);
    });

    it('returns OPEN_PALM after 8 frames of open hand', () => {
        const palm = openPalmLandmarks();
        let result = Gesture.IDLE;
        for (let i = 0; i < 10; i++) {
            result = classifier.classify(palm);
        }
        expect(result).toBe(Gesture.OPEN_PALM);
    });

    it('does NOT confirm gesture before 8 frames', () => {
        const fist = closedFistLandmarks();
        const classifier2 = new GestureClassifier();
        let lastResult = Gesture.IDLE;
        for (let i = 0; i < 7; i++) {
            lastResult = classifier2.classify(fist);
        }
        // Should still be IDLE — debounce not yet triggered
        expect(lastResult).toBe(Gesture.IDLE);
    });

    it('resets to IDLE when gesture changes mid-hold', () => {
        const fist = closedFistLandmarks();
        const palm = openPalmLandmarks();
        for (let i = 0; i < 5; i++) classifier.classify(fist);
        for (let i = 0; i < 5; i++) classifier.classify(palm);
        // Neither should be confirmed yet
        expect(classifier.confidence).toBeLessThan(1);
    });

    it('confidence increases per frame', () => {
        const fist = closedFistLandmarks();
        const c1 = classifier.confidence;
        classifier.classify(fist);
        const c2 = classifier.confidence;
        expect(c2).toBeGreaterThanOrEqual(c1);
    });

    it('returns IDLE for empty landmarks', () => {
        expect(classifier.classify([])).toBe(Gesture.IDLE);
    });
});
```

### 3.2 Peer Store Tests

File: `apps/desktop/frontend/src/lib/stores/peerStore.test.ts`
```typescript
import { describe, it, expect } from 'vitest';
import { get } from 'svelte/store';
import { peerStore, peers, hasPeers } from './peerStore';

const mockPeer = {
    id: 'GestureShare-MacBook._gestureshare._tcp.local.',
    name: 'MacBook',
    address: '192.168.1.42',
    port: 47291,
    os: 'darwin',
    code: '123456',
};

describe('peerStore', () => {
    it('starts empty', () => {
        expect(get(peers)).toHaveLength(0);
        expect(get(hasPeers)).toBe(false);
    });

    it('adds peer', () => {
        peerStore.add(mockPeer);
        expect(get(peers)).toHaveLength(1);
        expect(get(hasPeers)).toBe(true);
    });

    it('does not duplicate peers with same ID', () => {
        peerStore.add(mockPeer);
        peerStore.add(mockPeer);
        expect(get(peers)).toHaveLength(1);
    });

    it('removes peer by ID', () => {
        peerStore.add(mockPeer);
        peerStore.remove(mockPeer.id);
        expect(get(peers)).toHaveLength(0);
    });

    it('clears all peers', () => {
        peerStore.add(mockPeer);
        peerStore.add({ ...mockPeer, id: 'other-id' });
        peerStore.clear();
        expect(get(peers)).toHaveLength(0);
    });
});
```

---

## 4. Security Tests (Adversarial)

Run these manually before each release.

### Test S1: MITM Certificate Attack
```bash
# Setup: Machine A = GestureShare desktop, Machine B = attacker, Machine C = phone

# On Machine B (attacker): start a fake HTTPS server with a different cert
# on the same port, respond to /api/cert-ping with a fake fingerprint

# On Machine C: scan QR from Machine A (which has real fingerprint in hash)

# Expected: Browser shows "Certificate fingerprint mismatch" error
# FAIL if: Browser connects to Machine B and proceeds with session
```

### Test S2: Payload Tampering
```bash
# Send a file from phone to desktop
# While transfer is in progress, use a proxy to flip one byte in the body

# Expected: Desktop returns HTTP 400, file is NOT saved to Downloads
# FAIL if: File is saved (even partially) despite tampered HMAC
```

### Test S3: Session Token Replay
```bash
# Complete a session, capture the X-GS-Token value
# Terminate the session (DELETE /api/session)
# Attempt POST /api/transfer/upload with the old token

# Expected: HTTP 401 Unauthorized
# FAIL if: Upload is accepted with expired token
```

### Test S4: QR Hash Fragment Not Transmitted
```bash
# Monitor network traffic with Wireshark while scanning QR
# Filter: ip.addr == <desktop-ip> && http

# Expected: GET /join request has NO query parameters with key= or fp= or sid=
# FAIL if: hash fragment content appears anywhere in captured packets
```

### Test S5: Plaintext File Content Not Transmitted
```bash
# Create a recognizable test file: echo "CANARY_STRING_12345" > /tmp/canary.txt
# Transfer from phone to desktop while capturing with Wireshark

# On captured traffic: strings ./capture.pcap | grep CANARY_STRING_12345

# Expected: Zero matches
# FAIL if: CANARY_STRING_12345 appears in any packet
```

### Test S6: Key Not Written to Disk
```bash
# After a transfer completes, search for key material on disk:
# (Key would be 32 bytes = 64 hex chars)

# Check common locations:
find ~/.gestureshare -type f 2>/dev/null
find /tmp -name "*.key" 2>/dev/null
# On macOS: check Keychain Access for GestureShare entries

# Expected: No key files found
# FAIL if: Any key material found on disk
```

---

## 5. Performance Benchmarks

### Benchmark B1: Desktop-to-Desktop Transfer Speed
```bash
#!/bin/bash
# bench_transfer.sh

# Generate 1GB test file
dd if=/dev/urandom of=/tmp/bench1gb.bin bs=1M count=1024 2>/dev/null
ORIG_SHA=$(sha256sum /tmp/bench1gb.bin | cut -d' ' -f1)

echo "Starting transfer benchmark..."
START=$(date +%s%N)

# Trigger transfer via GestureShare CLI (once implemented)
# or manually select file and send

# Wait for completion signal
WAIT_FOR ~/Downloads/bench1gb.bin

END=$(date +%s%N)
ELAPSED_MS=$(( (END - START) / 1000000 ))
SIZE_BYTES=$(stat -f%z /tmp/bench1gb.bin)
SPEED_MBS=$(echo "scale=1; $SIZE_BYTES / $ELAPSED_MS / 1000" | bc)

echo "Transfer: ${ELAPSED_MS}ms = ${SPEED_MBS} MB/s"

# Verify integrity
RECV_SHA=$(sha256sum ~/Downloads/bench1gb.bin | cut -d' ' -f1)
if [ "$ORIG_SHA" == "$RECV_SHA" ]; then
    echo "✓ SHA-256 match"
else
    echo "✗ SHA-256 MISMATCH — data corruption!"
    exit 1
fi

# Cleanup
rm /tmp/bench1gb.bin ~/Downloads/bench1gb.bin
```

### Benchmark B2: Go Crypto Throughput
```bash
cd sidecar
go test -bench=. -benchtime=10s ./crypto/...

# Expected output:
# BenchmarkEncryptCTR_64KB-8    5000    210000 ns/op    312.50 MB/s
# BenchmarkDecryptCTR_64KB-8    5000    200000 ns/op    327.68 MB/s
# Minimum acceptable: 200 MB/s (will not bottleneck 40-100 MB/s network)
```

### Benchmark B3: Gesture Classification Latency
```bash
# In browser console while app is running with camera active:
const times = [];
const orig = GestureClassifier.prototype.classify;
GestureClassifier.prototype.classify = function(...args) {
    const start = performance.now();
    const result = orig.apply(this, args);
    times.push(performance.now() - start);
    return result;
};

// After 1 minute:
const avg = times.reduce((a,b) => a+b) / times.length;
const p99 = times.sort((a,b) => a-b)[Math.floor(times.length * 0.99)];
console.log(`Avg: ${avg.toFixed(2)}ms, P99: ${p99.toFixed(2)}ms`);

// Expected: Avg < 2ms, P99 < 5ms
```

---

## 6. Manual Test Protocol (Run Before Each Milestone)

Execute in order. Mark each as PASS/FAIL/SKIP.

### MT-01: IPC Smoke Test
- [ ] Launch app → console shows `[sidecar] Ready`
- [ ] Device name appears in UI header

### MT-02: Peer Discovery
- [ ] Two instances on same WiFi → each shows other in peer list within 5s
- [ ] Close one instance → disappears from peer list within 10s

### MT-03: QR Pairing
- [ ] QR code visible in UI
- [ ] Scan on iPhone Safari → browser loads
- [ ] All 5 handshake steps show green checkmarks
- [ ] Desktop shows "Phone connected"

### MT-04: Text Code Pairing
- [ ] 6-digit code visible in UI
- [ ] Enter code on second desktop → session establishes
- [ ] Works when QR scan is not possible

### MT-05: Phone → Desktop Transfer
- [ ] Select file on phone browser
- [ ] Progress ring shows real-time %
- [ ] File appears in ~/Downloads
- [ ] SHA-256 matches original

### MT-06: Desktop → Phone Transfer
- [ ] Select file on desktop
- [ ] Phone shows incoming file toast
- [ ] Accept → file downloads to phone
- [ ] SHA-256 matches original

### MT-07: Transfer Cancellation
- [ ] Start large file transfer
- [ ] Click cancel mid-transfer
- [ ] Partial file NOT saved to Downloads
- [ ] Both devices return to ready state

### MT-08: Security Validation
- [ ] Run S4 (hash fragment not transmitted)
- [ ] Run S5 (plaintext not in packets)
- [ ] Run S2 (tampered payload rejected)

### MT-09: Gesture Control (Phase 5+)
- [ ] Closed fist → file picker opens (no false positives in 60s idle test)
- [ ] Select file → orb appears on palm
- [ ] Open palm → transfer initiates
- [ ] Phantom skeleton renders at 30fps

### MT-10: Performance
- [ ] 1GB desktop-to-desktop transfer ≥ 40 MB/s
- [ ] 100MB phone-to-desktop transfer ≥ 15 MB/s
- [ ] App cold start < 3 seconds
