#!/usr/bin/env bash
# Builds the backend as a single static Linux binary.
#   scripts/build-backend.sh arm64   # Oracle Ampere A1 (ARM)
#   scripts/build-backend.sh amd64   # Oracle E2 Micro (AMD)
# The binary embeds the SQL migrations; nothing else needs to be copied.
set -euo pipefail

ARCH="${1:-amd64}"
case "$ARCH" in amd64|arm64) ;; *) echo "usage: $0 amd64|arm64" >&2; exit 1 ;; esac

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/dist/critter-bid-linux-$ARCH"
mkdir -p "$ROOT/dist"

cd "$ROOT/backend"

# go1.27.0 - go1.27.2 have known net/http and crypto vulnerabilities
# (govulncheck). Refuse to build a production binary with them.
MIN_PATCH=3
VER="$(go env GOVERSION)"            # e.g. go1.27.3
if [[ "$VER" =~ ^go1.27(.([0-9]+))?$ ]]; then
  PATCH="${BASH_REMATCH[2]:-0}"
  if (( PATCH < MIN_PATCH )); then
    echo "error: $VER is vulnerable; install go1.27.$MIN_PATCH+ from https://go.dev/dl/" >&2
    exit 1
  fi
fi
CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" \
  go build -trimpath -ldflags="-s -w" -o "$OUT" ./cmd/server
echo "built $OUT ($(du -h "$OUT" | cut -f1))"
