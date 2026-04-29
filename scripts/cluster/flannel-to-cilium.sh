#!/usr/bin/env bash
# scripts/cluster/flannel-to-cilium.sh
#
# Flannel CNI → Cilium CNI 마이그레이션
# 대상: infra-lab (lab-master-0, lab-worker-0, lab-worker-1)
#
# 사전 조건:
#   - multipass CLI 사용 가능 (VM 접근)
#   - kubectl 사용 가능 (KUBECONFIG 설정됨)
#   - helm 사용 가능
#   - 인터넷 접근 가능 (Cilium Helm chart, Gateway API CRD 다운로드)
#
# 사용법:
#   NAME_PREFIX=lab scripts/cluster/flannel-to-cilium.sh
#
# 주의:
#   - 스크립트 실행 중 Pod 네트워크가 일시 단절된다.
#   - MetalLB 가 설치된 경우 먼저 제거한다.
#   - 마이그레이션 완료 후 기존 Pod 를 재시작하여 새 IP를 받게 해야 할 수 있다.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

NAME_PREFIX="${NAME_PREFIX:-lab}"
MASTERS="${MASTERS:-1}"
WORKERS="${WORKERS:-2}"
FLANNEL_MANIFEST="${FLANNEL_MANIFEST:-https://raw.githubusercontent.com/flannel-io/flannel/v0.26.0/Documentation/kube-flannel.yml}"

# ── 노드 목록 구성 ─────────────────────────────────────────────────────────
NODES=()
for i in $(seq 0 $((MASTERS - 1))); do
  NODES+=("${NAME_PREFIX}-master-${i}")
done
for i in $(seq 0 $((WORKERS - 1))); do
  NODES+=("${NAME_PREFIX}-worker-${i}")
done

echo "[INFO] 마이그레이션 대상 노드: ${NODES[*]}"
echo "[INFO] 현재 노드 상태:"
kubectl get nodes -o wide
echo ""

# ── Step 1: MetalLB 제거 (설치된 경우) ────────────────────────────────────
echo "[STEP 1] MetalLB 제거 (설치된 경우)"
if kubectl -n metallb-system get deployment controller >/dev/null 2>&1; then
  echo "[INFO] MetalLB 감지됨 — 제거 중..."
  bash "${ROOT_DIR}/addons/optional/metallb/uninstall.sh"
  echo "[INFO] MetalLB 제거 완료"
else
  echo "[INFO] MetalLB 미설치 — 건너뜀"
fi

# ── Step 2: Flannel 제거 ───────────────────────────────────────────────────
echo "[STEP 2] Flannel 제거"
if kubectl -n kube-flannel get ds kube-flannel-ds >/dev/null 2>&1; then
  echo "[INFO] Flannel DaemonSet 삭제 중..."
  kubectl delete -f "${FLANNEL_MANIFEST}" --ignore-not-found
  kubectl -n kube-flannel wait --for=delete ds/kube-flannel-ds --timeout=60s 2>/dev/null || true
  echo "[INFO] Flannel namespace 삭제 중..."
  kubectl delete namespace kube-flannel --ignore-not-found
else
  echo "[INFO] Flannel DaemonSet 미발견 — 건너뜀"
fi

# ── Step 3: 각 노드에서 CNI 파일 정리 (multipass exec) ────────────────────
echo "[STEP 3] 각 노드 CNI 파일 정리"
for node in "${NODES[@]}"; do
  echo "[INFO] ${node}: CNI 파일 정리 중..."
  multipass exec "${node}" -- sudo bash -c '
    # flannel CNI 설정 제거
    rm -f /etc/cni/net.d/10-flannel.conflist
    rm -f /etc/cni/net.d/10-flannel.conf

    # flannel 네트워크 인터페이스 제거
    ip link delete flannel.1 2>/dev/null || true
    ip link delete flannel0   2>/dev/null || true

    # iptables flannel 규칙 정리 (오류 무시)
    iptables-save 2>/dev/null | grep -v FLANNEL | iptables-restore 2>/dev/null || true

    echo "  CNI 정리 완료"
  '
done

# ── Step 4: Cilium 설치 ────────────────────────────────────────────────────
echo "[STEP 4] Cilium 설치"
bash "${ROOT_DIR}/addons/optional/cilium/install.sh"

# ── Step 5: 노드 Ready 확인 ───────────────────────────────────────────────
echo "[STEP 5] 노드 Ready 대기 (최대 5분)"
kubectl wait --for=condition=Ready nodes --all --timeout=300s

echo ""
echo "[INFO] 최종 노드 상태:"
kubectl get nodes -o wide

# ── Step 6: 시스템 Pod 재시작 (기존 Flannel IP 해제) ──────────────────────
echo "[STEP 6] kube-system Pod 재시작 (coredns 등 — 새 IP 획득)"
kubectl -n kube-system rollout restart deployment/coredns 2>/dev/null || true
kubectl -n kube-system rollout status deployment/coredns --timeout=120s || true

# ── Step 7: 검증 ──────────────────────────────────────────────────────────
echo "[STEP 7] Cilium 상태 검증"
bash "${ROOT_DIR}/addons/optional/cilium/verify.sh"

echo ""
echo "=================================================="
echo " Sprint I-1 완료: Flannel → Cilium 마이그레이션"
echo " 다음 단계: Sprint I-2 (Harbor Helm + HTTPRoute)"
echo "=================================================="
