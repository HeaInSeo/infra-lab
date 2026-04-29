#!/usr/bin/env bash
set -euo pipefail

NAME_PREFIX="${NAME_PREFIX:-lab}"
MASTERS="${MASTERS:-1}"
WORKERS="${WORKERS:-2}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-./kubeconfig}"
VM_USER="${VM_USER:-ubuntu}"
VM_HOME="/home/${VM_USER}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing: $1" >&2
    exit 1
  }
}

need multipass

MASTER0="${NAME_PREFIX}-master-0"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

JOIN_SH="${tmpdir}/join.sh"
JOIN_CP_SH="${tmpdir}/join-controlplane.sh"

echo "[INFO] fetch join scripts from ${MASTER0}"
multipass exec "${MASTER0}" -- cat "${VM_HOME}/join.sh" > "${JOIN_SH}"
multipass exec "${MASTER0}" -- cat "${VM_HOME}/join-controlplane.sh" > "${JOIN_CP_SH}"
chmod +x "${JOIN_SH}" "${JOIN_CP_SH}"

if [[ "${MASTERS}" -gt 1 ]]; then
  for ((i=1; i<MASTERS; i++)); do
    node="${NAME_PREFIX}-master-${i}"
    < "${JOIN_CP_SH}" multipass exec "${node}" -- sudo bash -c "cat > ${VM_HOME}/join-controlplane.sh"
    multipass exec "${node}" -- bash -lc "\
      if [[ -f /etc/kubernetes/kubelet.conf ]]; then \
        echo '[INFO] already joined; skip'; \
      else \
        sudo chmod +x ${VM_HOME}/join-controlplane.sh && sudo bash ${VM_HOME}/join-controlplane.sh; \
      fi"
  done
fi

if [[ "${WORKERS}" -gt 0 ]]; then
  for ((i=0; i<WORKERS; i++)); do
    node="${NAME_PREFIX}-worker-${i}"
    < "${JOIN_SH}" multipass exec "${node}" -- sudo bash -c "cat > ${VM_HOME}/join.sh"
    multipass exec "${node}" -- bash -lc "\
      if [[ -f /etc/kubernetes/kubelet.conf ]]; then \
        echo '[INFO] already joined; skip'; \
      else \
        sudo chmod +x ${VM_HOME}/join.sh && sudo bash ${VM_HOME}/join.sh; \
      fi"
  done
fi

echo "[INFO] export kubeconfig from ${MASTER0}"
multipass exec "${MASTER0}" -- bash -lc "\
  sudo mkdir -p ${VM_HOME}/.kube && \
  sudo cp /etc/kubernetes/admin.conf ${VM_HOME}/.kube/config && \
  sudo chown ${VM_USER}:${VM_USER} ${VM_HOME}/.kube/config"

mkdir -p "$(dirname "${KUBECONFIG_PATH}")" 2>/dev/null || true
multipass exec "${MASTER0}" -- cat "${VM_HOME}/.kube/config" > "${KUBECONFIG_PATH}"

echo "[OK] kubeconfig written: ${KUBECONFIG_PATH}"
