#!/usr/bin/env bash
# Dev shortcut: build linux/amd64 redline, copy into guest, run scan (Tier 2-style).
# Not package-store proof. Does not manage VM lifecycle.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

usage() {
  cat <<'EOF'
usage: sideload-binary.sh --vm umbrel|ubuntu|startos [options]

Build GOOS=linux GOARCH=amd64 redline, install under /tmp/redline-lab on the
guest with a guest-local config and cookie path, and run a one-shot scan.

This is a development shortcut (ADR-011). It is not a substitute for installing
an official platform package once packaging exists.

options:
  --vm NAME
  --electrum KIND     both (default), electrs, fulcrum, none
  --skip-build        reuse existing .lab/runtime/dist/redline-linux-amd64
  --print-only        build/config only; print remote run command
  -h, --help
EOF
}

VM=""
ELECTRUM=both
SKIP_BUILD=0
PRINT_ONLY=0
RUNTIME="$(lab_default_runtime)"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --vm)
      VM="${2:-}"
      shift 2
      ;;
    --electrum)
      ELECTRUM="${2:-}"
      shift 2
      ;;
    --skip-build)
      SKIP_BUILD=1
      shift
      ;;
    --print-only)
      PRINT_ONLY=1
      shift
      ;;
    --runtime-dir)
      RUNTIME="${2:-}"
      shift 2
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

[[ -n "$VM" ]] || die "--vm is required"
lab_validate_vm "$VM"
case "$ELECTRUM" in
  both|electrs|fulcrum|none) ;;
  *) die "--electrum must be both, electrs, fulcrum, or none" ;;
esac

need_cmd go
need_cmd ssh
need_cmd scp
need_cmd python3

if [[ "$RUNTIME" != /* ]]; then
  RUNTIME="$REDLINE_ROOT/$RUNTIME"
fi
mkdir -p "$RUNTIME"
RUNTIME="$(cd "$RUNTIME" && pwd)"

install -d -m 0700 "$RUNTIME/dist"
BINARY="$RUNTIME/dist/redline-linux-amd64"
REMOTE_DIR="/tmp/redline-lab"
REMOTE_BIN="$REMOTE_DIR/redline"
REMOTE_CONFIG="$REMOTE_DIR/redline.json"
REMOTE_DB="$REMOTE_DIR/redline.db"
REMOTE_FS="$REMOTE_DIR/fs-probe"

if [[ "$SKIP_BUILD" -eq 1 ]]; then
  [[ -f "$BINARY" ]] || die "missing $BINARY"
else
  note "building linux/amd64 redline → $BINARY"
  (
    cd "$REDLINE_ROOT"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$BINARY" ./cmd/redline
  )
fi

guest_cookie="$(lab_guest_cookie_path "$VM")"
guest_rpc="$(lab_guest_rpc_endpoint "$VM")"
guest_electrs=""
guest_fulcrum=""
case "$ELECTRUM" in
  both)
    guest_electrs="$(lab_guest_electrum_endpoint electrs)"
    guest_fulcrum="$(lab_guest_electrum_endpoint fulcrum)"
    ;;
  electrs) guest_electrs="$(lab_guest_electrum_endpoint electrs)" ;;
  fulcrum) guest_fulcrum="$(lab_guest_electrum_endpoint fulcrum)" ;;
esac

local_guest_config="$RUNTIME/guest-redline.json"
lab_write_config "$local_guest_config" "$guest_rpc" "$guest_cookie" "$REMOTE_FS" "$REMOTE_DB" "$guest_electrs" "$guest_fulcrum"
chmod 600 "$local_guest_config"

note "installing binary and config on $VM under $REMOTE_DIR"
lab_ssh "$VM" "mkdir -p '$REMOTE_DIR' '$REMOTE_FS' && chmod 700 '$REMOTE_DIR' '$REMOTE_FS'"
lab_scp_to "$VM" "$BINARY" "$REMOTE_BIN"
lab_scp_to "$VM" "$local_guest_config" "$REMOTE_CONFIG"
lab_ssh "$VM" "chmod 700 '$REMOTE_BIN' && chmod 600 '$REMOTE_CONFIG'"

run_cmd="$REMOTE_BIN --config $REMOTE_CONFIG scan --json"
if [[ "$PRINT_ONLY" -eq 1 ]]; then
  note "remote command: $run_cmd"
  printf '%s\n' "$run_cmd"
  exit 0
fi

note "running in-guest scan (cookie contents not logged)"
# shellcheck disable=SC2029
lab_ssh "$VM" "$run_cmd"
