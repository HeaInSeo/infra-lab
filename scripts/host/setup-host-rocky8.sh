#!/usr/bin/env bash
set -euo pipefail

need_cmd() { command -v "$1" >/dev/null 2>&1; }
say() { echo "$*"; }
ok() { echo "OK: $*"; }
warn() { echo "WARN: $*"; }
die() { echo "ERROR: $*" >&2; exit 1; }

require_linux() {
  [[ -f /etc/os-release ]] || die "Cannot detect OS."
  # shellcheck disable=SC1091
  . /etc/os-release
  ok "Detected OS: ${PRETTY_NAME:-unknown}"

  local family="${ID_LIKE:-} ${ID:-}"
  if [[ ! "${family}" =~ (rhel|fedora|rocky|centos|almalinux) ]]; then
    die "host-setup currently supports Rocky/RHEL-family hosts only."
  fi
}

ensure_dnf_basics() {
  say "=== [1] Install basic packages ==="
  sudo dnf -y install ca-certificates curl git gnupg2 unzip jq
}

ensure_python3() {
  say "=== [2] Ensure Python 3 ==="
  if need_cmd python3; then
    ok "python3 already installed: $(python3 --version 2>&1)"
    return
  fi
  sudo dnf -y install python3 python3-pip
  ok "python3 installed: $(python3 --version 2>&1)"
}

ensure_snapd() {
  say "=== [3] Ensure snapd ==="
  if need_cmd snap; then
    ok "snap already available"
    return
  fi
  sudo dnf -y install epel-release
  sudo dnf -y install snapd
  sudo systemctl enable --now snapd.socket
  sudo ln -sf /var/lib/snapd/snap /snap
  ok "snapd installed and enabled"
}

ensure_opentofu() {
  say "=== [4] Ensure OpenTofu ==="
  if need_cmd tofu; then
    ok "OpenTofu already installed: $(tofu --version | head -n 1)"
    return
  fi
  curl --proto '=https' --tlsv1.2 -fsSL https://get.opentofu.org/install-opentofu.sh -o install-opentofu.sh
  chmod +x install-opentofu.sh
  ./install-opentofu.sh --install-method rpm
  rm -f install-opentofu.sh
  ok "OpenTofu installed: $(tofu --version | head -n 1)"
}

ensure_multipass() {
  say "=== [5] Ensure Multipass ==="
  if need_cmd multipass; then
    ok "Multipass already installed: $(multipass version | head -n 1)"
    return
  fi
  ensure_snapd
  sudo snap install multipass
  ok "Multipass installed"
}

ensure_kubectl_optional() {
  say "=== [6] Ensure kubectl (optional) ==="
  if need_cmd kubectl; then
    ok "kubectl already installed"
    return
  fi
  ensure_snapd
  sudo snap install kubectl --classic
  ok "kubectl installed"
}

ensure_helm_optional() {
  say "=== [7] Ensure helm (optional) ==="
  if need_cmd helm; then
    ok "helm already installed"
    return
  fi
  ensure_snapd
  sudo snap install helm --classic
  ok "helm installed"
}

print_next_steps() {
  echo
  ok "Host setup completed"
  echo "Next:"
  echo "  ./scripts/k8s-tool.sh up"
  echo "  ./scripts/k8s-tool.sh status"
  echo "  ./scripts/k8s-tool.sh addons-install base"
}

main() {
  require_linux
  ensure_dnf_basics
  ensure_python3
  ensure_opentofu
  ensure_multipass
  ensure_kubectl_optional
  ensure_helm_optional
  print_next_steps
}

main "$@"
