#!/bin/bash
set -e

echo "==> GestureShare Setup"

# Check prerequisites
command -v go >/dev/null 2>&1 || { echo "ERROR: Go not installed. Install from https://go.dev"; exit 1; }
command -v node >/dev/null 2>&1 || { echo "ERROR: Node.js not installed. Install from https://nodejs.org"; exit 1; }
command -v cargo >/dev/null 2>&1 || { echo "ERROR: Rust not installed. Install from https://rustup.rs"; exit 1; }

echo "--> Installing Tauri CLI..."
cargo install tauri-cli 2>/dev/null || echo "(already installed)"

echo "--> Installing Go sidecar dependencies..."
cd sidecar && go mod tidy && cd ..

echo "--> Installing frontend dependencies..."
cd apps/desktop/frontend && npm install && cd ../../..

echo ""
echo "==> Setup complete! Run ./scripts/dev.sh to start"
