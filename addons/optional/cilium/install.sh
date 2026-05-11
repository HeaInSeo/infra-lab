#!/usr/bin/env bash
# addons/optional/cilium/install.sh
# Cilium 1.16 설치 (Gateway API + L2 LB)
# 선행 조건: Flannel 이미 제거됨 (scripts/cluster/flannel-to-cilium.sh 실행 후)
#
# 중요:
# - 이 스크립트는 generic addon 설치 경로다.
# - remote-seoy live 기준선은 profiles/remote-seoy/cilium 에 별도 정리한다.
# - 운영 중인 기존 클러스터에 대해 IPAM mode 변경을 유발하는 업그레이드 경로로 사용하지 않는다.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"

CILIUM_VERSION="${CILIUM_VERSION:-1.16.5}"
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.2.0}"
CILIUM_NS="${CILIUM_NS:-kube-system}"

echo "[INFO] install optional addon: cilium ${CILIUM_VERSION}"

# ── 1. Gateway API CRD 설치 ────────────────────────────────────────────────
echo "[INFO] installing Gateway API CRDs ${GATEWAY_API_VERSION}"
kubectl apply -f \
  "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml"

# ── 2. Cilium Helm repo ────────────────────────────────────────────────────
helm repo add cilium https://helm.cilium.io/ --force-update
helm repo update cilium

# ── 3. Control-plane endpoint 감지 ────────────────────────────────────────
# kubeProxyReplacement=true 시 API 서버 주소를 명시해야 한다.
K8S_API_HOST="$(kubectl get endpoints kubernetes -o jsonpath='{.subsets[0].addresses[0].ip}')"
K8S_API_PORT="$(kubectl get endpoints kubernetes -o jsonpath='{.subsets[0].ports[0].port}')"
echo "[INFO] K8s API server: ${K8S_API_HOST}:${K8S_API_PORT}"

# ── 4. Cilium Helm 설치 ────────────────────────────────────────────────────
helm upgrade --install cilium cilium/cilium \
  --version "${CILIUM_VERSION}" \
  --namespace "${CILIUM_NS}" \
  --create-namespace \
  --values "${ROOT_DIR}/addons/values/cilium/values.yaml" \
  --set k8sServiceHost="${K8S_API_HOST}" \
  --set k8sServicePort="${K8S_API_PORT}" \
  --wait \
  --timeout 5m

# ── 5. DaemonSet rollout 대기 ─────────────────────────────────────────────
echo "[INFO] waiting for cilium DaemonSet rollout"
kubectl -n "${CILIUM_NS}" rollout status ds/cilium --timeout=300s

# ── 6. L2 LB 풀 적용 ──────────────────────────────────────────────────────
echo "[INFO] applying CiliumLoadBalancerIPPool + L2AnnouncementPolicy"
kubectl apply -f "${ROOT_DIR}/addons/values/cilium/l2pool.yaml"

echo "[INFO] cilium install complete"
