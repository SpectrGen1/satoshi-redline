#!/usr/bin/env bash
# SSH local forwards for lab guests whose services bind loopback, LXC, or restrict rpcallowip.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

usage() {
  cat <<'EOF'
usage: tunnel.sh --vm umbrel|ubuntu|startos start|stop|status

Forward host ports into the guest so Tier 1 host-side scans can reach RPC
(and Electrum when not published on the guest LAN).

umbrel:
  9332 → guest 127.0.0.1:9332     Bitcoin RPC (LAN :9332 is 403: rpcallowip)
  Electrs/Fulcrum stay on guest :50001/:50002

ubuntu:
  38332 → guest 127.0.0.1:38332   Bitcoin RPC
  50001 → guest 127.0.0.1:50001   Electrs
  50002 → guest 127.0.0.1:50002   Fulcrum

startos:
  Discovers package LXC IPv4s, then:
  18332 → <bitcoind-lxc>:8332     Bitcoin RPC
  25001 → <electrs-lxc>:50001     Electrs
  25002 → <fulcrum-lxc>:50001     Fulcrum (container port; host :50002 is not used)

Does not start or stop the lab VM. Uses SSH ControlMaster under the runtime ssh/ dir.
EOF
}

VM=""
CMD=""
RUNTIME=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --vm)
      VM="${2:-}"
      shift 2
      ;;
    --runtime-dir)
      RUNTIME="${2:-}"
      shift 2
      ;;
    start|stop|status)
      CMD="$1"
      shift
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
[[ -n "$CMD" ]] || die "expected start, stop, or status"
lab_validate_vm "$VM"

need_cmd ssh
if [[ -z "$RUNTIME" ]]; then
  RUNTIME="$(lab_runtime_for_vm "$VM")"
fi
if [[ "$RUNTIME" != /* ]]; then
  RUNTIME="$REDLINE_ROOT/$RUNTIME"
fi
CONTROL_DIR="$RUNTIME/ssh"
CONTROL_PATH="$CONTROL_DIR/${VM}-tunnel.sock"
install -d -m 0700 "$RUNTIME" "$CONTROL_DIR"

ip="$(lab_guest_ip "$VM")"
user="$(lab_ssh_user "$VM")"
key="$(lab_ssh_key "$VM")"

ssh_base=(
  ssh -i "$key"
  -o BatchMode=yes
  -o IdentitiesOnly=yes
  -o StrictHostKeyChecking=accept-new
  -o ConnectTimeout=10
  -o ExitOnForwardFailure=yes
  -o ControlMaster=auto
  -o "ControlPath=$CONTROL_PATH"
  -o ControlPersist=yes
)

build_forward_args() {
  forward_args=()
  case "$VM" in
    umbrel)
      # Umbrel Knots rpcallowip is docker+loopback only; host→guest-IP:9332 gets HTTP 403.
      forward_args=(-L "9332:127.0.0.1:9332")
      ;;
    ubuntu)
      forward_args=(
        -L "38332:127.0.0.1:38332"
        -L "50001:127.0.0.1:50001"
        -L "50002:127.0.0.1:50002"
      )
      ;;
    startos)
      local disc bitcoind_ip electrs_ip fulcrum_ip
      note "discovering StartOS package LXC addresses"
      disc="$(lab_startos_lxc_ips)" || die "StartOS LXC discovery failed (is bitcoind running?)"
      bitcoind_ip="$(printf '%s\n' "$disc" | awk -F= '/^bitcoind_ip=/{print $2}')"
      electrs_ip="$(printf '%s\n' "$disc" | awk -F= '/^electrs_ip=/{print $2}')"
      fulcrum_ip="$(printf '%s\n' "$disc" | awk -F= '/^fulcrum_ip=/{print $2}')"
      [[ -n "$bitcoind_ip" ]] || die "StartOS bitcoind LXC IP empty"
      printf '%s\n' "$disc" >"$RUNTIME/startos-lxc.env"
      chmod 600 "$RUNTIME/startos-lxc.env"
      note "startos LXC bitcoind=$bitcoind_ip electrs=${electrs_ip:-none} fulcrum=${fulcrum_ip:-none}"
      forward_args=(-L "18332:${bitcoind_ip}:8332")
      if [[ -n "$electrs_ip" ]]; then
        forward_args+=(-L "25001:${electrs_ip}:50001")
      fi
      if [[ -n "$fulcrum_ip" ]]; then
        # Fulcrum package listens on 50001 inside its LXC.
        forward_args+=(-L "25002:${fulcrum_ip}:50001")
      fi
      ;;
  esac
}

case "$CMD" in
  start)
    if [[ -S "$CONTROL_PATH" ]] && "${ssh_base[@]}" -O check "${user}@${ip}" >/dev/null 2>&1; then
      note "$VM tunnel already up ($CONTROL_PATH)"
      exit 0
    fi
    # Drop a stale control socket from a dead session.
    rm -f "$CONTROL_PATH"
    build_forward_args
    note "starting $VM tunnel via $user@$ip (${forward_args[*]})"
    "${ssh_base[@]}" -fN "${forward_args[@]}" "${user}@${ip}"
    note "$VM tunnel started"
    ;;
  stop)
    if [[ -S "$CONTROL_PATH" ]]; then
      "${ssh_base[@]}" -O exit "${user}@${ip}" 2>/dev/null || true
      rm -f "$CONTROL_PATH"
      note "$VM tunnel stopped"
    else
      note "no active $VM tunnel control socket"
    fi
    ;;
  status)
    if [[ -S "$CONTROL_PATH" ]] && "${ssh_base[@]}" -O check "${user}@${ip}" >/dev/null 2>&1; then
      note "$VM tunnel active → $ip"
      exit 0
    fi
    note "$VM tunnel inactive"
    exit 1
    ;;
esac
