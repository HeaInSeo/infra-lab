#!/usr/bin/env bash
set -euo pipefail

need_cmd() { command -v "$1" >/dev/null 2>&1; }
ok() { echo "OK: $*"; }
die() { echo "ERROR: $*" >&2; exit 1; }

require_linux() {
  [[ -f /etc/os-release ]] || die "Cannot detect OS."
  # shellcheck disable=SC1091
  . /etc/os-release
  ok "Detected OS: ${PRETTY_NAME:-unknown}"
}

require_libvirt() {
  need_cmd virsh || die "virsh not found"
  need_cmd qemu-img || die "qemu-img not found"
  sudo virsh -c "${LIBVIRT_URI:-qemu:///system}" uri >/dev/null
  ok "libvirt reachable: ${LIBVIRT_URI:-qemu:///system}"
}

main() {
  require_linux
  require_libvirt
}

main "$@"
