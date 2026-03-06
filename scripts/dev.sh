#!/bin/bash
set -e

echo "==> Starting GestureShare in development mode"
echo ""
echo "  Go sidecar: will be spawned by Tauri"
echo "  SvelteKit:  http://localhost:5173"
echo "  Tauri app:  native window"
echo ""

cd apps/desktop
cargo tauri dev
