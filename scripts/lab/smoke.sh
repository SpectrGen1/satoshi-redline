#!/usr/bin/env bash
# Run opt-in live Go smoke tests against a prepared lab runtime (Tier 1).
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

usage() {
  cat <<'EOF'
usage: smoke.sh [options]

Run testlab/livebitcoin and/or testlab/liveelectrum using env from prepare.sh.

options:
  --runtime-dir PATH   default: $REDLINE_LAB_RUNTIME or .lab/runtime
  --bitcoin-only       only ./testlab/livebitcoin
  --electrum-only      only ./testlab/liveelectrum
  --prepare ...        run prepare.sh first with the given args (must be last
                       option group; example: --prepare -- --vm umbrel)
  -h, --help
EOF
}

RUNTIME=""
BITCOIN=1
ELECTRUM=1
PREPARE_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --runtime-dir)
      RUNTIME="${2:-}"
      shift 2
      ;;
    --bitcoin-only)
      BITCOIN=1
      ELECTRUM=0
      shift
      ;;
    --electrum-only)
      BITCOIN=0
      ELECTRUM=1
      shift
      ;;
    --prepare)
      shift
      if [[ "${1:-}" == "--" ]]; then shift; fi
      PREPARE_ARGS=("$@")
      break
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

if [[ ${#PREPARE_ARGS[@]} -gt 0 ]]; then
  "$SCRIPT_DIR/prepare.sh" "${PREPARE_ARGS[@]}"
fi

if [[ -z "$RUNTIME" ]]; then
  RUNTIME="$(lab_default_runtime)"
fi
if [[ "$RUNTIME" != /* ]]; then
  RUNTIME="$REDLINE_ROOT/$RUNTIME"
fi

ENV_FILE="$RUNTIME/env"
[[ -f "$ENV_FILE" ]] || die "missing $ENV_FILE — run ./scripts/lab/prepare.sh first"

need_cmd go
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

cd "$REDLINE_ROOT"

if [[ "$BITCOIN" -eq 1 ]]; then
  [[ "${REDLINE_LIVE_TEST:-}" == "1" ]] || die "REDLINE_LIVE_TEST not set in $ENV_FILE"
  note "running ./testlab/livebitcoin"
  go test -v ./testlab/livebitcoin
fi

if [[ "$ELECTRUM" -eq 1 ]]; then
  if [[ "${REDLINE_LIVE_ELECTRUM_TEST:-}" != "1" ]]; then
    note "skipping electrum smoke (not enabled in runtime env)"
  else
    note "running ./testlab/liveelectrum"
    go test -v ./testlab/liveelectrum
  fi
fi

note "smoke complete"
