#!/usr/bin/env bash
# flannel-to-cilium.sh — Flannel CNI에서 Cilium으로 마이그레이션한다.
#
# 수행 순서:
#   1. Flannel kubernetes 리소스 제거 (DaemonSet, Namespace)
#   2. 각 노드의 Flannel 잔여물 정리 (flannel.1, cni config, subnet.env)
#   3. cilium-install.sh 호출
#
# 노드 접근 방식 (libvirt / direct SSH 전용):
#   이 스크립트는 노드 InternalIP로 직접 SSH를 사용한다.
#   - backend: libvirt 또는 SSH 접근이 가능한 환경
#   - multipass 환경에서는 `multipass exec <vm> -- ...` 방식이 필요하다 (미지원)
#   향후 infra-lab에 node access abstraction이 생기면 그쪽으로 이전할 것.
#
# 사전 조건:
#   - kubectl, helm이 PATH에 있어야 한다.
#   - KUBECONFIG가 대상 클러스터를 가리켜야 한다.
#   - SSH_KEY_PATH의 키로 모든 노드에 SSH 접근이 가능해야 한다.
#
# 환경변수:
#   KUBECONFIG    — 대상 클러스터 kubeconfig (필수)
#   SSH_KEY_PATH  — 노드 SSH 키 경로 (기본: ~/.ssh/id_ed25519)
#   VM_USER       — 노드 SSH 사용자 (기본: ubuntu)
#
# 사용법:
#   KUBECONFIG=state/test-wizard-env/kubeconfig bash scripts/host/flannel-to-cilium.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSH_KEY_PATH="${SSH_KEY_PATH:-${HOME}/.ssh/id_ed25519}"
VM_USER="${VM_USER:-ubuntu}"
SSH_OPTS="-i ${SSH_KEY_PATH} -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o BatchMode=yes -o ConnectTimeout=10"

echo "[flannel-to-cilium] KUBECONFIG=${KUBECONFIG:-<default>}"

# ── 1. Flannel 존재 확인 ───────────────────────────────────────────────────────

if ! kubectl get daemonset kube-flannel-ds -n kube-flannel >/dev/null 2>&1; then
  echo "[flannel-to-cilium] Flannel daemonset not found. Nothing to migrate."
  echo "[flannel-to-cilium] If you want a fresh Cilium install, use cilium-install.sh directly."
  exit 0
fi

# ── 2. Flannel kubernetes 리소스 제거 ─────────────────────────────────────────

echo "[flannel-to-cilium] removing Flannel resources from cluster..."
kubectl delete daemonset kube-flannel-ds -n kube-flannel --ignore-not-found
kubectl delete namespace kube-flannel --ignore-not-found
echo "[flannel-to-cilium] Flannel resources removed."

# ── 3. 각 노드에서 Flannel 잔여물 정리 ────────────────────────────────────────
# 정리 대상: Flannel이 남기는 것들만 제거한다.
#   - flannel.1    : Flannel VXLAN 인터페이스
#   - 10-flannel.conflist : Flannel CNI 설정 파일
#   - /run/flannel/subnet.env : Flannel subnet 정보
#
# 정리하지 않는 것:
#   - cilium_* 디바이스 : Cilium 자체가 관리하므로 여기서 건드리지 않는다.
#     Cilium 설치 실패 후 잔여물이 문제라면 Cilium 재설치로 해결해야 한다.

NODE_IPS=$(kubectl get nodes \
  -o jsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}')

echo "[flannel-to-cilium] cleaning Flannel residue on nodes: ${NODE_IPS}"

for NODE_IP in ${NODE_IPS}; do
  echo "[flannel-to-cilium]   -> ${NODE_IP}"
  # shellcheck disable=SC2029
  ssh ${SSH_OPTS} "${VM_USER}@${NODE_IP}" \
    "sudo ip link delete flannel.1 2>/dev/null || true
     sudo rm -f /etc/cni/net.d/10-flannel.conflist
     sudo rm -f /run/flannel/subnet.env
     echo 'flannel residue cleaned on ${NODE_IP}'" || {
    echo "[flannel-to-cilium] WARNING: could not clean node ${NODE_IP}, continuing..."
  }
done

echo "[flannel-to-cilium] Flannel residue cleaned."

# ── 4. Cilium 설치 ────────────────────────────────────────────────────────────

echo "[flannel-to-cilium] running cilium-install.sh..."
bash "${SCRIPT_DIR}/cilium-install.sh"
