#!/usr/bin/env bash
# scripts/build.sh — build bismuth and the web PWA.
# Idempotent. Safe to run from anywhere.

set -euo pipefail

# Use home for Go temp (tmpfs is often 100% full on this machine).
export GOTMPDIR="${GOTMPDIR:-/home/lisergico25/.tmp}"
mkdir -p "$GOTMPDIR"

# Go server
echo "==> building bismuth (Go)"
cd "$(dirname "$0")/.."
go build -o bin/bismuth ./cmd/bismuth
echo "    -> $(pwd)/bin/bismuth"

# Web PWA (optional, if node_modules present)
if [ -d web ] && [ -f web/package.json ]; then
  if [ -d web/node_modules ]; then
    echo "==> building web PWA"
    (cd web && npm run build)
  else
    echo "==> skipping web PWA (run 'cd web && npm install' first)"
  fi
fi

echo "==> done"
