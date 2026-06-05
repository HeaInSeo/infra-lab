#!/usr/bin/env bash
# Write /etc/infra-lab/build.json to every cluster node.
# The file records which infra-lab version and configuration created the VM,
# enabling 'ilab vm version <node>' queries without external state lookups.
#
# JSON is base64-encoded before transmission to avoid shell quoting issues.
#
# Env:
#   NAME_PREFIX              VM name prefix          (default: lab)
#   MASTERS                  Control-plane count     (default: TF_VAR_masters or 1)
#   WORKERS                  Worker count            (default: TF_VAR_workers or 2)
#   BACKEND                  multipass|libvirt       (default: multipass)
#   ENV_NAME                 Environment profile name
#   CNI                      flannel|cilium|none     (default: flannel)
#   VM_RUNTIME               multipass|ssh           (default: multipass)
#   VM_USER                  Guest user              (default: ubuntu)
#   INFRA_LAB_GIT_COMMIT     Git commit hash
#   INFRA_LAB_GIT_BRANCH     Git branch name
#   SSH_PRIVATE_KEY_PATH     SSH key for libvirt/ssh runtime
#   MASTER_ENDPOINTS         Comma-separated IPs for libvirt masters (optional)
#   WORKER_ENDPOINTS         Comma-separated IPs for libvirt workers (optional)
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck disable=SC1090,SC1091
. "${ROOT_DIR}/scripts/runtime/lib.sh"

NAME_PREFIX="${NAME_PREFIX:-lab}"
MASTERS="${MASTERS:-${TF_VAR_masters:-1}}"
WORKERS="${WORKERS:-${TF_VAR_workers:-2}}"
BACKEND="${BACKEND:-multipass}"
ENV_NAME="${ENV_NAME:-}"
CNI="${CNI:-flannel}"
INFRA_LAB_GIT_COMMIT="${INFRA_LAB_GIT_COMMIT:-unknown}"
INFRA_LAB_GIT_BRANCH="${INFRA_LAB_GIT_BRANCH:-unknown}"
MASTER_ENDPOINTS="${MASTER_ENDPOINTS:-}"
WORKER_ENDPOINTS="${WORKER_ENDPOINTS:-}"

require_runtime

_write_build_json() {
  local endpoint="$1"
  local role="$2"
  local node_name="$3"
  local k8s_version created_at json json_b64

  k8s_version="$(vm_exec "$endpoint" \
    "kubelet --version 2>/dev/null || echo unknown" | awk '{print $NF}')"
  created_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

  json="$(printf \
    '{"schemaVersion":"infra-lab.vm.v1","infraLabGitCommit":"%s","infraLabGitBranch":"%s","envName":"%s","backend":"%s","cni":"%s","role":"%s","nodeName":"%s","kubernetesVersion":"%s","createdAt":"%s"}' \
    "$INFRA_LAB_GIT_COMMIT" "$INFRA_LAB_GIT_BRANCH" \
    "$ENV_NAME" "$BACKEND" "$CNI" \
    "$role" "$node_name" "$k8s_version" "$created_at")"

  # Encode to base64 so the JSON can be embedded in a shell command safely.
  json_b64="$(printf '%s' "$json" | base64 -w0)"

  vm_exec "$endpoint" \
    "sudo mkdir -p /etc/infra-lab && printf '%s' ${json_b64} | base64 -d | sudo tee /etc/infra-lab/build.json > /dev/null"

  echo "[INFO] build.json written: ${endpoint} (${role})"
}

_endpoints_from_prefix() {
  local prefix="$1"
  local count="$2"
  local i
  for ((i = 0; i < count; i++)); do
    printf '%s\n' "${prefix}-${i}"
  done
}

if [[ "$VM_RUNTIME" == "multipass" ]]; then
  # VM names are predictable from the prefix.
  while IFS= read -r node; do
    _write_build_json "$node" "control-plane" "$node"
  done < <(_endpoints_from_prefix "${NAME_PREFIX}-master" "$MASTERS")

  while IFS= read -r node; do
    _write_build_json "$node" "worker" "$node"
  done < <(_endpoints_from_prefix "${NAME_PREFIX}-worker" "$WORKERS")

elif [[ "$VM_RUNTIME" == "ssh" ]]; then
  # For SSH runtime, endpoints must be supplied as comma-separated IP lists.
  if [[ -z "$MASTER_ENDPOINTS" && -z "$WORKER_ENDPOINTS" ]]; then
    echo "[WARN] VM_RUNTIME=ssh: set MASTER_ENDPOINTS and WORKER_ENDPOINTS to write build.json" >&2
    exit 0
  fi

  IFS=',' read -ra _masters <<< "$MASTER_ENDPOINTS"
  for ip in "${_masters[@]}"; do
    [[ -n "$ip" ]] || continue
    _name="$(vm_exec "$ip" "hostname 2>/dev/null" || echo "$ip")"
    _write_build_json "$ip" "control-plane" "$_name"
  done

  IFS=',' read -ra _workers <<< "$WORKER_ENDPOINTS"
  for ip in "${_workers[@]}"; do
    [[ -n "$ip" ]] || continue
    _name="$(vm_exec "$ip" "hostname 2>/dev/null" || echo "$ip")"
    _write_build_json "$ip" "worker" "$_name"
  done

else
  echo "[WARN] write-build-json.sh: unsupported VM_RUNTIME=${VM_RUNTIME}" >&2
  exit 1
fi
