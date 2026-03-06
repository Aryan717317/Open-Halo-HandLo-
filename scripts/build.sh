#!/bin/bash
set -e

echo "==> Building GestureShare"

# Build Go sidecar for all platforms
echo "--> Compiling Go sidecar..."
cd sidecar

echo "  macOS (arm64)..."
GOOS=darwin  GOARCH=arm64  go build -o ../apps/desktop/src-tauri/binaries/sidecar-aarch64-apple-darwin .

echo "  macOS (x64)..."
GOOS=darwin  GOARCH=amd64  go build -o ../apps/desktop/src-tauri/binaries/sidecar-x86_64-apple-darwin .

echo "  Windows (x64)..."
GOOS=windows GOARCH=amd64  go build -o ../apps/desktop/src-tauri/binaries/sidecar-x86_64-pc-windows-msvc.exe .

echo "  Linux (x64)..."
GOOS=linux   GOARCH=amd64  go build -o ../apps/desktop/src-tauri/binaries/sidecar-x86_64-unknown-linux-gnu .

cd ../apps/desktop
echo "--> Building Tauri app..."
cargo tauri build

echo ""
echo "==> Build complete! Installers in apps/desktop/src-tauri/target/release/bundle/"
