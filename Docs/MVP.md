# GestureShare — MVP Definition

**Version:** 1.0  
**Status:** Active  
**Sprint Target:** 8 weeks to shippable MVP  
**Last Updated:** 2026-03-06

---

## 1. MVP Philosophy

> Build the thinnest vertical slice that proves the core value proposition end-to-end.

The core value proposition is: **two devices, no internet, file arrives encrypted in seconds.**

Everything else — gestures, phantom UI, clipboard, Mini-Mode — is enhancement. The MVP proves the security model and transfer pipeline are real. The magic gets layered on top.

---

## 2. MVP Scope: What's In

### Must Ship (MVP Blockers)

| Feature | Why It's in MVP | Acceptance Criteria |
|---------|----------------|---------------------|
| LAN peer discovery (mDNS) | Core connection method | Two instances find each other on same WiFi within 5 seconds |
| QR code pairing | Phone connection without typing | Phone scans QR, session established in < 5 seconds |
| Zero-trust P-521 ECDH handshake | Entire security foundation | TLS cert fingerprint verified; connection rejected if mismatch |
| AES-256-CTR file encryption | Core security promise | File bytes never transmitted unencrypted |
| HMAC-SHA256 integrity check | Prevent tampered files | Transfer rejected if HMAC fails |
| Desktop → Phone file transfer | Primary use case | Phone receives and saves file to Downloads |
| Phone → Desktop file transfer | Reverse direction | Desktop receives and saves file |
| Browser client (index.html) | Zero install on phone | Works on Safari iOS + Chrome Android |
| Progress feedback | User confidence | Real-time percent + MB/s shown on both devices |
| Basic Tauri desktop UI | Desktop shell | File picker, peer list, transfer status |

### Should Ship (MVP Quality)

| Feature | Rationale |
|---------|-----------|
| 6-digit text code pairing | Needed for laptop-to-laptop without phone camera |
| Transfer cancellation | Basic UX hygiene |
| File already exists handling | Avoid silent overwrites |
| Session audit log | Proves the security model to technical users |
| Incoming file accept/reject | User must consent to receive |

---

## 3. MVP Scope: What's Out

| Feature | Reason Deferred | Target Phase |
|---------|----------------|--------------|
| Hand gesture control | Additive — transfers work without it | Phase 2 |
| Phantom hand 3D overlay | Pure enhancement | Phase 2 |
| File orb animation | Enhancement | Phase 2 |
| Clipboard sync | Nice-to-have | Phase 2 |
| Mini-Mode | Enhancement | Phase 2 |
| Glassmorphism UI polish | Enhancement | Phase 2 |
| Folder / directory transfer | Complexity | Phase 3 |
| Transfer resume on disconnect | Complexity | Phase 3 |
| Transfer history persistence | Scope creep | Phase 3 |
| Multiple simultaneous transfers | Complexity | Phase 3 |
| Windows installer / portable binary | Distribution | Post-MVP |

---

## 4. MVP User Flows

### Flow A: Phone Receives File from Desktop

```
1. User launches GestureShare on desktop
2. App generates P-521 keypair + session ID + self-signed TLS cert
3. App displays QR code on screen
4. User opens phone camera, scans QR
5. Phone browser opens https://<local-ip>:<port>/join
6. Browser parses hash fragment (key, cert fingerprint, session ID)
7. Browser verifies TLS cert fingerprint matches QR value
8. Browser generates P-521 keypair via WebCrypto
9. ECDH handshake completes → AES-256-CTR key derived on both sides
10. Desktop UI shows "Phone connected"
11. User clicks "Select file" on desktop → native file picker opens
12. User selects file
13. File encrypted on desktop → streamed to phone via HTTPS
14. Phone browser shows progress ring + MB/s
15. File saved to phone Downloads
16. Both devices show "Transfer complete"
```

### Flow B: Phone Sends File to Desktop

```
1-9. Same as Flow A (session established)
10. Phone user taps "Drop file or tap to browse"
11. Phone file picker opens (native, triggered by browser)
12. User selects photo/document
13. File encrypted IN BROWSER via WebCrypto before upload
14. Encrypted bytes POSTed to desktop HTTPS server
15. Desktop verifies HMAC → decrypts → saves to Downloads
16. Desktop notifies Tauri frontend via IPC event
17. Both devices show "Transfer complete"
```

### Flow C: Desktop-to-Desktop (Text Code)

```
1. Desktop A launches, displays 6-digit code (e.g., 847291) + broadcasts via mDNS
2. User on Desktop B opens GestureShare, sees "Enter code" tab
3. User types 847291
4. Desktop B resolves code → IP via mDNS lookup
5. HTTPS handshake + ECDH session established
6. Either device can now send files to the other via raw TCP
```

---

## 5. MVP Technical Milestones

### Milestone 1 — Skeleton (End of Week 1)
- [ ] Tauri app launches and shows basic window
- [ ] Go sidecar spawns and IPC messages flow (stdin/stdout JSON)
- [ ] SvelteKit frontend renders inside Tauri WebView
- [ ] `CMD_DISCOVER` → Go → mDNS scan → `EVT_PEER_FOUND` → Svelte store updates

**Test:** Two terminals running the app see each other's names in the peer list.

### Milestone 2 — Secure Handshake (End of Week 2)
- [ ] Go generates P-521 keypair on launch
- [ ] Go generates self-signed TLS cert, computes SHA-256 fingerprint
- [ ] Go starts HTTPS server on port 47291
- [ ] QR code generated and displayed in Tauri UI
- [ ] Browser opens URL, parses hash fragment
- [ ] Browser verifies cert fingerprint
- [ ] Browser completes ECDH, registers session via POST
- [ ] Desktop receives browser's public key, completes ECDH
- [ ] Both sides derive identical AES-256-CTR key

**Test:** Open browser console, confirm `Crypto.hasSession === true` after scanning QR.

### Milestone 3 — File Transfer (End of Week 3)
- [ ] Desktop can send file to connected phone browser
- [ ] Phone browser encrypts file, POSTs to desktop
- [ ] Desktop verifies HMAC, decrypts, saves to Downloads
- [ ] Progress updates flow in real-time (percent, MB/s)
- [ ] Transfer of 100MB file completes without error

**Test:** Transfer a 100MB video from phone to desktop. Verify file appears in Downloads and is byte-identical to original (sha256 check).

### Milestone 4 — Desktop-to-Desktop (End of Week 4)
- [ ] 6-digit code pairing works
- [ ] Raw TCP sender/receiver working
- [ ] AES-256-CTR applied over TCP stream
- [ ] Transfer speed ≥ 40 MB/s on gigabit LAN
- [ ] Both directions work

**Test:** Transfer 1GB file between two machines. Speed ≥ 40 MB/s. SHA-256 match.

### Milestone 5 — UX Completeness (End of Week 5)
- [ ] Incoming file accept/reject flow
- [ ] Transfer cancellation
- [ ] File collision handling
- [ ] Session audit log displayed in UI
- [ ] Error states handled gracefully (connection lost, bad HMAC, etc.)
- [ ] Basic Tauri UI — peer list, connection status, transfer history

**Test:** User testing session with 3 non-technical users.

---

## 6. MVP Definition of Done

A milestone is "done" when:

1. **Functional**: The described behavior works end-to-end on all three target OSes
2. **Secure**: No plaintext file data leaves the sender unencrypted (verified by packet capture)
3. **Tested**: Manual test protocol completed and documented
4. **Clean**: Code reviewed, no TODO/panic/unwrap without comment

The MVP ships when all 5 milestones are done and a 1GB file transfer completes in under 30 seconds on a gigabit LAN.

---

## 7. MVP Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| WebCrypto P-521 not supported on older iOS | Low | High | Test on iOS 15 Safari in Week 2; fallback plan: P-256 |
| MediaPipe WASM performance on low-end devices | Medium | Medium | Profile in Week 6; gesture control is post-MVP anyway |
| TLS cert verification blocked by browser security policy | Medium | High | Test in Week 2; if blocked, use certificate pinning via API |
| Raw TCP speed doesn't hit 40 MB/s target | Low | Medium | Benchmark in Week 4; fallback: increase buffer sizes |
| Go sidecar cross-compilation fails on Windows | Low | Medium | Test CI pipeline in Week 1 |
| mDNS blocked on enterprise WiFi | High | Low | Document known limitation; text code works as fallback |

---

## 8. MVP Success Criteria

The MVP is successful if after 5 weeks:

- A developer on macOS can transfer a 1GB file to an iPhone in under 30 seconds
- The same developer can confirm (via Wireshark packet capture) that zero plaintext file bytes crossed the network
- A non-technical user can connect a phone to the desktop by scanning QR in under 5 seconds without any instructions
- The security audit log shows all 5 handshake steps completed
