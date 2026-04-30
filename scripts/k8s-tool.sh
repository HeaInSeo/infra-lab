#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOL="${TOOL:-tofu}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-${ROOT_DIR}/kubeconfig}"
BLOCKED_LOCAL_TS_IP="${BLOCKED_LOCAL_TS_IP:-100.92.45.46}"
BACKEND="${BACKEND:-multipass}"
AUTO_INSTALL_BASE_ADDONS="${AUTO_INSTALL_BASE_ADDONS:-1}"
LAB_HOST_MODE="${LAB_HOST_MODE:-local}"
LAB_REMOTE_SSH_TARGET="${LAB_REMOTE_SSH_TARGET:-}"
LAB_REMOTE_REPO_PATH="${LAB_REMOTE_REPO_PATH:-}"
LAB_REMOTE_SSH_CONFIG="${LAB_REMOTE_SSH_CONFIG:-/dev/null}"
HOST_PROFILE="${HOST_PROFILE:-}"

if [[ -n "$HOST_PROFILE" ]]; then
  profile_path="$HOST_PROFILE"
  if [[ "$profile_path" != /* ]]; then
    profile_path="${ROOT_DIR}/${profile_path}"
  fi
  if [[ -f "$profile_path" ]]; then
    # shellcheck disable=SC1090
    . "$profile_path"
  else
    echo "host profile not found: $profile_path" >&2
    exit 1
  fi
fi

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
  BACKEND=multipass|libvirt
  HOST_PROFILE=hosts/<profile>.env
  LAB_HOST_MODE=local|remote
  LAB_REMOTE_SSH_TARGET=user@host
  LAB_REMOTE_REPO_PATH=/path/to/infra-lab
  LAB_REMOTE_SSH_CONFIG=/dev/null
  AUTO_INSTALL_BASE_ADDONS=1
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

backend_dir() {
  case "$BACKEND" in
    multipass)
      printf '%s\n' "$ROOT_DIR"
      ;;
    libvirt)
      printf '%s\n' "${ROOT_DIR}/backends/libvirt"
      ;;
    *)
      echo "unknown backend: $BACKEND" >&2
      exit 1
      ;;
  esac
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
  local state_file="$1"
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
      "$TOOL" taint -allow-missing -state="$state_file" "$address" >/dev/null 2>&1 || true
    done
  fi
}

passthrough_env() {
  while IFS='=' read -r name value; do
    case "$name" in
      BACKEND|TOOL|KUBECONFIG_PATH|FORCE|ALLOW_LOCAL_VM|BLOCKED_LOCAL_TS_IP|VM_USER|TF_VAR_*)
        printf '%s=%q ' "$name" "$value"
        ;;
    esac
  done < <(env)
}

run_remote() {
  local remote_cmd env_prefix args_quoted ssh_opts

  [[ -n "$LAB_REMOTE_SSH_TARGET" ]] || {
    echo "LAB_REMOTE_SSH_TARGET is required when LAB_HOST_MODE=remote" >&2
    exit 1
  }
  [[ -n "$LAB_REMOTE_REPO_PATH" ]] || {
    echo "LAB_REMOTE_REPO_PATH is required when LAB_HOST_MODE=remote" >&2
    exit 1
  }

  env_prefix="$(passthrough_env)"
  printf -v args_quoted '%q ' "$cmd" "$@"
  remote_cmd="cd $(printf '%q' "$LAB_REMOTE_REPO_PATH") && ${env_prefix}LAB_HOST_MODE=local bash scripts/k8s-tool.sh ${args_quoted}"
  ssh_opts=(
    -F "$LAB_REMOTE_SSH_CONFIG"
    -o StrictHostKeyChecking=no
    -o UserKnownHostsFile=/dev/null
    -o BatchMode=yes
    -o ConnectTimeout=10
  )
  ssh "${ssh_opts[@]}" "$LAB_REMOTE_SSH_TARGET" "$remote_cmd"
}

cmd="${1:-}"
if [[ -z "$cmd" ]]; then
  usage
  exit 1
fi

shift || true

if [[ "$LAB_HOST_MODE" == "remote" ]]; then
  run_remote "$@"
  exit 0
fi

case "$cmd" in
  host-setup)
    ensure_local_vm_allowed
    case "$BACKEND" in
      multipass)
        bash "${ROOT_DIR}/scripts/host/setup-host-rocky8.sh"
        ;;
      libvirt)
        bash "${ROOT_DIR}/scripts/host/setup-host-libvirt.sh"
        ;;
    esac
    ;;
  host-cleanup)
    ensure_local_vm_allowed
    case "$BACKEND" in
      multipass)
        bash "${ROOT_DIR}/scripts/host/cleanup-host-rocky8.sh"
        ;;
      libvirt)
        echo "host-cleanup is not implemented for the libvirt backend" >&2
        exit 1
        ;;
    esac
    ;;
  up)
    ensure_local_vm_allowed
    need_cmd "$TOOL"
    (
      cd "$(backend_dir)"
      "$TOOL" init
      if [[ "$BACKEND" == "multipass" ]]; then
        reconcile_multipass_state "$(pwd)/terraform.tfstate"
      fi
      "$TOOL" plan
      "$TOOL" apply -auto-approve
    )
    if [[ "$AUTO_INSTALL_BASE_ADDONS" == "1" ]]; then
      echo "[INFO] install default base add-ons"
      bash "${ROOT_DIR}/addons/manage.sh" install base
      echo "[INFO] verify default base add-ons"
      bash "${ROOT_DIR}/addons/manage.sh" verify base
    fi
    ;;
  down)
    ensure_local_vm_allowed
    need_cmd "$TOOL"
    (
      cd "$(backend_dir)"
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
    elif [[ "$BACKEND" == "libvirt" ]] && command -v virsh >/dev/null 2>&1; then
      virsh list --all || true
    elif command -v multipass >/dev/null 2>&1; then
      multipass list || true
    else
      echo "kubectl, multipass, or virsh not found" >&2
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
      cd "$(backend_dir)"
      rm -rf .terraform .terraform.lock.hcl terraform.tfstate* tofu.tfstate* tofu.tfstate.d
      if [[ "$BACKEND" == "multipass" ]]; then
        rm -f "$KUBECONFIG_PATH"
      fi
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
