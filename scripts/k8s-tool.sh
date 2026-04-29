#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOL="${TOOL:-tofu}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-${ROOT_DIR}/kubeconfig}"
BLOCKED_LOCAL_TS_IP="${BLOCKED_LOCAL_TS_IP:-100.92.45.46}"

if [[ -f "$KUBECONFIG_PATH" ]]; then
  export KUBECONFIG="$KUBECONFIG_PATH"
fi

usage() {
  cat <<'USAGE'
Usage: scripts/k8s-tool.sh <command> [args]

Commands:
  host-setup                         Install or verify host prerequisites
  host-cleanup                       Remove host-installed tools (requires FORCE=1)
  up                                 Create VMs and bootstrap the baseline cluster
  down                               Destroy VMs and OpenTofu-managed resources
  status                             Show cluster or VM status
  clean                              Remove local state files (requires FORCE=1)
  addons-install <base|optional> [name]
  addons-uninstall <base|optional> [name]
  addons-verify [base|optional] [name]

Env:
  TOOL=tofu|terraform
  KUBECONFIG_PATH=/path/to/kubeconfig
  FORCE=1
USAGE
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing: $1" >&2
    exit 1
  }
}

local_tailscale_ip() {
  if command -v tailscale >/dev/null 2>&1; then
    tailscale ip -4 2>/dev/null | head -n 1 || true
  fi
}

ensure_local_vm_allowed() {
  local ts_ip

  if [[ "${ALLOW_LOCAL_VM:-0}" == "1" ]]; then
    return 0
  fi

  ts_ip="$(local_tailscale_ip)"
  if [[ -n "$ts_ip" && "$ts_ip" == "$BLOCKED_LOCAL_TS_IP" ]]; then
    echo "local VM operations are blocked on ${ts_ip}; use the remote lab host instead" >&2
    echo "remote lab host: seoy@100.123.80.48" >&2
    echo "set ALLOW_LOCAL_VM=1 only if you intentionally want to bypass this guard" >&2
    exit 1
  fi
}

reconcile_multipass_state() {
  local state_file="${ROOT_DIR}/terraform.tfstate"
  local missing=0

  [[ -f "$state_file" ]] || return 0
  command -v jq >/dev/null 2>&1 || return 0
  command -v multipass >/dev/null 2>&1 || return 0

  while IFS=$'\t' read -r resource_name index_key vm_name; do
    [[ -n "$resource_name" && -n "$index_key" && -n "$vm_name" ]] || continue
    if multipass info "$vm_name" >/dev/null 2>&1; then
      continue
    fi

    echo "[WARN] state has null_resource.${resource_name}[${index_key}] but VM ${vm_name} is missing; tainting for recreation"
    "$TOOL" taint -allow-missing "null_resource.${resource_name}[${index_key}]" >/dev/null
    missing=1
  done < <(
    jq -r '
      .resources[]
      | select(.type == "null_resource" and (.name == "masters" or .name == "workers"))
      | .name as $name
      | .instances[]
      | [$name, (.index_key | tostring), .attributes.triggers.name]
      | @tsv
    ' "$state_file"
  )

  if [[ "$missing" -eq 1 ]]; then
    for address in null_resource.init_cluster null_resource.join_all; do
      "$TOOL" taint -allow-missing "$address" >/dev/null 2>&1 || true
    done
  fi
}

cmd="${1:-}"
if [[ -z "$cmd" ]]; then
  usage
  exit 1
fi

shift || true

case "$cmd" in
  host-setup)
    ensure_local_vm_allowed
    bash "${ROOT_DIR}/scripts/host/setup-host-rocky8.sh"
    ;;
  host-cleanup)
    ensure_local_vm_allowed
    bash "${ROOT_DIR}/scripts/host/cleanup-host-rocky8.sh"
    ;;
  up)
    ensure_local_vm_allowed
    need_cmd "$TOOL"
    (
      cd "${ROOT_DIR}"
      "$TOOL" init
      reconcile_multipass_state
      "$TOOL" plan
      "$TOOL" apply -auto-approve
    )
    ;;
  down)
    ensure_local_vm_allowed
    need_cmd "$TOOL"
    (
      cd "${ROOT_DIR}"
      "$TOOL" destroy -auto-approve
    )
    ;;
  status)
    if [[ -f "$KUBECONFIG_PATH" ]] && command -v kubectl >/dev/null 2>&1; then
      export KUBECONFIG="$KUBECONFIG_PATH"
      echo "== Nodes =="
      kubectl get nodes -o wide || true
      echo
      echo "== Pods =="
      kubectl get pods -A -o wide || true
    elif command -v multipass >/dev/null 2>&1; then
      multipass list || true
    else
      echo "kubectl or multipass not found" >&2
      exit 1
    fi
    ;;
  clean)
    ensure_local_vm_allowed
    if [[ "${FORCE:-0}" != "1" ]]; then
      echo "FORCE=1 is required to clean local state files" >&2
      exit 1
    fi
    (
      cd "${ROOT_DIR}"
      rm -rf .terraform .terraform.lock.hcl terraform.tfstate* tofu.tfstate* tofu.tfstate.d
      rm -f "$KUBECONFIG_PATH"
    )
    ;;
  addons-install)
    bash "${ROOT_DIR}/addons/manage.sh" install "$@"
    ;;
  addons-uninstall)
    bash "${ROOT_DIR}/addons/manage.sh" uninstall "$@"
    ;;
  addons-verify)
    bash "${ROOT_DIR}/addons/manage.sh" verify "$@"
    ;;
  *)
    usage
    exit 1
    ;;
esac
