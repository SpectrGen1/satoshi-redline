#!/usr/bin/env bash
# Compatibility wrapper — prefer: ./scripts/lab/tunnel.sh --vm ubuntu …
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$SCRIPT_DIR/tunnel.sh" --vm ubuntu "$@"
