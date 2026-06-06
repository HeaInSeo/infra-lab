#!/usr/bin/env bash
# cilium-install.sh — Cilium 설치 (순수 설치만 담당).
#
# 이 스크립트의 역할:
#   - Cilium helm install (Gateway API + L2 Announcements + LB IPAM)
#   - Gateway API CRDs 설치
#   - Cilium LB IPAM pool + L2 Announcement Policy 생성
#
# 이 스크립트가 하지 않는 것:
#   - Flannel 제거 — flannel-to-cilium.sh 에서 수행
#   - CNI 잔여 네트워크 디바이스 삭제 — 노드 직접 접근이 필요한 작업은
#     이 스크립트의 역할이 아님
#   - 노드 SSH 접속
#
# 사전 조건:
#   - kubectl, helm이 PATH에 있어야 한다.
#   - KUBECONFIG가 대상 클러스터를 가리켜야 한다.
#   - 클러스터에 다른 CNI(Flannel 등)가 설치되어 있으면 안 된다.
#     (Flannel에서 전환하려면 scripts/host/flannel-to-cilium.sh 를 사용할 것)
#
# 환경변수:
#   KUBECONFIG         — 대상 클러스터 kubeconfig (필수)
#   LB_IPAM_CIDR       — LB IPAM에 할당할 IP 대역 (기본: 192.168.122.200/32)
#   CILIUM_VERSION     — Cilium helm chart 버전 (기본: 1.19.4)
#
# 사용법:
#   KUBECONFIG=state/test-wizard-env/kubeconfig bash scripts/host/cilium-install.sh
set -euo pipefail

CILIUM_VERSION="${CILIUM_VERSION:-1.19.4}"
LB_IPAM_CIDR="${LB_IPAM_CIDR:-192.168.122.200/32}"
GATEWAY_API_CRD_URL="https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.1/standard-install.yaml"

echo "[cilium-install] KUBECONFIG=${KUBECONFIG:-<default>}"
echo "[cilium-install] Cilium version: ${CILIUM_VERSION}"
echo "[cilium-install] LB IPAM CIDR: ${LB_IPAM_CIDR}"

# ── guard: Flannel이 설치되어 있으면 중단 ─────────────────────────────────────

if kubectl get daemonset kube-flannel-ds -n kube-flannel >/dev/null 2>&1; then
  echo "[cilium-install] ERROR: Flannel daemonset detected."
  echo "[cilium-install] Direct Cilium install on a Flannel cluster will cause CNI conflicts."
  echo "[cilium-install] Use scripts/host/flannel-to-cilium.sh for migration."
  exit 1
fi

# ── 0. API server 주소 자동 감지 ──────────────────────────────────────────────

K8S_API_HOST=$(kubectl get endpoints kubernetes -o jsonpath='{.subsets[0].addresses[0].ip}')
K8S_API_PORT=$(kubectl get endpoints kubernetes -o jsonpath='{.subsets[0].ports[0].port}')
echo "[cilium-install] API server: ${K8S_API_HOST}:${K8S_API_PORT}"

# ── 1. kube-proxy 제거 (kubeProxyReplacement 모드) ───────────────────────────
# kube-proxy 제거는 Cilium kubeProxyReplacement 설정의 일부로 여기서 수행한다.
# Flannel cleanup이 아닌 Cilium 설치 요건이기 때문에 이 스크립트에 속한다.

if kubectl get daemonset kube-proxy -n kube-system >/dev/null 2>&1; then
  echo "[cilium-install] removing kube-proxy (kubeProxyReplacement mode)..."
  kubectl -n kube-system delete daemonset kube-proxy --ignore-not-found
  kubectl -n kube-system delete configmap kube-proxy --ignore-not-found
fi

# ── 2. Cilium helm install ───────────────────────────────────────────────────

echo "[cilium-install] installing Cilium ${CILIUM_VERSION}..."
helm upgrade --install cilium cilium/cilium \
  --version "${CILIUM_VERSION}" \
  --namespace kube-system \
  --set kubeProxyReplacement=true \
  --set k8sServiceHost="${K8S_API_HOST}" \
  --set k8sServicePort="${K8S_API_PORT}" \
  --set gatewayAPI.enabled=true \
  --set l2announcements.enabled=true \
  --set externalIPs.enabled=true \
  --set socketLB.enabled=true \
  --set nodePort.enabled=true \
  --wait \
  --timeout 10m

echo "[cilium-install] Cilium installed."

# ── 3. 모든 노드 Ready 대기 ───────────────────────────────────────────────────

echo "[cilium-install] waiting for nodes to become Ready..."
sleep 10
kubectl wait nodes --all --for=condition=Ready --timeout=180s

# ── 4. Gateway API CRDs ──────────────────────────────────────────────────────

if ! kubectl get crd gateways.gateway.networking.k8s.io >/dev/null 2>&1; then
  echo "[cilium-install] installing Gateway API CRDs..."
  kubectl apply -f "${GATEWAY_API_CRD_URL}"
else
  echo "[cilium-install] Gateway API CRDs already present."
fi

# ── 5. Cilium LB IPAM pool + L2 Announcement Policy ─────────────────────────

echo "[cilium-install] applying LB IPAM pool (${LB_IPAM_CIDR})..."
kubectl apply -f - <<EOF
apiVersion: "cilium.io/v2"
kind: CiliumLoadBalancerIPPool
metadata:
  name: lab-pool
spec:
  blocks:
    - cidr: "${LB_IPAM_CIDR}"
EOF

kubectl apply -f - <<EOF
apiVersion: "cilium.io/v2alpha1"
kind: CiliumL2AnnouncementPolicy
metadata:
  name: lab-l2-policy
spec:
  loadBalancerIPs: true
  interfaces:
    - "^enp.*"
    - "^eth.*"
  nodeSelector:
    matchLabels:
      kubernetes.io/os: linux
EOF

# ── 6. GatewayClass (Cilium operator가 CRD 설치 전 시작된 경우 자동 생성하지 않음) ──

echo "[cilium-install] ensuring GatewayClass 'cilium' exists..."
kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: cilium
spec:
  controllerName: io.cilium/gateway-controller
EOF

# ── 완료 ──────────────────────────────────────────────────────────────────────

echo ""
echo "[cilium-install] ✓ Done."
echo "[cilium-install]   Cilium:      ${CILIUM_VERSION}"
echo "[cilium-install]   LB IPAM:     ${LB_IPAM_CIDR}"
echo "[cilium-install]   Gateway API: ready"
