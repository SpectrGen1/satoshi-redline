# Shared helpers for opt-in lab operator scripts.
# These scripts never start/stop/promote/discard lab VMs or Bitcoin packages.
# shellcheck shell=bash

set -Eeuo pipefail

lab_repo_root() {
  local here
  here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  cd "$here/../.." && pwd
}

REDLINE_ROOT="${REDLINE_ROOT:-$(lab_repo_root)}"

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

note() {
  printf 'lab: %s\n' "$*" >&2
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

lab_default_runtime() {
  # Prefer per-VM runtime when REDLINE_LAB_VM is set; else shared .lab/runtime.
  if [[ -n "${REDLINE_LAB_RUNTIME:-}" ]]; then
    printf '%s\n' "$REDLINE_LAB_RUNTIME"
  elif [[ -n "${REDLINE_LAB_VM:-}" ]]; then
    printf '%s\n' "$REDLINE_ROOT/.lab/runtime/${REDLINE_LAB_VM}"
  else
    printf '%s\n' "$REDLINE_ROOT/.lab/runtime"
  fi
}

lab_runtime_for_vm() {
  local vm="$1"
  if [[ -n "${REDLINE_LAB_RUNTIME:-}" ]]; then
    printf '%s\n' "$REDLINE_LAB_RUNTIME"
  else
    printf '%s\n' "$REDLINE_ROOT/.lab/runtime/${vm}"
  fi
}

lab_path() {
  local p="${BITCOIN_VM_LAB:-$HOME/projects/bitcoin-vm-lab}"
  [[ -d "$p" ]] || die "bitcoin-vm-lab not found at $p (set BITCOIN_VM_LAB)"
  printf '%s\n' "$p"
}

bvml_bin() {
  local lab root
  lab="$(lab_path)"
  root="$lab/bin/bvml"
  [[ -x "$root" ]] || die "bvml not executable at $root"
  printf '%s\n' "$root"
}

# Resolve guest IPv4 from bvml info, or REDLINE_LAB_IP override.
lab_guest_ip() {
  local vm="$1" ip info
  if [[ -n "${REDLINE_LAB_IP:-}" ]]; then
    printf '%s\n' "$REDLINE_LAB_IP"
    return 0
  fi
  need_cmd grep
  info="$("$(bvml_bin)" info "$vm" 2>/dev/null)" || die "bvml info $vm failed (is the lab installed?)"
  ip="$(printf '%s\n' "$info" | grep -E '^\s+IPv4\s+' | head -1 | awk '{print $2}')"
  [[ -n "$ip" && "$ip" != "unavailable"* ]] || die "could not resolve IPv4 for $vm; start/resume the VM or set REDLINE_LAB_IP"
  [[ "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "invalid IPv4 from bvml info: $ip"
  printf '%s\n' "$ip"
}

lab_ssh_user() {
  case "$1" in
    umbrel) printf '%s\n' "${UMBREL_SSH_USER:-umbrel}" ;;
    ubuntu) printf '%s\n' "${UBUNTU_CLOUD_USER:-ubuntu}" ;;
    startos) printf '%s\n' "${STARTOS_SSH_USER:-start9}" ;;
    *) die "unknown vm: $1" ;;
  esac
}

# Resolve a usable private SSH key for the guest.
# Note: bitcoin-vm-lab's UBUNTU_CLOUD_SSH_KEY is often the *public* key path used
# for cloud-init inject; we derive the private key by stripping .pub when needed.
lab_ssh_key() {
  local vm="$1" candidate=""
  case "$vm" in
    umbrel)
      candidate="${UMBREL_SSH_PRIVATE_KEY:-$HOME/.config/bitcoin-vm-lab/umbrel_ed25519}"
      [[ -f "$candidate" ]] || die "Umbrel SSH private key not found at $candidate"
      printf '%s\n' "$candidate"
      return 0
      ;;
    startos)
      candidate="${STARTOS_SSH_PRIVATE_KEY:-$HOME/.config/bitcoin-vm-lab/startos_ed25519}"
      [[ -f "$candidate" ]] || die "StartOS SSH private key not found at $candidate"
      printf '%s\n' "$candidate"
      return 0
      ;;
    ubuntu)
      local try
      for try in \
        "${UBUNTU_SSH_PRIVATE_KEY:-}" \
        "${UBUNTU_CLOUD_SSH_KEY:-}" \
        "$HOME/.ssh/id_ed25519_spectrgen" \
        "$HOME/.ssh/id_ed25519" \
        "$HOME/.ssh/id_rsa"
      do
        [[ -n "$try" ]] || continue
        # If a public key path was given, try the matching private key.
        if [[ "$try" == *.pub && -f "${try%.pub}" ]]; then
          try="${try%.pub}"
        fi
        [[ -f "$try" ]] || continue
        # Reject obvious public-key-only files used as -i.
        if head -1 "$try" 2>/dev/null | grep -q 'BEGIN OPENSSH PRIVATE KEY\|BEGIN .*PRIVATE KEY'; then
          printf '%s\n' "$try"
          return 0
        fi
        # Accept non-PEM OpenSSH private keys without scanning (mode-600 files
        # that are not .pub); ssh will fail closed if unusable.
        if [[ "$try" != *.pub ]]; then
          printf '%s\n' "$try"
          return 0
        fi
      done
      die "Ubuntu SSH private key not found (set UBUNTU_SSH_PRIVATE_KEY or install the key used for cloud-init)"
      ;;
    *) die "unknown vm: $vm" ;;
  esac
}

lab_ssh() {
  local vm="$1"
  shift
  local ip user key
  ip="$(lab_guest_ip "$vm")"
  user="$(lab_ssh_user "$vm")"
  key="$(lab_ssh_key "$vm")"
  ssh -i "$key" \
    -o BatchMode=yes \
    -o IdentitiesOnly=yes \
    -o StrictHostKeyChecking=accept-new \
    -o ConnectTimeout=10 \
    "${user}@${ip}" "$@"
}

lab_scp_from() {
  local vm="$1" remote="$2" local_path="$3"
  local ip user key
  ip="$(lab_guest_ip "$vm")"
  user="$(lab_ssh_user "$vm")"
  key="$(lab_ssh_key "$vm")"
  scp -i "$key" \
    -o BatchMode=yes \
    -o IdentitiesOnly=yes \
    -o StrictHostKeyChecking=accept-new \
    -o ConnectTimeout=10 \
    "${user}@${ip}:${remote}" "$local_path"
}

lab_scp_to() {
  local vm="$1" local_path="$2" remote="$3"
  local ip user key
  ip="$(lab_guest_ip "$vm")"
  user="$(lab_ssh_user "$vm")"
  key="$(lab_ssh_key "$vm")"
  scp -i "$key" \
    -o BatchMode=yes \
    -o IdentitiesOnly=yes \
    -o StrictHostKeyChecking=accept-new \
    -o ConnectTimeout=10 \
    "$local_path" "${user}@${ip}:${remote}"
}

# Guest cookie path for signet lab profiles.
lab_guest_cookie_path() {
  local vm="$1"
  case "$vm" in
    umbrel)
      printf '%s\n' "${REDLINE_LAB_COOKIE_REMOTE:-/home/umbrel/umbrel/app-data/bitcoin-knots/data/bitcoin/signet/.cookie}"
      ;;
    ubuntu)
      printf '%s\n' "${REDLINE_LAB_COOKIE_REMOTE:-/srv/bitcoin/signet/.cookie}"
      ;;
    startos)
      printf '%s\n' "${REDLINE_LAB_COOKIE_REMOTE:-/media/startos/data/package-data/volumes/bitcoind/data/main/signet/.cookie}"
      ;;
    *) die "unknown vm: $vm" ;;
  esac
}

# Fetch cookie to a local mode-600 file. Never prints cookie contents.
lab_fetch_cookie() {
  local vm="$1" dest="$2"
  local remote tmp
  remote="$(lab_guest_cookie_path "$vm")"
  tmp="$(mktemp "${dest}.XXXXXX")"
  chmod 600 "$tmp"
  case "$vm" in
    startos)
      # Package volume is root-only; start9 has passwordless sudo in the lab.
      if ! lab_ssh "$vm" "sudo cat $(printf '%q' "$remote")" >"$tmp"; then
        rm -f "$tmp"
        die "startos cookie fetch failed via sudo cat $remote"
      fi
      ;;
    *)
      if ! lab_scp_from "$vm" "$remote" "$tmp"; then
        rm -f "$tmp"
        die "cookie fetch failed from $vm:$remote"
      fi
      ;;
  esac
  [[ -s "$tmp" ]] || { rm -f "$tmp"; die "cookie file empty after fetch from $vm"; }
  mv -f "$tmp" "$dest"
  chmod 600 "$dest"
}

# Host-local tunnel listen ports (chosen to allow concurrent multi-VM tunnels).
lab_host_rpc_port() {
  case "$1" in
    umbrel) printf '9332\n' ;;
    ubuntu) printf '38332\n' ;;
    startos) printf '18332\n' ;;
    *) die "unknown vm: $1" ;;
  esac
}

lab_host_electrum_port() {
  local vm="$1" which="${2:-electrs}"
  case "$vm" in
    umbrel)
      # Direct to guest LAN (no tunnel).
      case "$which" in
        electrs) printf '50001\n' ;;
        fulcrum) printf '50002\n' ;;
        *) die "unknown electrum: $which" ;;
      esac
      ;;
    ubuntu)
      case "$which" in
        electrs) printf '50001\n' ;;
        fulcrum) printf '50002\n' ;;
        *) die "unknown electrum: $which" ;;
      esac
      ;;
    startos)
      # Tunneled to LXC private IPs; host ports avoid clashing with ubuntu tunnels.
      case "$which" in
        electrs) printf '25001\n' ;;
        fulcrum) printf '25002\n' ;;
        *) die "unknown electrum: $which" ;;
      esac
      ;;
    *) die "unknown vm: $vm" ;;
  esac
}

# Discover StartOS package LXC IPv4s (bitcoind / electrs / fulcrum). Cached per call.
lab_startos_lxc_ips() {
  # Prints: bitcoind_ip=... electrs_ip=... fulcrum_ip=...
  lab_ssh startos 'sudo bash -s' <<'REMOTE'
set -euo pipefail
bitcoind_ip=""; electrs_ip=""; fulcrum_ip=""
for c in $(lxc-ls 2>/dev/null); do
  ip="$(lxc-info -n "$c" -iH 2>/dev/null | head -1 || true)"
  [[ -n "$ip" ]] || continue
  if lxc-attach -n "$c" -- sh -c 'ss -lntp 2>/dev/null | grep -qE ":8332[[:space:]]"' 2>/dev/null; then
    bitcoind_ip="$ip"
  fi
  if lxc-attach -n "$c" -- sh -c 'ss -lntp 2>/dev/null | grep -qi electrs' 2>/dev/null; then
    electrs_ip="$ip"
  fi
  if lxc-attach -n "$c" -- sh -c 'ss -lntp 2>/dev/null | grep -qi Fulcrum' 2>/dev/null; then
    fulcrum_ip="$ip"
  fi
done
[[ -n "$bitcoind_ip" ]] || { echo "bitcoind LXC with :8332 not found" >&2; exit 1; }
printf 'bitcoind_ip=%s\n' "$bitcoind_ip"
printf 'electrs_ip=%s\n' "${electrs_ip:-}"
printf 'fulcrum_ip=%s\n' "${fulcrum_ip:-}"
REMOTE
}

# Host-facing RPC URL for Tier 1 (via SSH tunnel where required).
lab_rpc_endpoint() {
  local vm="$1" port
  if [[ -n "${REDLINE_LAB_RPC_ENDPOINT:-}" ]]; then
    printf '%s\n' "$REDLINE_LAB_RPC_ENDPOINT"
    return 0
  fi
  port="$(lab_host_rpc_port "$vm")"
  printf 'http://127.0.0.1:%s\n' "$port"
}

# Whether prepare should ensure an SSH tunnel before host-side Tier 1 work.
lab_needs_rpc_tunnel() {
  case "$1" in
    umbrel|ubuntu|startos) return 0 ;;
    *) return 1 ;;
  esac
}

lab_electrum_endpoint() {
  local vm="$1" which="${2:-electrs}"
  local ip port
  case "$which" in
    electrs|fulcrum) ;;
    none) printf '\n'; return 0 ;;
    *) die "unknown electrum backend: $which (electrs|fulcrum|none)" ;;
  esac
  if [[ -n "${REDLINE_LAB_ELECTRUM_ENDPOINT:-}" ]]; then
    printf '%s\n' "$REDLINE_LAB_ELECTRUM_ENDPOINT"
    return 0
  fi
  case "$vm" in
    umbrel)
      ip="$(lab_guest_ip umbrel)"
      port="$(lab_host_electrum_port umbrel "$which")"
      printf 'tcp://%s:%s\n' "$ip" "$port"
      ;;
    ubuntu|startos)
      port="$(lab_host_electrum_port "$vm" "$which")"
      printf 'tcp://127.0.0.1:%s\n' "$port"
      ;;
    *) die "unknown vm: $vm" ;;
  esac
}

# Guest-local endpoints when running redline inside the guest.
lab_guest_rpc_endpoint() {
  local vm="$1"
  case "$vm" in
    umbrel) printf 'http://127.0.0.1:9332\n' ;;
    ubuntu) printf 'http://127.0.0.1:38332\n' ;;
    startos)
      # StartOS bitcoind listens in an LXC, not on the StartOS host loopback.
      printf '%s\n' "${REDLINE_LAB_GUEST_RPC_ENDPOINT:-}"
      [[ -n "${REDLINE_LAB_GUEST_RPC_ENDPOINT:-}" ]] || die "startos in-guest RPC requires REDLINE_LAB_GUEST_RPC_ENDPOINT (bitcoind LXC IP:8332)"
      ;;
    *) die "unknown vm: $vm" ;;
  esac
}

lab_guest_electrum_endpoint() {
  local which="${1:-electrs}"
  case "$which" in
    electrs) printf 'tcp://127.0.0.1:50001\n' ;;
    fulcrum) printf 'tcp://127.0.0.1:50002\n' ;;
    none) printf '\n' ;;
    *) die "unknown electrum backend: $which" ;;
  esac
}

lab_validate_vm() {
  case "$1" in
    umbrel|ubuntu|startos) ;;
    *) die "unknown --vm $1 (umbrel|ubuntu|startos)" ;;
  esac
}

lab_write_config() {
  # Args: OUT RPC COOKIE DATA_DIR DB_PATH [ELECTRS_EP] [FULCRUM_EP]
  local out="$1" rpc="$2" cookie="$3" data_dir="$4" db_path="$5"
  local electrs_ep="${6:-}" fulcrum_ep="${7:-}"
  python3 - "$out" "$rpc" "$cookie" "$data_dir" "$db_path" "$electrs_ep" "$fulcrum_ep" <<'INNER'
import json, sys
out, rpc, cookie, data_dir, db_path, electrs_ep, fulcrum_ep = sys.argv[1:]
backends = []
if electrs_ep:
    backends.append({
        "id": "electrum.electrs",
        "enabled": True,
        "display_alias": "Electrs",
        "endpoint": electrs_ep,
        "timeout_ms": 5000,
        "bitcoin_component_id": "bitcoin.knots",
    })
if fulcrum_ep:
    backends.append({
        "id": "electrum.fulcrum",
        "enabled": True,
        "display_alias": "Fulcrum",
        "endpoint": fulcrum_ep,
        "timeout_ms": 5000,
        "bitcoin_component_id": "bitcoin.knots",
    })
cfg = {
    "bitcoin_nodes": [{
        "id": "bitcoin.knots",
        "enabled": True,
        "display_alias": "Bitcoin Knots",
        "rpc_endpoint": rpc,
        "cookie_file": cookie,
        "cookie_env": "",
        "data_directory": data_dir,
    }],
    "electrum_backends": backends,
    "electrum_thresholds": {
        "tip_warning_age_seconds": 3600,
        "tip_failure_age_seconds": 7200,
        "lag_warning_blocks": 2,
        "lag_failure_blocks": 10,
        "max_observation_skew_seconds": 30,
        "max_observation_age_seconds": 60,
        "max_future_skew_seconds": 300,
    },
    "api": {"listen_address": "127.0.0.1:8383"},
    "database_path": db_path,
    "rpc_timeout_ms": 10000,
    "synchronization": {
        "max_tip_age_seconds": 7200,
        "min_verification_progress": 0.999,
        "max_header_lag": 2,
    },
    "filesystem": {
        "warning_available_percent": 15,
        "failure_available_percent": 5,
    },
}
with open(out, "w", encoding="utf-8") as f:
    json.dump(cfg, f, indent=2)
    f.write("\n")
INNER
}
