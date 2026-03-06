# GestureShare — Product Requirements Document (PRD)

**Version:** 1.0  
**Status:** Draft  
**Owner:** Product  
**Last Updated:** 2026-03-06

---

## 1. Executive Summary

GestureShare is a desktop application that enables lightning-fast, end-to-end encrypted peer-to-peer file sharing over a local network using hand gestures as the primary interaction paradigm. Users close their fist to select a file and open their palm to send it — no buttons, no menus, no cloud. Any device (phone, tablet, another PC) can receive files by scanning a QR code and opening a browser, with zero app installation required on the receiving end.

---

## 2. Problem Statement

### 2.1 Current Pain Points

Local file sharing in 2026 remains fragmented and friction-heavy:

- **Platform lock-in**: AirDrop works only between Apple devices
- **Privacy concerns**: Tools like WeTransfer, Google Drive route files through foreign servers
- **Installation friction**: LocalSend, Snapdrop require app installs on every receiving device
- **No security baseline**: Most LAN tools assume the local network is trusted — it isn't
- **Speed caps**: Browser-based tools are throttled by WebRTC overhead (~20-30 MB/s ceiling)
- **No gesture interface**: Every existing tool requires UI interaction; none leverage spatial computing

### 2.2 Opportunity

There is no tool that simultaneously offers:
- Cross-platform support (Windows, macOS, Linux sending; any browser receiving)
- Zero install on receiving device
- Genuine end-to-end encryption assuming a hostile local network
- 40-100 MB/s LAN transfer speed
- Gesture-based spatial interaction

---

## 3. Goals and Non-Goals

### 3.1 Goals

| Priority | Goal |
|----------|------|
| P0 | Peer-to-peer file transfer over LAN at 40-100 MB/s |
| P0 | End-to-end encryption with zero cloud dependency |
| P0 | Any browser on any device can receive without installing anything |
| P0 | Three connection methods: QR code, 6-digit text code, mDNS auto-discovery |
| P1 | Hand gesture control: closed fist to select, open palm to send/accept |
| P1 | Phantom 3D hand skeleton overlay with file orb animation |
| P1 | Universal clipboard sync between desktop and connected devices |
| P1 | Glassmorphism UI with Mini-Mode for unobtrusive multitasking |
| P2 | Portable binary (.exe) and standard installer distribution |
| P2 | Transfer history and per-session audit log |
| P2 | Multi-file queue and folder transfer |

### 3.2 Non-Goals

- Cloud backup or sync (Dropbox-style)
- Internet-based file transfer (WAN)
- Video/audio streaming
- Mobile native app (browser-only by design)
- User accounts or authentication backend

---

## 4. Target Users

### 4.1 Primary: Power Users / Developers

- Transfer build artifacts, screenshots, logs between workstations constantly
- Value security — suspicious of tools that phone home
- Comfortable with technical UX, appreciate the gesture novelty
- Run Windows/macOS/Linux mix in their environment

### 4.2 Secondary: Creative Professionals

- Transfer large video files, RAW photos between devices frequently
- Speed is critical — 100MB+ files are routine
- Value aesthetic quality in the tools they use

### 4.3 Tertiary: Privacy-Conscious General Users

- Want to share files between phone and PC without using cloud services
- No technical knowledge required — QR scan just works

---

## 5. User Stories

### Connection

- As a user, I want to scan a QR code on my phone so that I can connect to the desktop app in under 5 seconds without typing anything
- As a user, I want to type a 6-digit code so that I can connect two laptops without using a phone camera
- As a user, I want nearby devices running GestureShare to appear automatically so that I don't need to do any pairing step on the same trusted network

### Security

- As a security-conscious user, I want my files encrypted before they leave my device so that even my router cannot see the contents
- As a user, I want the connection to fail visibly if someone is intercepting my traffic so that I'm never tricked into sending to the wrong device
- As a user, I want session keys to be wiped when I close the app so that there's no persistent key material on disk

### File Transfer

- As a sender, I want to close my fist to trigger a file picker so that I can select a file using only gestures
- As a sender, I want to open my palm to initiate a transfer so that sending feels like physically throwing the file
- As a receiver on my phone, I want to see a real-time progress ring with MB/s speed so that I know the transfer is working
- As a user, I want to transfer a 1GB file in under 30 seconds on a gigabit network

### Clipboard

- As a user, I want to paste a link on my desktop and have it appear on my phone instantly so that I can continue browsing on mobile
- As a user, I want to copy text on my phone and have it sync to my desktop clipboard so that I can paste it into documents

### UI

- As a user, I want a Mini-Mode window so that GestureShare stays accessible without covering my work
- As a user, I want to see my hand as a glowing phantom skeleton so that gesture detection feels spatial and magical

---

## 6. Functional Requirements

### 6.1 Connection Layer

| ID | Requirement | Priority |
|----|-------------|----------|
| CON-01 | App advertises itself via mDNS (`_gestureshare._tcp`) on launch | P0 |
| CON-02 | App discovers peers via mDNS, updates peer list every 3 seconds | P0 |
| CON-03 | QR code encodes HTTPS URL with P-521 pubkey, TLS cert fingerprint, session ID in hash fragment | P0 |
| CON-04 | Hash fragment never transmitted over network (browser-only parsing) | P0 |
| CON-05 | 6-digit pairing code broadcast via mDNS TXT record | P0 |
| CON-06 | TLS certificate fingerprint verified client-side against QR value | P0 |
| CON-07 | Connection rejected if cert fingerprint does not match | P0 |

### 6.2 Security Layer

| ID | Requirement | Priority |
|----|-------------|----------|
| SEC-01 | P-521 ECDH keypair generated fresh on every app launch | P0 |
| SEC-02 | HKDF-SHA512 used to derive AES-256-CTR key and HMAC-SHA256 key from shared secret | P0 |
| SEC-03 | Files encrypted client-side (in browser) before upload | P0 |
| SEC-04 | HMAC-SHA256 computed over full ciphertext, verified before decryption | P0 |
| SEC-05 | TLS 1.3 minimum; no TLS 1.2 or below | P0 |
| SEC-06 | Session keys wiped from memory on app exit | P0 |
| SEC-07 | WebCrypto keys are non-extractable (browser client) | P0 |
| SEC-08 | No key material written to disk at any point | P0 |

### 6.3 Transfer Layer

| ID | Requirement | Priority |
|----|-------------|----------|
| TRX-01 | Desktop-to-desktop transfer via raw TCP socket | P0 |
| TRX-02 | Browser-to-desktop transfer via HTTPS POST with streaming support | P0 |
| TRX-03 | AES-256-CTR stream cipher applied before TCP/HTTPS send | P0 |
| TRX-04 | Transfer speed ≥ 40 MB/s on gigabit LAN | P0 |
| TRX-05 | Real-time progress events: bytes sent, MB/s, ETA | P0 |
| TRX-06 | HMAC verification before file is written to disk | P0 |
| TRX-07 | Multiple files queued and sent sequentially | P1 |
| TRX-08 | Transfer can be cancelled mid-flight | P1 |

### 6.4 Gesture Layer

| ID | Requirement | Priority |
|----|-------------|----------|
| GES-01 | MediaPipe Hands runs offline via WebAssembly, 21 landmarks per hand | P1 |
| GES-02 | Closed fist (all fingertips within threshold of palm) → GRAB gesture | P1 |
| GES-03 | Open palm (all fingertips extended) → SEND/ACCEPT gesture | P1 |
| GES-04 | Gesture must hold for 8 frames (~267ms) before confirmed — debouncing | P1 |
| GES-05 | Phantom hand skeleton overlay rendered in Three.js on transparent canvas | P1 |
| GES-06 | Skeleton color changes: cyan=idle, orange=grab, green=send | P1 |
| GES-07 | File orb attaches to palm centroid on GRAB, flies off on SEND | P1 |

### 6.5 UI Layer

| ID | Requirement | Priority |
|----|-------------|----------|
| UI-01 | Glassmorphism design: backdrop-filter blur, translucent panels | P1 |
| UI-02 | Mini-Mode: compact always-on-top window for multitasking | P1 |
| UI-03 | Progress ring animation with speed and ETA statistics | P0 |
| UI-04 | Zero-trust handshake step indicators (QR → Cert → ECDH → HKDF → E2EE) | P0 |
| UI-05 | Live audit log showing every cryptographic operation with timestamp | P1 |
| UI-06 | Clipboard textarea on browser client with two-way sync | P1 |
| UI-07 | Incoming file toast notification on receiving device | P0 |

---

## 7. Non-Functional Requirements

| Category | Requirement |
|----------|-------------|
| Performance | File transfer ≥ 40 MB/s on gigabit LAN, desktop-to-desktop |
| Performance | Gesture classification latency < 33ms (one frame at 30fps) |
| Performance | App binary size < 20MB (Tauri target) |
| Security | Zero bytes of plaintext file data ever transmitted unencrypted |
| Security | No network requests to any external server at any time |
| Security | TLS 1.3 only; cipher suite: AES-256-GCM or CHACHA20-POLY1305 |
| Reliability | Transfer resumes or retries on connection drop (Phase 2) |
| Compatibility | Desktop: Windows 10+, macOS 12+, Ubuntu 20.04+ |
| Compatibility | Browser client: Safari iOS 15+, Chrome 90+, Firefox 90+ |
| Privacy | No telemetry, no analytics, no network calls outside LAN |

---

## 8. Success Metrics

| Metric | Target |
|--------|--------|
| Time to first file sent (from launch) | < 30 seconds |
| QR scan → session active | < 5 seconds |
| Transfer speed (1GB file, gigabit LAN) | ≥ 40 MB/s |
| Gesture detection false positive rate | < 2% of frames |
| App cold start time | < 2 seconds |
| Browser client load time (LAN) | < 1 second |

---

## 9. Open Questions

1. Should Mini-Mode show gesture camera feed or just transfer status?
2. Is folder/directory transfer in scope for MVP or Phase 2?
3. Should the 6-digit code expire after a timeout (e.g., 60 seconds)?
4. Do we support simultaneous transfers (multiple senders to one receiver)?
5. Should transfer history persist across sessions (requires encrypted local storage)?

---

## 10. Out of Scope (v1.0)

- WAN / relay server for transfers outside local network
- Mobile native app (iOS/Android)
- File transfer resume after full disconnect
- Directory tree transfer
- Peer-to-peer chat / messaging
- Video preview of transferred media
