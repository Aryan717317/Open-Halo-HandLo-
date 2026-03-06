# GestureShare

> Peer-to-peer encrypted file sharing via hand gestures. No internet. No servers. Just magic.

## Stack
- **Frontend**: SvelteKit + Three.js + MediaPipe Hands
- **Desktop Shell**: Tauri (Rust)
- **Networking Core**: Go sidecar (pion/webrtc + hashicorp/mdns)
- **Encryption**: ECDH (Curve25519) + AES-256-GCM

## Connection Methods
1. **QR Code** — scan to pair instantly
2. **Text Code** — 6-digit code for keyboard entry  
3. **LAN Auto-Discovery** — mDNS, zero config

## Gesture Controls
- Closed fist → select file
- Open palm   → send / accept file

## Quick Start
  ./scripts/setup.sh   # install deps
  ./scripts/dev.sh     # development mode
  ./scripts/build.sh   # production build
