#!/usr/bin/env bash
# serve.sh — build and start dep-health from a clean checkout.
#
# Usage:
#   ./serve.sh              # starts on default port 8080
#   PORT=9000 ./serve.sh    # starts on a custom port
#   DB=/tmp/my.db ./serve.sh

set -euo pipefail

PORT="${PORT:-8080}"
DB="${DB:-dep-health.db}"

# ── helpers ───────────────────────────────────────────────────────────────────

log() { printf '\033[1;36m==> %s\033[0m\n' "$*"; }
die() { printf '\033[1;31merror: %s\033[0m\n' "$*" >&2; exit 1; }

# ── prerequisite checks ───────────────────────────────────────────────────────

command -v go  >/dev/null 2>&1 || die "Go is not installed (https://go.dev/dl)"
command -v node >/dev/null 2>&1 || die "Node.js is not installed (https://nodejs.org)"
command -v npm  >/dev/null 2>&1 || die "npm is not installed (comes with Node.js)"

# ── locate the script's directory so it works from any CWD ───────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# ── 1. Go dependencies ────────────────────────────────────────────────────────

log "Tidying Go modules …"
go mod tidy

# ── 2. Frontend ───────────────────────────────────────────────────────────────

log "Installing frontend dependencies …"
npm --prefix frontend install --silent

log "Building frontend …"
npm --prefix frontend run build --silent

# ── 3. Binary ─────────────────────────────────────────────────────────────────

log "Compiling binary …"
go build -o dep-health .

# ── 4. Launch ─────────────────────────────────────────────────────────────────

log "Starting dep-health serve on http://localhost:${PORT}"
exec ./dep-health serve --port "$PORT" --db "$DB"
