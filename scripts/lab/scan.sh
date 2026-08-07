#!/usr/bin/env bash
# One-shot redline scan against prepared lab runtime (Tier 1).
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

usage() {
  cat <<'EOF'
usage: scan.sh [--runtime-dir PATH] [-- extra redline args]

Build and run: go run ./cmd/redline --config <runtime>/redline.json scan --json
EOF
}

RUNTIME=""
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

if [[ -z "$RUNTIME" ]]; then
  RUNTIME="$(lab_default_runtime)"
fi
if [[ "$RUNTIME" != /* ]]; then
  RUNTIME="$REDLINE_ROOT/$RUNTIME"
fi

CONFIG="$RUNTIME/redline.json"
[[ -f "$CONFIG" ]] || die "missing $CONFIG — run ./scripts/lab/prepare.sh first"
need_cmd go
cd "$REDLINE_ROOT"
note "scan using $CONFIG"
go run ./cmd/redline --config "$CONFIG" scan --json "${EXTRA[@]}"
