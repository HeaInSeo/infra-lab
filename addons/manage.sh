#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ACTION="${1:-}"
SCOPE="${2:-}"
NAME="${3:-}"

usage() {
  cat <<'USAGE'
Usage:
  addons/manage.sh install <base|optional> [name]
  addons/manage.sh uninstall <base|optional> [name]
  addons/manage.sh verify [base|optional] [name]

Examples:
  addons/manage.sh install base
  addons/manage.sh install optional local-path-storage
  addons/manage.sh install optional metallb
  addons/manage.sh install optional cilium
  addons/manage.sh verify
  addons/manage.sh verify optional cilium
USAGE
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing: $1" >&2
    exit 1
  }
}

install_base() {
  bash "${ROOT_DIR}/addons/base/metrics-server/install.sh"
}

uninstall_base() {
  bash "${ROOT_DIR}/addons/base/metrics-server/uninstall.sh"
}

verify_base() {
  bash "${ROOT_DIR}/addons/base/metrics-server/verify.sh"
}

install_optional() {
  case "$1" in
    local-path-storage)
      bash "${ROOT_DIR}/addons/optional/local-path-storage/install.sh"
      ;;
    metallb)
      bash "${ROOT_DIR}/addons/optional/metallb/install.sh"
      ;;
    cilium)
      bash "${ROOT_DIR}/addons/optional/cilium/install.sh"
      ;;
    *)
      echo "unknown optional addon: $1" >&2
      exit 1
      ;;
  esac
}

uninstall_optional() {
  case "$1" in
    local-path-storage)
      bash "${ROOT_DIR}/addons/optional/local-path-storage/uninstall.sh"
      ;;
    metallb)
      bash "${ROOT_DIR}/addons/optional/metallb/uninstall.sh"
      ;;
    cilium)
      bash "${ROOT_DIR}/addons/optional/cilium/uninstall.sh"
      ;;
    *)
      echo "unknown optional addon: $1" >&2
      exit 1
      ;;
  esac
}

verify_optional() {
  case "$1" in
    local-path-storage)
      bash "${ROOT_DIR}/addons/optional/local-path-storage/verify.sh"
      ;;
    metallb)
      bash "${ROOT_DIR}/addons/optional/metallb/verify.sh"
      ;;
    cilium)
      bash "${ROOT_DIR}/addons/optional/cilium/verify.sh"
      ;;
    *)
      echo "unknown optional addon: $1" >&2
      exit 1
      ;;
  esac
}

is_installed() {
  case "$1" in
    local-path-storage)
      kubectl -n local-path-storage get deployment local-path-provisioner >/dev/null 2>&1
      ;;
    metallb)
      kubectl -n metallb-system get deployment controller >/dev/null 2>&1
      ;;
    cilium)
      kubectl -n kube-system get ds cilium >/dev/null 2>&1
      ;;
    *)
      return 1
      ;;
  esac
}

verify_installed() {
  verify_base

  for addon in local-path-storage metallb cilium; do
    if is_installed "$addon"; then
      verify_optional "$addon"
    fi
  done
}

case "$ACTION" in
  install)
    need_cmd kubectl
    if [[ "$SCOPE" == "base" ]]; then
      install_base
    elif [[ "$SCOPE" == "optional" ]]; then
      [[ -n "$NAME" ]] || { echo "optional addon name required" >&2; exit 1; }
      install_optional "$NAME"
    else
      usage
      exit 1
    fi
    ;;
  uninstall)
    need_cmd kubectl
    if [[ "$SCOPE" == "base" ]]; then
      uninstall_base
    elif [[ "$SCOPE" == "optional" ]]; then
      [[ -n "$NAME" ]] || { echo "optional addon name required" >&2; exit 1; }
      uninstall_optional "$NAME"
    else
      usage
      exit 1
    fi
    ;;
  verify)
    need_cmd kubectl
    if [[ -z "$SCOPE" ]]; then
      verify_installed
    elif [[ "$SCOPE" == "base" ]]; then
      verify_base
    elif [[ "$SCOPE" == "optional" ]]; then
      [[ -n "$NAME" ]] || { echo "optional addon name required" >&2; exit 1; }
      verify_optional "$NAME"
    else
      usage
      exit 1
    fi
    ;;
  *)
    usage
    exit 1
    ;;
esac
