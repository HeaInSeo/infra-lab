#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Capture user-passed env values before sourcing the profile so they take precedence.
_env_BACKEND="${BACKEND:-}"
_env_TOOL="${TOOL:-}"
_env_BLOCKED_LOCAL_TS_IP="${BLOCKED_LOCAL_TS_IP:-}"
_env_AUTO_INSTALL_BASE_ADDONS="${AUTO_INSTALL_BASE_ADDONS:-}"
_env_LAB_HOST_MODE="${LAB_HOST_MODE:-}"
_env_LAB_REMOTE_SSH_TARGET="${LAB_REMOTE_SSH_TARGET:-}"
_env_LAB_REMOTE_REPO_PATH="${LAB_REMOTE_REPO_PATH:-}"
_env_LAB_REMOTE_SSH_CONFIG="${LAB_REMOTE_SSH_CONFIG:-}"
_env_CNI="${CNI:-}"
_env_ADDONS="${ADDONS:-}"
_env_NAME_PREFIX="${NAME_PREFIX:-}"
_env_ENV_NAME="${ENV_NAME:-}"
_env_KUBECONFIG_PATH_EXPLICIT="${KUBECONFIG_PATH:-}"
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

# Command-line env > profile values > built-in defaults
TOOL="${_env_TOOL:-${TOOL:-tofu}}"
BACKEND="${_env_BACKEND:-${BACKEND:-multipass}}"
BLOCKED_LOCAL_TS_IP="${_env_BLOCKED_LOCAL_TS_IP:-${BLOCKED_LOCAL_TS_IP:-100.92.45.46}}"
AUTO_INSTALL_BASE_ADDONS="${_env_AUTO_INSTALL_BASE_ADDONS:-${AUTO_INSTALL_BASE_ADDONS:-1}}"
LAB_HOST_MODE="${_env_LAB_HOST_MODE:-${LAB_HOST_MODE:-local}}"
LAB_REMOTE_SSH_TARGET="${_env_LAB_REMOTE_SSH_TARGET:-${LAB_REMOTE_SSH_TARGET:-}}"
LAB_REMOTE_REPO_PATH="${_env_LAB_REMOTE_REPO_PATH:-${LAB_REMOTE_REPO_PATH:-}}"
LAB_REMOTE_SSH_CONFIG="${_env_LAB_REMOTE_SSH_CONFIG:-${LAB_REMOTE_SSH_CONFIG:-/dev/null}}"
CNI="${_env_CNI:-${CNI:-flannel}}"
ADDONS="${_env_ADDONS:-${ADDONS:-}}"
# NAME_PREFIX mirrors TF_VAR_name_prefix so flannel-to-cilium.sh finds the right VMs
NAME_PREFIX="${_env_NAME_PREFIX:-${NAME_PREFIX:-${TF_VAR_name_prefix:-lab}}}"

# ENV_NAME identifies the environment for state isolation.
# Derived from HOST_PROFILE filename when not set explicitly.
ENV_NAME="${_env_ENV_NAME:-}"
if [[ -z "$ENV_NAME" && -n "$HOST_PROFILE" ]]; then
  _profile_base="$(basename "$HOST_PROFILE")"
  ENV_NAME="${_profile_base%.env*}"
fi

# STATE_DIR groups all per-environment files: tofu state, kubeconfig, metadata.
# Enabled only when ENV_NAME is known; otherwise falls back to legacy paths
# so existing environments without a profile continue to work unchanged.
if [[ -n "$ENV_NAME" ]]; then
  STATE_DIR="${ROOT_DIR}/state/${ENV_NAME}"
  KUBECONFIG_PATH="${_env_KUBECONFIG_PATH_EXPLICIT:-${STATE_DIR}/kubeconfig}"
else
  STATE_DIR=""
  if [[ "$BACKEND" == "libvirt" ]]; then
    KUBECONFIG_PATH="${_env_KUBECONFIG_PATH_EXPLICIT:-${KUBECONFIG_PATH:-${ROOT_DIR}/kubeconfig.libvirt}}"
  else
    KUBECONFIG_PATH="${_env_KUBECONFIG_PATH_EXPLICIT:-${KUBECONFIG_PATH:-${ROOT_DIR}/kubeconfig}}"
  fi
fi

# Keep the tofu kubeconfig_path variable in sync so modules write to the right place.
export TF_VAR_kubeconfig_path="$KUBECONFIG_PATH"

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
  profile-cilium-collect            Collect remote-seoy Cilium live snapshot (read-only)
  profile-cilium-verify             Verify remote-seoy Cilium baseline (read-only)
  profile-gateway-verify            Verify remote-seoy Gateway baseline (read-only)

Env:
  BACKEND=multipass|libvirt
  CNI=flannel|cilium|none         CNI for the cluster (default: flannel)
  ADDONS=                         Space-separated optional addons to auto-install after up
  HOST_PROFILE=envs/<name>.env    Environment profile (sets BACKEND, CNI, ADDONS, etc.)
  ENV_NAME=<name>                 Override the environment name (default: derived from HOST_PROFILE)
  LAB_HOST_MODE=local|remote
  LAB_REMOTE_SSH_TARGET=user@host
  LAB_REMOTE_REPO_PATH=/path/to/infra-lab
  LAB_REMOTE_SSH_CONFIG=/dev/null
  AUTO_INSTALL_BASE_ADDONS=1
  TOOL=tofu|terraform
  KUBECONFIG_PATH=/path/to/kubeconfig
  FORCE=1

State isolation (when HOST_PROFILE or ENV_NAME is set):
  state/<ENV_NAME>/
    terraform.tfstate   OpenTofu state
    kubeconfig          Cluster kubeconfig
    meta                Environment creation metadata
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
    [[ -n "$LAB_REMOTE_SSH_TARGET" ]] && echo "remote lab host: ${LAB_REMOTE_SSH_TARGET}" >&2
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
    "$TOOL" taint -allow-missing -state="$state_file" "null_resource.${resource_name}[${index_key}]" >/dev/null
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
      BACKEND|CNI|ADDONS|ENV_NAME|NAME_PREFIX|TOOL|KUBECONFIG_PATH|FORCE|ALLOW_LOCAL_VM|BLOCKED_LOCAL_TS_IP|VM_USER|TF_VAR_*)
        printf '%s=%q ' "$name" "$value"
        ;;
    esac
  done < <(env)
}

write_env_meta() {
  local meta_file git_commit git_branch
  if [[ -n "$STATE_DIR" ]]; then
    meta_file="${STATE_DIR}/meta"
  else
    meta_file="${KUBECONFIG_PATH}.meta"
  fi

  git_commit="$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || echo unknown)"
  git_branch="$(git -C "$ROOT_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)"

  cat > "$meta_file" <<EOF
infra_lab_git_commit=${git_commit}
infra_lab_git_branch=${git_branch}
env_name=${ENV_NAME}
backend=${BACKEND}
cni=${CNI}
name_prefix=${NAME_PREFIX}
addons=${ADDONS}
kubeconfig=${KUBECONFIG_PATH}
state_dir=${STATE_DIR}
created_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF

  echo "[INFO] environment metadata written: ${meta_file}"
}

run_profile_script() {
  local script_path="$1"

  [[ -x "$script_path" ]] || {
    echo "profile script is not executable: $script_path" >&2
    exit 1
  }

  bash "$script_path"
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
    [[ -n "$STATE_DIR" ]] && mkdir -p "$STATE_DIR"
    (
      cd "$(backend_dir)"
      _state_args=()
      [[ -n "$STATE_DIR" ]] && _state_args=("-state=${STATE_DIR}/terraform.tfstate")
      "$TOOL" init
      if [[ "$BACKEND" == "multipass" ]]; then
        if [[ -n "$STATE_DIR" ]]; then
          reconcile_multipass_state "${STATE_DIR}/terraform.tfstate"
        else
          reconcile_multipass_state "$(pwd)/terraform.tfstate"
        fi
      fi
      "$TOOL" plan "${_state_args[@]}"
      "$TOOL" apply -auto-approve "${_state_args[@]}"
    )
    if [[ "$AUTO_INSTALL_BASE_ADDONS" == "1" ]]; then
      echo "[INFO] install default base add-ons"
      bash "${ROOT_DIR}/addons/manage.sh" install base
      echo "[INFO] verify default base add-ons"
      bash "${ROOT_DIR}/addons/manage.sh" verify base
    fi
    if [[ "$CNI" == "cilium" ]]; then
      if [[ "$BACKEND" == "multipass" ]]; then
        echo "[INFO] CNI=cilium: running Flannel → Cilium migration"
        NAME_PREFIX="$NAME_PREFIX" bash "${ROOT_DIR}/scripts/cluster/flannel-to-cilium.sh"
      else
        echo "[WARN] CNI=cilium with BACKEND=${BACKEND}: auto-migration is not supported for this backend." >&2
        echo "[WARN] Run scripts/cluster/flannel-to-cilium.sh manually after setting the VM endpoints." >&2
      fi
    fi
    if [[ -n "$ADDONS" ]]; then
      read -ra _addon_list <<< "$ADDONS"
      for addon in "${_addon_list[@]}"; do
        echo "[INFO] install optional addon: ${addon}"
        bash "${ROOT_DIR}/addons/manage.sh" install optional "${addon}"
        bash "${ROOT_DIR}/addons/manage.sh" verify optional "${addon}"
      done
    fi
    write_env_meta
    ;;
  down)
    ensure_local_vm_allowed
    need_cmd "$TOOL"
    (
      cd "$(backend_dir)"
      _state_args=()
      [[ -n "$STATE_DIR" ]] && _state_args=("-state=${STATE_DIR}/terraform.tfstate")
      "$TOOL" destroy -auto-approve "${_state_args[@]}"
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
    if [[ -n "$STATE_DIR" ]]; then
      rm -rf "${STATE_DIR}"
      echo "[INFO] state directory removed: ${STATE_DIR}"
    else
      (
        cd "$(backend_dir)"
        rm -rf .terraform terraform.tfstate* tofu.tfstate* tofu.tfstate.d
        rm -f "$KUBECONFIG_PATH" "${KUBECONFIG_PATH}.meta"
      )
    fi
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
  profile-cilium-collect)
    run_profile_script "${ROOT_DIR}/profiles/remote-seoy/cilium/verify/collect-live-state.sh"
    ;;
  profile-cilium-verify)
    run_profile_script "${ROOT_DIR}/profiles/remote-seoy/cilium/verify/verify-cilium.sh"
    ;;
  profile-gateway-verify)
    run_profile_script "${ROOT_DIR}/profiles/remote-seoy/cilium/verify/verify-gateway.sh"
    ;;
  *)
    usage
    exit 1
    ;;
esac
