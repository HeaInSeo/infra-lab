#!/usr/bin/env bash
# addons/optional/harbor/install.sh
# Harbor 설치 (Helm, NodePort HTTP, local-path 스토리지)
#
# 선행 조건:
#   - local-path-storage addon 설치됨 (StorageClass: local-path)
#   - helm 사용 가능
#
# 환경 변수:
#   HARBOR_VERSION      Helm chart 버전 (기본: 1.16.2)
#   HARBOR_NS           네임스페이스 (기본: harbor)
#   HARBOR_NODE_PORT    HTTP NodePort (기본: 30002)
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"

HARBOR_VERSION="${HARBOR_VERSION:-1.16.2}"
HARBOR_NS="${HARBOR_NS:-harbor}"
HARBOR_NODE_PORT="${HARBOR_NODE_PORT:-30002}"

echo "[INFO] install optional addon: harbor chart ${HARBOR_VERSION}"

# ── 선행 조건: local-path StorageClass ────────────────────────────────────
if ! kubectl get storageclass local-path >/dev/null 2>&1; then
  echo "[ERROR] StorageClass 'local-path' not found." >&2
  echo "        Install the local-path-storage addon first:" >&2
  echo "        addons/manage.sh install optional local-path-storage" >&2
  exit 1
fi

# ── control-plane IP 감지 (externalURL 용) ────────────────────────────────
MASTER_IP="$(kubectl get nodes -l node-role.kubernetes.io/control-plane \
  -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')"
if [[ -z "$MASTER_IP" ]]; then
  echo "[ERROR] control-plane node IP를 감지할 수 없습니다." >&2
  exit 1
fi
HARBOR_EXTERNAL_URL="http://${MASTER_IP}:${HARBOR_NODE_PORT}"
echo "[INFO] Harbor externalURL: ${HARBOR_EXTERNAL_URL}"

# ── Helm repo ──────────────────────────────────────────────────────────────
helm repo add harbor https://helm.goharbor.io --force-update
helm repo update harbor

# ── Harbor 설치 ───────────────────────────────────────────────────────────
helm upgrade --install harbor harbor/harbor \
  --version "${HARBOR_VERSION}" \
  --namespace "${HARBOR_NS}" \
  --create-namespace \
  --values "${ROOT_DIR}/addons/values/harbor/values.yaml" \
  --set expose.nodePort.ports.http.nodePort="${HARBOR_NODE_PORT}" \
  --set externalURL="${HARBOR_EXTERNAL_URL}" \
  --wait \
  --timeout 10m

echo "[INFO] Harbor install complete"
echo "[INFO] URL  : ${HARBOR_EXTERNAL_URL}"
echo "[INFO] Login: admin / Harbor12345"
