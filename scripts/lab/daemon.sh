#!/usr/bin/env bash
# Run redlined against prepared lab runtime (Tier 1).
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

usage() {
  cat <<'EOF'
usage: daemon.sh [--runtime-dir PATH] [-- extra redlined args]

Build and run: go run ./cmd/redlined --config <runtime>/redline.json
EOF
}

RUNTIME="$(lab_default_runtime)"
EXTRA=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --runtime-dir)
      RUNTIME="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      EXTRA=("$@")
      break
      ;;
    *)
      EXTRA+=("$1")
      shift
      ;;
  esac
done

CONFIG="$RUNTIME/redline.json"
[[ -f "$CONFIG" ]] || die "missing $CONFIG — run ./scripts/lab/prepare.sh first"
need_cmd go
cd "$REDLINE_ROOT"
note "daemon using $CONFIG (Ctrl-C to stop)"
go run ./cmd/redlined --config "$CONFIG" "${EXTRA[@]}"
