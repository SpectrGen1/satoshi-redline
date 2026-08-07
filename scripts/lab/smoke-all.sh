#!/usr/bin/env bash
# Run Tier 1 live smoke against all three lab consumers sequentially.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

usage() {
  cat <<'EOF'
usage: smoke-all.sh [--electrum electrs|fulcrum|none] [--vms umbrel,ubuntu,startos]

For each VM: prepare runtime, run livebitcoin + liveelectrum (if enabled),
print a summary. Does not manage lab VM lifecycle — guests must already be up.
EOF
}

ELECTRUM=electrs
VMS=(umbrel ubuntu startos)

while [[ $# -gt 0 ]]; do
  case "$1" in
    --electrum)
      ELECTRUM="${2:-}"
      shift 2
      ;;
    --vms)
      IFS=',' read -r -a VMS <<<"${2:-}"
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

need_cmd go
cd "$REDLINE_ROOT"

declare -a RESULTS=()
fail=0

for vm in "${VMS[@]}"; do
  lab_validate_vm "$vm"
  runtime="$(lab_runtime_for_vm "$vm")"
  note "======== $vm ========"
  if ! "$SCRIPT_DIR/prepare.sh" --vm "$vm" --electrum "$ELECTRUM" --runtime-dir "$runtime"; then
    RESULTS+=("$vm:PREPARE_FAIL")
    fail=1
    continue
  fi
  if "$SCRIPT_DIR/smoke.sh" --runtime-dir "$runtime"; then
    RESULTS+=("$vm:PASS")
    note "$vm smoke PASS"
  else
    RESULTS+=("$vm:SMOKE_FAIL")
    fail=1
    note "$vm smoke FAIL"
  fi
done

note "======== summary ========"
for r in "${RESULTS[@]}"; do
  note "  $r"
done

exit "$fail"
