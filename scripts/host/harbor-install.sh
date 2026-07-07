#!/usr/bin/env bash
# harbor-install.sh — Harbor를 현재 kubeconfig 클러스터에 설치한다.
#
# 아키텍처:
#   Client (docker login / kubectl run)
#     → Cilium Gateway API (harbor-gateway, HTTPS 443)
#     → HTTPRoute (harbor-route)
#     → Harbor ClusterIP service (harbor:80)
#
# 해결된 이슈 (설치 과정에서 발견):
#   1. HTTP 레지스트리 불가 — containerd v2.2.1 CRI path는 hosts.toml 기반
#      HTTP 레지스트리를 지원하지 않는다 (ctr --plain-http 는 작동하지만
#      kubelet → containerd CRI gRPC 경로는 무조건 HTTPS를 시도한다).
#      → TLS 활성화 + Harbor CA 인증서를 모든 노드에 배포하는 방식으로 해결.
#   2. containerd 재시작 후 kubelet cgroup 불일치 — containerd restart 이후
#      kubelet도 재시작하지 않으면 "expected cgroupsPath slice:prefix:name" 오류 발생.
#      → 이 스크립트에서는 재시작 순서를 containerd → kubelet으로 보장.
#   3. StorageClass 없음 — 기본 kubeadm 클러스터에는 SC가 없다.
#      → local-path-provisioner 자동 설치로 해결.
#   4. CA cert secret 이름 — nodePort expose 타입에서는 harbor-ingress 아닌
#      harbor-nginx secret에 ca.crt 가 저장된다.
#   5. containerd v2 certs.d/hosts.toml 무효 — IP:port 형식의 디렉터리에서
#      hosts.toml 이 동작하지 않았다 (containerd v2.2.1 버그 추정).
#      → 시스템 CA store (update-ca-certificates) 에 추가하는 방식으로 해결.
#   6. Gateway API expose — harbor-nginx를 외부 진입점으로 쓰지 않는다.
#      Cilium Gateway → HTTPRoute → Harbor ClusterIP 구조가 표준.
#      externalURL은 Gateway가 발급받은 LB IP 기반 hostname을 사용.
#
# 사전 조건:
#   - kubectl, helm이 PATH에 있어야 한다.
#   - harbor helm repo: helm repo add harbor https://helm.goharbor.io && helm repo update
#   - KUBECONFIG가 대상 클러스터를 가리켜야 한다.
#   - SSH_KEY_PATH가 노드 SSH 접근에 사용할 키 경로여야 한다.
#   - cilium-install.sh가 완료된 상태여야 한다 (Gateway API CRDs + LB IPAM pool).
#   - cert-manager 또는 manual TLS secret harbor-tls가 준비되어 있어야 한다.
#     이 스크립트는 Harbor 자체 CA를 harbor-tls Secret으로 변환하여 사용한다.
#
# 환경변수:
#   KUBECONFIG              — 대상 클러스터 kubeconfig (필수)
#   HARBOR_ADMIN_PASSWORD   — Harbor admin 패스워드 (기본: Harbor12345)
#   HARBOR_HOSTNAME         — Harbor 외부 hostname (기본: harbor.lab.local)
#   HARBOR_GATEWAY_PORT     — Gateway HTTPS 포트 (기본: 443)
#   SSH_KEY_PATH            — 노드 SSH 키 경로 (기본: ~/.ssh/id_ed25519)
#   VM_USER                 — 노드 SSH 사용자 (기본: ubuntu)
#
# 사용법:
#   KUBECONFIG=state/test-wizard-env/kubeconfig bash scripts/host/harbor-install.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

HARBOR_NAMESPACE="${HARBOR_NAMESPACE:-harbor}"
HARBOR_ADMIN_PASSWORD="${HARBOR_ADMIN_PASSWORD:?HARBOR_ADMIN_PASSWORD is required}"
HARBOR_HOSTNAME="${HARBOR_HOSTNAME:-harbor.lab.local}"
HARBOR_GATEWAY_PORT="${HARBOR_GATEWAY_PORT:-443}"
HARBOR_VALUES="${REPO_ROOT}/k8s/harbor/values.gateway.yaml"
HARBOR_GATEWAY_MANIFEST="${REPO_ROOT}/k8s/harbor/gateway.yaml"
HARBOR_ROUTE_MANIFEST="${REPO_ROOT}/k8s/harbor/httproute.yaml"
LOCAL_PATH_VERSION="v0.0.30"
SSH_KEY_PATH="${SSH_KEY_PATH:-${HOME}/.ssh/id_ed25519}"
VM_USER="${VM_USER:-ubuntu}"

HARBOR_EXTERNAL_URL="https://${HARBOR_HOSTNAME}"

echo "[harbor-install] KUBECONFIG=${KUBECONFIG:-<default>}"
echo "[harbor-install] externalURL: ${HARBOR_EXTERNAL_URL}"
echo "[harbor-install] namespace: ${HARBOR_NAMESPACE}"

# ── 1. local-path-provisioner (StorageClass) ─────────────────────────────────

if ! kubectl get storageclass local-path >/dev/null 2>&1; then
  echo "[harbor-install] installing local-path-provisioner ${LOCAL_PATH_VERSION}..."
  kubectl apply -f "https://raw.githubusercontent.com/rancher/local-path-provisioner/${LOCAL_PATH_VERSION}/deploy/local-path-storage.yaml"
  kubectl rollout status deployment/local-path-provisioner -n local-path-storage --timeout=120s
else
  echo "[harbor-install] local-path StorageClass already present, skipping."
fi
# Ensure local-path is default even when the StorageClass pre-existed (e.g. from
# a prior environment) -- otherwise charts that omit an explicit storageClassName
# leave PVCs permanently unbound.
kubectl annotate storageclass local-path storageclass.kubernetes.io/is-default-class=true --overwrite

# ── 2. harbor namespace ───────────────────────────────────────────────────────

kubectl create namespace "${HARBOR_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# ── 3. helm install (ClusterIP + TLS via Gateway) ────────────────────────────

echo "[harbor-install] installing Harbor via Helm (ClusterIP, Gateway API TLS)..."
helm upgrade --install harbor harbor/harbor \
  --namespace "${HARBOR_NAMESPACE}" \
  --values "${HARBOR_VALUES}" \
  --set externalURL="${HARBOR_EXTERNAL_URL}" \
  --set harborAdminPassword="${HARBOR_ADMIN_PASSWORD}" \
  --timeout 10m \
  --wait

echo "[harbor-install] Harbor pods ready."

# ── 4. Self-signed TLS cert 생성 → harbor-tls Secret ────────────────────────
#
# ClusterIP expose 모드에서는 Harbor가 TLS cert를 생성하지 않는다.
# Gateway TLS termination을 위해 openssl로 self-signed cert를 생성한다.
# Gateway listener: HTTPS terminate (harbor-tls Secret)
# Harbor backend: HTTP:80 (Gateway 뒤에서 plain HTTP)

CERT_DIR="${HOME}/.config/infra-lab/certs"
mkdir -p "${CERT_DIR}"
chmod 700 "${CERT_DIR}"
CA_KEY="${CERT_DIR}/harbor-ca.key"
CA_CRT_FILE="${CERT_DIR}/harbor-ca.crt"
SERVER_KEY="${CERT_DIR}/harbor-server.key"
SERVER_CSR="${CERT_DIR}/harbor-server.csr"
SERVER_CRT="${CERT_DIR}/harbor-server.crt"

echo "[harbor-install] generating self-signed TLS cert for ${HARBOR_HOSTNAME}..."
openssl req -x509 -newkey rsa:4096 -sha256 -days 365 -nodes \
  -keyout "${CA_KEY}" -out "${CA_CRT_FILE}" \
  -subj "/CN=harbor-lab-ca" \
  -extensions v3_ca 2>/dev/null

openssl req -newkey rsa:4096 -nodes \
  -keyout "${SERVER_KEY}" -out "${SERVER_CSR}" \
  -subj "/CN=${HARBOR_HOSTNAME}" 2>/dev/null

openssl x509 -req -in "${SERVER_CSR}" \
  -CA "${CA_CRT_FILE}" -CAkey "${CA_KEY}" -CAcreateserial \
  -out "${SERVER_CRT}" -days 365 -sha256 \
  -extfile <(printf 'subjectAltName=DNS:%s' "${HARBOR_HOSTNAME}") 2>/dev/null

echo "[harbor-install] TLS cert generated."

# harbor-tls Secret (Gateway용)
kubectl create secret tls harbor-tls \
  --namespace "${HARBOR_NAMESPACE}" \
  --cert="${SERVER_CRT}" \
  --key="${SERVER_KEY}" \
  --dry-run=client -o yaml | kubectl apply -f -
echo "[harbor-install] harbor-tls Secret applied."

# ── 5. Gateway + HTTPRoute 적용 ───────────────────────────────────────────────

# GatewayClass가 Accepted=True가 될 때까지 대기 (operator reconcile 시간 필요)
echo "[harbor-install] waiting for GatewayClass 'cilium' to be Accepted..."
for i in $(seq 1 24); do
  STATUS=$(kubectl get gatewayclass cilium \
    -o jsonpath='{.status.conditions[?(@.type=="Accepted")].status}' 2>/dev/null || true)
  if [[ "${STATUS}" == "True" ]]; then
    echo "[harbor-install] GatewayClass accepted."
    break
  fi
  if [[ "${i}" == "12" ]]; then
    echo "[harbor-install] restarting cilium-operator to trigger reconcile..."
    kubectl rollout restart deployment/cilium-operator -n kube-system
  fi
  echo "[harbor-install]   waiting... (${i}/24)"
  sleep 5
done

echo "[harbor-install] applying Gateway and HTTPRoute..."
kubectl apply -f "${HARBOR_GATEWAY_MANIFEST}"
kubectl apply -f "${HARBOR_ROUTE_MANIFEST}"

# Gateway가 LB IP를 할당받을 때까지 대기
echo "[harbor-install] waiting for Gateway to receive LoadBalancer IP..."
GATEWAY_IP=""
for i in $(seq 1 36); do
  GATEWAY_IP=$(kubectl get gateway harbor-gateway -n "${HARBOR_NAMESPACE}" \
    -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)
  if [[ -n "${GATEWAY_IP}" ]]; then
    echo "[harbor-install] Gateway IP: ${GATEWAY_IP}"
    break
  fi
  echo "[harbor-install]   waiting... (${i}/36)"
  sleep 5
done

if [[ -z "${GATEWAY_IP}" ]]; then
  echo "[harbor-install] WARNING: Gateway did not receive an IP within 150s."
  echo "[harbor-install] Check: kubectl get gateway harbor-gateway -n harbor"
else
  FIRST_NODE_IP=$(kubectl get nodes \
    -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
  ROUTE_DEV=$(ip route get "${FIRST_NODE_IP}" 2>/dev/null | awk '{for (i = 1; i <= NF; i++) if ($i == "dev") print $(i + 1)}' | head -1)
  if [[ -n "${ROUTE_DEV}" ]]; then
    echo "[harbor-install] updating host route for ${GATEWAY_IP}/32 via ${ROUTE_DEV}..."
    sudo ip route replace "${GATEWAY_IP}/32" dev "${ROUTE_DEV}"
  fi
  echo "[harbor-install] updating host /etc/hosts with ${HARBOR_HOSTNAME} -> ${GATEWAY_IP}..."
  sudo sed -i "/[[:space:]]${HARBOR_HOSTNAME//./\\.}$/d" /etc/hosts
  echo "${GATEWAY_IP} ${HARBOR_HOSTNAME}" | sudo tee -a /etc/hosts >/dev/null

  # ── 6. CoreDNS에 harbor.lab.local 등록 ──────────────────────────────────────
  echo "[harbor-install] updating CoreDNS with ${HARBOR_HOSTNAME} -> ${GATEWAY_IP}..."
  # NodeHosts ConfigMap 패치 (기존 Corefile에 hosts 블록 추가)
  EXISTING_COREFILE=$(kubectl get configmap coredns -n kube-system \
    -o jsonpath='{.data.Corefile}')

  # 이미 harbor.lab.local 항목이 있으면 업데이트, 없으면 추가
  if echo "${EXISTING_COREFILE}" | grep -q "${HARBOR_HOSTNAME}"; then
    echo "[harbor-install] CoreDNS entry already exists, skipping."
  else
    # shellcheck disable=SC2001
    NEW_COREFILE=$(echo "${EXISTING_COREFILE}" | sed "s|ready|ready\n    hosts {\n        ${GATEWAY_IP} ${HARBOR_HOSTNAME}\n        fallthrough\n    }|")
    kubectl patch configmap coredns -n kube-system \
      --type merge \
      -p "{\"data\":{\"Corefile\":$(echo "${NEW_COREFILE}" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')}}"
    kubectl rollout restart deployment/coredns -n kube-system
    kubectl rollout status deployment/coredns -n kube-system --timeout=60s
    echo "[harbor-install] CoreDNS updated."
  fi
fi

# ── 7. Harbor CA를 모든 노드의 시스템 CA store에 배포 ─────────────────────────

SSH_OPTS=(-i "${SSH_KEY_PATH}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o BatchMode=yes -o ConnectTimeout=10)
NODE_IPS=$(kubectl get nodes \
  -o jsonpath='{.items[*].status.addresses[?(@.type=="InternalIP")].address}')

echo "[harbor-install] distributing Harbor CA cert to nodes: ${NODE_IPS}"

for NODE_IP in ${NODE_IPS}; do
  echo "[harbor-install]   -> ${NODE_IP}"
  scp "${SSH_OPTS[@]}" "${CA_CRT_FILE}" "${VM_USER}@${NODE_IP}:/tmp/harbor-ca.crt"
  ssh "${SSH_OPTS[@]}" "${VM_USER}@${NODE_IP}" "
    sudo cp /tmp/harbor-ca.crt /usr/local/share/ca-certificates/harbor-ca.crt
    sudo update-ca-certificates
    rm -f /tmp/harbor-ca.crt
    [[ -n '${GATEWAY_IP}' ]] && (grep -q '${HARBOR_HOSTNAME}' /etc/hosts || sudo sh -c 'echo \"${GATEWAY_IP} ${HARBOR_HOSTNAME}\" >> /etc/hosts') || true
    sudo systemctl restart containerd
    sleep 3
    sudo systemctl restart kubelet
    echo done
  "
done

echo "[harbor-install] CA cert distributed to all nodes."
echo "[harbor-install] Certs saved to: ${CERT_DIR}"

# ── 8. 노드 안정화 대기 ───────────────────────────────────────────────────────
# kubelet 재시작 후 API 서버가 일시 불응할 수 있으므로 재연결될 때까지 대기

echo "[harbor-install] waiting for API server to recover after kubelet restart..."
for i in $(seq 1 24); do
  if kubectl get nodes >/dev/null 2>&1; then
    break
  fi
  echo "[harbor-install]   API server not ready yet... (${i}/24)"
  sleep 5
done

echo "[harbor-install] waiting for nodes to become Ready..."
kubectl wait nodes --all --for=condition=Ready --timeout=120s

# ── 9. GHCR proxy cache 설정 ─────────────────────────────────────────────────

HARBOR_API="${HARBOR_EXTERNAL_URL}/api/v2.0"

# core/nginx가 막 재시작된 직후라 API가 아직 응답하지 않을 수 있다. curl 실패를
# set -e가 아무 설명 없이 삼켜서 스크립트가 조용히 죽는 것을 방지한다.
echo "[harbor-install] waiting for Harbor API to become ready..."
for i in $(seq 1 24); do
  READY_CODE=$(curl -sk -o /dev/null -w "%{http_code}" "${HARBOR_API}/systeminfo" || echo "000")
  [[ "${READY_CODE}" == "200" ]] && break
  echo "[harbor-install]   Harbor API not ready yet (http=${READY_CODE})... (${i}/24)"
  sleep 5
done

echo "[harbor-install] configuring GHCR proxy cache endpoint..."
HTTP_CODE=$(curl -sk -o /dev/null -w "%{http_code}" \
  -X POST "${HARBOR_API}/registries" \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  -H 'Content-Type: application/json' \
  -d '{"name":"ghcr.io","type":"docker-registry","url":"https://ghcr.io","insecure":false}' || echo "000")

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
  -d "{\"project_name\":\"ghcr-io\",\"public\":true,\"registry_id\":${REGISTRY_ID},\"metadata\":{\"proxy_speed_kb\":\"-1\"}}" || echo "000")

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
echo "[harbor-install]   UI:         ${HARBOR_EXTERNAL_URL}"
echo "[harbor-install]   CA cert:    ${CERT_DIR}/harbor-ca.crt"
if [[ -n "${GATEWAY_IP:-}" ]]; then
  echo "[harbor-install]   Gateway IP: ${GATEWAY_IP}"
  echo "[harbor-install]   DNS entry:  ${HARBOR_HOSTNAME} -> ${GATEWAY_IP}"
  echo "[harbor-install]   host /etc/hosts: sudo sh -c 'echo \"${GATEWAY_IP} ${HARBOR_HOSTNAME}\" >> /etc/hosts'"
fi
echo "[harbor-install]   proxy pull: ${HARBOR_HOSTNAME}/ghcr-io/<image>:<tag>"
echo "[harbor-install]   example:    kubectl run test --image=${HARBOR_HOSTNAME}/ghcr-io/kube-vip/kube-vip:v0.8.9"
