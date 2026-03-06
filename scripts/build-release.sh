#!/bin/bash
# scripts/build-release.sh — Cross-compile Go sidecar + build Tauri for all platforms
set -e

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SIDECAR="$ROOT/sidecar"
BINS="$ROOT/apps/desktop/src-tauri/binaries"

mkdir -p "$BINS"

echo "🔨 Building Go sidecar for all platforms..."

# macOS ARM (Apple Silicon)
GOOS=darwin  GOARCH=arm64  go build -o "$BINS/gestureshare-sidecar-aarch64-apple-darwin"   "$SIDECAR"
echo "  ✅ macOS arm64"

# macOS Intel
GOOS=darwin  GOARCH=amd64  go build -o "$BINS/gestureshare-sidecar-x86_64-apple-darwin"     "$SIDECAR"
echo "  ✅ macOS amd64"

# Windows
GOOS=windows GOARCH=amd64  go build -o "$BINS/gestureshare-sidecar-x86_64-pc-windows-msvc.exe" "$SIDECAR"
echo "  ✅ Windows amd64"

# Linux
GOOS=linux   GOARCH=amd64  go build -o "$BINS/gestureshare-sidecar-x86_64-unknown-linux-gnu"    "$SIDECAR"
echo "  ✅ Linux amd64"

echo ""
echo "📦 Building Tauri app..."
cd "$ROOT/apps/desktop"
cargo tauri build

echo ""
echo "🎉 Build complete! Output in apps/desktop/src-tauri/target/release/bundle"
