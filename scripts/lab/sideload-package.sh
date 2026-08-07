#!/usr/bin/env bash
# Tier 2 package install into a lab guest. Stub until packaging/ formats exist.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

usage() {
  cat <<'EOF'
usage: sideload-package.sh --vm umbrel|ubuntu|startos --artifact PATH

Install an official platform package artifact into a lab guest (Tier 2).

Status: stub. Platform package formats are not in-tree yet (see packaging/).
When packages land, this script will dispatch to platform-native installers
without defining a lab-only package fork (ADR-011).

Until then use:
  ./scripts/lab/prepare.sh --vm umbrel && ./scripts/lab/smoke.sh
  ./scripts/lab/sideload-binary.sh --vm umbrel
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || $# -eq 0 ]]; then
  usage
  exit 0
fi

VM=""
ARTIFACT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --vm)
      VM="${2:-}"
      shift 2
      ;;
    --artifact)
      ARTIFACT="${2:-}"
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
[[ -n "$ARTIFACT" ]] || die "--artifact is required"
lab_validate_vm "$VM"
[[ -f "$ARTIFACT" ]] || die "artifact not found: $ARTIFACT"

cat >&2 <<EOF
error: package sideload is not implemented yet.

  vm:       $VM
  artifact: $ARTIFACT

Official package formats are not present under packaging/. Per ADR-011 the lab
must consume the same packages users install — not a parallel lab package.

Available now:
  ./scripts/lab/prepare.sh --vm $VM
  ./scripts/lab/smoke.sh
  ./scripts/lab/sideload-binary.sh --vm $VM
EOF
exit 2
