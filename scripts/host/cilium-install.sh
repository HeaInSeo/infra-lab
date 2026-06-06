#!/usr/bin/env bash
# cilium-install.sh — kubeadm 클러스터에 Cilium을 설치하거나 flannel에서 마이그레이션한다.
#
# 포함 기능:
#   - flannel 제거 (설치된 경우)
#   - kube-proxy 제거 (kubeProxyReplacement 모드)
#   - Cilium 1.19.x with Gateway API + L2 Announcements + LB IPAM
#   - Gateway API CRDs (standard channel)
#   - Cilium LB IPAM pool (HARBOR_LB_CIDR 기반)
#
# 사전 조건:
#   - kubectl, helm이 PATH에 있어야 한다.
#   - KUBECONFIG가 대상 클러스터를 가리켜야 한다.
#
# 환경변수:
#   KUBECONFIG         — 대상 클러스터 kubeconfig (필수)
#   HARBOR_LB_CIDR     — Harbor Gateway에 할당할 IP 대역 (기본: 192.168.122.200/32)
#   CILIUM_VERSION     — Cilium helm chart 버전 (기본: 1.19.4)
#
# 사용법:
#   KUBECONFIG=state/test-wizard-env/kubeconfig bash scripts/host/cilium-install.sh
set -euo pipefail

CILIUM_VERSION="${CILIUM_VERSION:-1.19.4}"
HARBOR_LB_CIDR="${HARBOR_LB_CIDR:-192.168.122.200/32}"

# Gateway API CRDs (standard channel)
GATEWAY_API_CRD_URL="https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.1/standard-install.yaml"

echo "[cilium-install] KUBECONFIG=${KUBECONFIG:-<default>}"
echo "[cilium-install] Cilium version: ${CILIUM_VERSION}"
echo "[cilium-install] LB IPAM CIDR: ${HARBOR_LB_CIDR}"

# ── 0. API server 주소 자동 감지 ──────────────────────────────────────────────

K8S_API_HOST=$(kubectl get endpoints kubernetes -o jsonpath='{.subsets[0].addresses[0].ip}')
K8S_API_PORT=$(kubectl get endpoints kubernetes -o jsonpath='{.subsets[0].ports[0].port}')
echo "[cilium-install] API server: ${K8S_API_HOST}:${K8S_API_PORT}"

# ── 1. flannel 제거 (있으면) ─────────────────────────────────────────────────

if kubectl get daemonset kube-flannel-ds -n kube-flannel >/dev/null 2>&1; then
  echo "[cilium-install] removing flannel..."
  kubectl delete -n kube-flannel daemonset kube-flannel-ds --ignore-not-found
  kubectl delete namespace kube-flannel --ignore-not-found
  # CNI 설정 파일 노드에서 정리 (flannel이 남긴 cni config가 Cilium 초기화를 방해할 수 있음)
  for NODE_IP in $(kubectl get nodes \
    -o jsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}'); do
    ssh -i "${SSH_KEY_PATH:-${HOME}/.ssh/id_ed25519}" \
      -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
      -o LogLevel=ERROR -o BatchMode=yes \
      "${VM_USER:-ubuntu}@${NODE_IP}" \
      "sudo rm -f /etc/cni/net.d/10-flannel.conflist && echo 'cni cleaned on ${NODE_IP}'" || true
  done
  echo "[cilium-install] flannel removed."
else
  echo "[cilium-install] flannel not found, skipping removal."
fi

# ── 2. kube-proxy 제거 (kubeProxyReplacement 모드) ───────────────────────────

if kubectl get daemonset kube-proxy -n kube-system >/dev/null 2>&1; then
  echo "[cilium-install] removing kube-proxy..."
  kubectl -n kube-system delete daemonset kube-proxy --ignore-not-found
  kubectl -n kube-system delete configmap kube-proxy --ignore-not-found
  echo "[cilium-install] kube-proxy removed."
fi

# ── 3. Cilium helm install ───────────────────────────────────────────────────

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

# ── 4. 모든 노드 Ready 대기 ──────────────────────────────────────────────────

echo "[cilium-install] waiting for nodes to become Ready..."
sleep 10
kubectl wait nodes --all --for=condition=Ready --timeout=180s

# ── 5. Gateway API CRDs 설치 ─────────────────────────────────────────────────

echo "[cilium-install] installing Gateway API CRDs..."
if ! kubectl get crd gateways.gateway.networking.k8s.io >/dev/null 2>&1; then
  kubectl apply -f "${GATEWAY_API_CRD_URL}"
  echo "[cilium-install] Gateway API CRDs installed."
else
  echo "[cilium-install] Gateway API CRDs already present."
fi

# ── 6. Cilium LB IPAM pool ───────────────────────────────────────────────────

echo "[cilium-install] applying LB IPAM pool (${HARBOR_LB_CIDR})..."
kubectl apply -f - <<EOF
apiVersion: "cilium.io/v2alpha1"
kind: CiliumLoadBalancerIPPool
metadata:
  name: lab-pool
spec:
  blocks:
    - cidr: "${HARBOR_LB_CIDR}"
EOF

kubectl apply -f - <<EOF
apiVersion: "cilium.io/v2alpha1"
kind: CiliumL2AnnouncementPolicy
metadata:
  name: lab-l2-policy
spec:
  loadBalancerIPs: true
  interfaces:
    - "^eth.*"
  nodeSelector:
    matchLabels:
      kubernetes.io/os: linux
EOF

echo "[cilium-install] LB IPAM pool and L2 announcement policy applied."

# ── 완료 ──────────────────────────────────────────────────────────────────────

echo ""
echo "[cilium-install] ✓ Done."
echo "[cilium-install]   Cilium:       ${CILIUM_VERSION}"
echo "[cilium-install]   LB IPAM:      ${HARBOR_LB_CIDR}"
echo "[cilium-install]   Gateway API:  ready"
