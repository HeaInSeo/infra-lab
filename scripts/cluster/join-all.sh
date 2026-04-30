#!/usr/bin/env bash
set -euo pipefail

NAME_PREFIX="${NAME_PREFIX:-lab}"
MASTERS="${MASTERS:-1}"
WORKERS="${WORKERS:-2}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-./kubeconfig}"
VM_RUNTIME="${VM_RUNTIME:-multipass}"
VM_USER="${VM_USER:-ubuntu}"
VM_HOME="/home/${VM_USER}"
MASTER0_ENDPOINT="${MASTER0_ENDPOINT:-}"
MASTER_ENDPOINTS="${MASTER_ENDPOINTS:-}"
WORKER_ENDPOINTS="${WORKER_ENDPOINTS:-}"

# shellcheck disable=SC1091
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/runtime/lib.sh"
require_runtime

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

JOIN_SH="${tmpdir}/join.sh"
JOIN_CP_SH="${tmpdir}/join-controlplane.sh"

if [[ "$VM_RUNTIME" == "multipass" ]]; then
  MASTER0_ENDPOINT="${MASTER0_ENDPOINT:-${NAME_PREFIX}-master-0}"
  if [[ "${MASTERS}" -gt 1 ]]; then
    master_list=()
    for ((i=1; i<MASTERS; i++)); do
      master_list+=("${NAME_PREFIX}-master-${i}")
    done
    MASTER_ENDPOINTS="$(IFS=,; echo "${master_list[*]}")"
  fi
  if [[ "${WORKERS}" -gt 0 ]]; then
    worker_list=()
    for ((i=0; i<WORKERS; i++)); do
      worker_list+=("${NAME_PREFIX}-worker-${i}")
    done
    WORKER_ENDPOINTS="$(IFS=,; echo "${worker_list[*]}")"
  fi
fi

[[ -n "$MASTER0_ENDPOINT" ]] || {
  echo "MASTER0_ENDPOINT is required" >&2
  exit 1
}

IFS=',' read -r -a master_endpoints <<< "$MASTER_ENDPOINTS"
IFS=',' read -r -a worker_endpoints <<< "$WORKER_ENDPOINTS"
expected_nodes=$((1 + ${#master_endpoints[@]} + ${#worker_endpoints[@]}))

echo "[INFO] fetch join scripts from ${MASTER0_ENDPOINT}"
vm_read_file "${MASTER0_ENDPOINT}" "${VM_HOME}/join.sh" > "${JOIN_SH}"
vm_read_file "${MASTER0_ENDPOINT}" "${VM_HOME}/join-controlplane.sh" > "${JOIN_CP_SH}"
chmod +x "${JOIN_SH}" "${JOIN_CP_SH}"

if [[ "${#master_endpoints[@]}" -gt 0 && -n "${master_endpoints[0]}" ]]; then
  for node in "${master_endpoints[@]}"; do
    vm_write_file "${node}" "${JOIN_CP_SH}" "${VM_HOME}/join-controlplane.sh"
    vm_exec "${node}" "\
      if [[ -f /etc/kubernetes/kubelet.conf ]]; then \
        echo '[INFO] already joined; skip'; \
      else \
        sudo chmod +x ${VM_HOME}/join-controlplane.sh && sudo bash ${VM_HOME}/join-controlplane.sh; \
      fi"
  done
fi

if [[ "${#worker_endpoints[@]}" -gt 0 && -n "${worker_endpoints[0]}" ]]; then
  for node in "${worker_endpoints[@]}"; do
    vm_write_file "${node}" "${JOIN_SH}" "${VM_HOME}/join.sh"
    vm_exec "${node}" "\
      if [[ -f /etc/kubernetes/kubelet.conf ]]; then \
        echo '[INFO] already joined; skip'; \
      else \
        sudo chmod +x ${VM_HOME}/join.sh && sudo bash ${VM_HOME}/join.sh; \
      fi"
  done
fi

echo "[INFO] export kubeconfig from ${MASTER0_ENDPOINT}"
vm_exec "${MASTER0_ENDPOINT}" "\
  sudo mkdir -p ${VM_HOME}/.kube && \
  sudo cp /etc/kubernetes/admin.conf ${VM_HOME}/.kube/config && \
  sudo chown ${VM_USER}:${VM_USER} ${VM_HOME}/.kube/config"

echo "[INFO] wait for ${expected_nodes} node(s) to become Ready"
vm_exec "${MASTER0_ENDPOINT}" "\
  export KUBECONFIG=${VM_HOME}/.kube/config && \
  deadline=\$((SECONDS + 300)) && \
  expected=${expected_nodes} && \
  while true; do \
    total=\$(kubectl get nodes --no-headers 2>/dev/null | wc -l) && \
    ready=\$(kubectl get nodes --no-headers 2>/dev/null | awk '\$2 == \"Ready\" {c++} END {print c+0}') && \
    if [[ \$total -ge \$expected && \$ready -ge \$expected ]]; then \
      kubectl get nodes -o wide; \
      break; \
    fi; \
    if (( SECONDS >= deadline )); then \
      kubectl get nodes -o wide || true; \
      echo '[ERROR] timed out waiting for nodes to become Ready' >&2; \
      exit 1; \
    fi; \
    sleep 5; \
  done"

mkdir -p "$(dirname "${KUBECONFIG_PATH}")" 2>/dev/null || true
vm_read_file "${MASTER0_ENDPOINT}" "${VM_HOME}/.kube/config" > "${KUBECONFIG_PATH}"

echo "[OK] kubeconfig written: ${KUBECONFIG_PATH}"
