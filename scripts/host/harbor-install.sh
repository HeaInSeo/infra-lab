#!/usr/bin/env bash
# harbor-install.sh — Harbor를 현재 kubeconfig 클러스터에 설치한다.
#
# 해결된 이슈 (v1 설치 시 발견):
#   1. HTTP 레지스트리 불가 — containerd v2.2.1 CRI path는 hosts.toml 기반
#      HTTP 레지스트리를 지원하지 않는다 (ctr --plain-http 는 작동하지만
#      kubelet → containerd CRI gRPC 경로는 무조건 HTTPS를 시도한다).
#      → TLS 활성화 + Harbor CA 인증서를 모든 노드에 배포하는 방식으로 해결.
#   2. containerd 재시작 후 kubelet cgroup 불일치 — containerd restart 이후
#      kubelet도 재시작하지 않으면 "expected cgroupsPath slice:prefix:name" 오류 발생.
#      → 이 스크립트에서는 재시작 순서를 containerd → kubelet으로 보장.
#   3. StorageClass 없음 — 기본 kubeadm 클러스터에는 SC가 없다.
#      → local-path-provisioner 자동 설치로 해결.
#
# 사전 조건:
#   - kubectl, helm이 PATH에 있어야 한다.
#   - harbor helm repo: helm repo add harbor https://helm.goharbor.io && helm repo update
#   - KUBECONFIG가 대상 클러스터를 가리켜야 한다.
#   - SSH_KEY_PATH가 노드 SSH 접근에 사용할 키 경로여야 한다.
#
# 환경변수:
#   KUBECONFIG           — 대상 클러스터 kubeconfig (필수)
#   HARBOR_ADMIN_PASSWORD — Harbor admin 패스워드 (기본: Harbor12345)
#   HARBOR_NODE_IP       — Harbor nodePort를 노출할 노드 IP (기본: master 자동감지)
#   HARBOR_NODEPORT_HTTPS — HTTPS NodePort 번호 (기본: 30003)
#   SSH_KEY_PATH         — 노드 SSH 키 경로 (기본: ~/.ssh/id_ed25519)
#   VM_USER              — 노드 SSH 사용자 (기본: ubuntu)
#
# 사용법:
#   KUBECONFIG=state/test-wizard-env/kubeconfig bash scripts/host/harbor-install.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

HARBOR_NAMESPACE="${HARBOR_NAMESPACE:-harbor}"
HARBOR_ADMIN_PASSWORD="${HARBOR_ADMIN_PASSWORD:-Harbor12345}"
HARBOR_NODEPORT_HTTPS="${HARBOR_NODEPORT_HTTPS:-30003}"
HARBOR_VALUES="${REPO_ROOT}/k8s/harbor/values.nodeport.yaml"
LOCAL_PATH_VERSION="v0.0.30"
SSH_KEY_PATH="${SSH_KEY_PATH:-${HOME}/.ssh/id_ed25519}"
VM_USER="${VM_USER:-ubuntu}"

# ── 0. master 노드 IP 자동 감지 ───────────────────────────────────────────────

if [[ -z "${HARBOR_NODE_IP:-}" ]]; then
  HARBOR_NODE_IP=$(kubectl get nodes \
    -l node-role.kubernetes.io/control-plane \
    -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
  echo "[harbor-install] auto-detected master IP: ${HARBOR_NODE_IP}"
fi

HARBOR_EXTERNAL_URL="https://${HARBOR_NODE_IP}:${HARBOR_NODEPORT_HTTPS}"

echo "[harbor-install] KUBECONFIG=${KUBECONFIG:-<default>}"
echo "[harbor-install] externalURL: ${HARBOR_EXTERNAL_URL}"
echo "[harbor-install] namespace: ${HARBOR_NAMESPACE}"

# ── 1. local-path-provisioner (StorageClass) ─────────────────────────────────

if ! kubectl get storageclass local-path >/dev/null 2>&1; then
  echo "[harbor-install] installing local-path-provisioner ${LOCAL_PATH_VERSION}..."
  kubectl apply -f "https://raw.githubusercontent.com/rancher/local-path-provisioner/${LOCAL_PATH_VERSION}/deploy/local-path-storage.yaml"
  kubectl annotate storageclass local-path storageclass.kubernetes.io/is-default-class=true --overwrite
  kubectl rollout status deployment/local-path-provisioner -n local-path-storage --timeout=120s
else
  echo "[harbor-install] local-path StorageClass already present, skipping."
fi

# ── 2. harbor namespace ───────────────────────────────────────────────────────

kubectl create namespace "${HARBOR_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# ── 3. helm install (TLS + auto cert) ────────────────────────────────────────

echo "[harbor-install] installing Harbor via Helm (TLS enabled)..."
helm upgrade --install harbor harbor/harbor \
  --namespace "${HARBOR_NAMESPACE}" \
  --values "${HARBOR_VALUES}" \
  --set externalURL="${HARBOR_EXTERNAL_URL}" \
  --set harborAdminPassword="${HARBOR_ADMIN_PASSWORD}" \
  --timeout 10m \
  --wait

echo "[harbor-install] Harbor pods ready."

# ── 4. Harbor CA 인증서 추출 ──────────────────────────────────────────────────

CA_CERT_TMP=$(mktemp)
echo "[harbor-install] extracting Harbor CA cert..."
kubectl get secret harbor-ingress -n "${HARBOR_NAMESPACE}" \
  -o jsonpath='{.data.ca\.crt}' | base64 -d > "${CA_CERT_TMP}"

if [[ ! -s "${CA_CERT_TMP}" ]]; then
  echo "[harbor-install] ERROR: CA cert is empty. Harbor TLS secret may not be ready."
  rm -f "${CA_CERT_TMP}"
  exit 1
fi
echo "[harbor-install] CA cert extracted ($(wc -c < "${CA_CERT_TMP}") bytes)."

# ── 5. 모든 노드에 CA 인증서 배포 ────────────────────────────────────────────

SSH_OPTS="-i ${SSH_KEY_PATH} -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o BatchMode=yes -o ConnectTimeout=10"
CERTS_DIR="/etc/containerd/certs.d/${HARBOR_NODE_IP}:${HARBOR_NODEPORT_HTTPS}"

NODE_IPS=$(kubectl get nodes \
  -o jsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}')

echo "[harbor-install] distributing CA cert to nodes: ${NODE_IPS}"

for NODE_IP in ${NODE_IPS}; do
  echo "[harbor-install]   -> ${NODE_IP}"
  scp ${SSH_OPTS} "${CA_CERT_TMP}" "${VM_USER}@${NODE_IP}:/tmp/harbor-ca.crt"
  # shellcheck disable=SC2029
  ssh ${SSH_OPTS} "${VM_USER}@${NODE_IP}" "
    sudo mkdir -p '${CERTS_DIR}'
    sudo cp /tmp/harbor-ca.crt '${CERTS_DIR}/ca.crt'
    rm -f /tmp/harbor-ca.crt
    # 이전 HTTP insecure 설정 정리 (v1 설치 잔재)
    sudo rm -f /etc/containerd/conf.d/registry-config.toml
    sudo rm -rf /etc/containerd/certs.d/${HARBOR_NODE_IP}:30002
    echo 'cert installed, restarting containerd...'
    sudo systemctl restart containerd
    sleep 3
    sudo systemctl restart kubelet
    echo 'done'
  "
done

rm -f "${CA_CERT_TMP}"
echo "[harbor-install] CA cert distributed to all nodes."

# ── 6. 클러스터가 안정화될 때까지 대기 ────────────────────────────────────────

echo "[harbor-install] waiting for nodes to become Ready..."
sleep 15
kubectl wait nodes --all --for=condition=Ready --timeout=120s

# ── 7. Harbor API 연결 확인 ───────────────────────────────────────────────────

echo "[harbor-install] verifying Harbor API..."
if curl -sf -k "${HARBOR_EXTERNAL_URL}/api/v2.0/systeminfo" \
    -u "admin:${HARBOR_ADMIN_PASSWORD}" > /dev/null; then
  echo "[harbor-install] Harbor API OK."
else
  echo "[harbor-install] WARNING: Harbor API not responding. Check pods."
fi

# ── 8. GHCR proxy cache 설정 ─────────────────────────────────────────────────

HARBOR_API="${HARBOR_EXTERNAL_URL}/api/v2.0"

echo "[harbor-install] configuring GHCR proxy cache endpoint..."
HTTP_CODE=$(curl -sk -o /dev/null -w "%{http_code}" \
  -X POST "${HARBOR_API}/registries" \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  -H 'Content-Type: application/json' \
  -d '{"name":"ghcr.io","type":"docker-registry","url":"https://ghcr.io","insecure":false}')

if [[ "${HTTP_CODE}" == "201" ]]; then
  echo "[harbor-install] ghcr.io endpoint created."
elif [[ "${HTTP_CODE}" == "409" ]]; then
  echo "[harbor-install] ghcr.io endpoint already exists, skipping."
else
  echo "[harbor-install] WARNING: unexpected HTTP ${HTTP_CODE} creating ghcr.io endpoint."
fi

REGISTRY_ID=$(curl -sk "${HARBOR_API}/registries?name=ghcr.io" \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d[0]["id"])' 2>/dev/null || echo "1")

echo "[harbor-install] creating ghcr-io proxy cache project (registry_id=${REGISTRY_ID})..."
HTTP_CODE=$(curl -sk -o /dev/null -w "%{http_code}" \
  -X POST "${HARBOR_API}/projects" \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  -H 'Content-Type: application/json' \
  -d "{\"project_name\":\"ghcr-io\",\"public\":true,\"registry_id\":${REGISTRY_ID},\"metadata\":{\"proxy_speed_kb\":\"-1\"}}")

if [[ "${HTTP_CODE}" == "201" ]]; then
  echo "[harbor-install] ghcr-io proxy project created."
elif [[ "${HTTP_CODE}" == "409" ]]; then
  echo "[harbor-install] ghcr-io project already exists, skipping."
else
  echo "[harbor-install] WARNING: unexpected HTTP ${HTTP_CODE} creating ghcr-io project."
fi

# ── 완료 ──────────────────────────────────────────────────────────────────────

echo ""
echo "[harbor-install] ✓ Done."
echo "[harbor-install]   UI:         ${HARBOR_EXTERNAL_URL}  (admin / ${HARBOR_ADMIN_PASSWORD})"
echo "[harbor-install]   proxy pull: ${HARBOR_NODE_IP}:${HARBOR_NODEPORT_HTTPS}/ghcr-io/<image>:<tag>"
echo "[harbor-install]   example:    kubectl run test --image=${HARBOR_NODE_IP}:${HARBOR_NODEPORT_HTTPS}/ghcr-io/kube-vip/kube-vip:v0.8.9"
