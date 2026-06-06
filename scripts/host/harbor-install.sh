#!/usr/bin/env bash
# harbor-install.sh — Harbor를 현재 kubeconfig 클러스터에 설치한다.
#
# 사전 조건:
#   - kubectl, helm이 PATH에 있어야 한다.
#   - harbor helm repo가 추가돼 있어야 한다: helm repo add harbor https://helm.goharbor.io
#   - local-path-provisioner StorageClass가 없으면 자동 설치한다.
#   - KUBECONFIG 환경변수가 대상 클러스터를 가리켜야 한다.
#
# 사용법:
#   KUBECONFIG=state/<env>/kubeconfig bash scripts/host/harbor-install.sh
#   HARBOR_ADMIN_PASSWORD=mypass KUBECONFIG=... bash scripts/host/harbor-install.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

HARBOR_NAMESPACE="${HARBOR_NAMESPACE:-harbor}"
HARBOR_ADMIN_PASSWORD="${HARBOR_ADMIN_PASSWORD:-Harbor12345}"
HARBOR_VALUES="${REPO_ROOT}/k8s/harbor/values.nodeport.yaml"
LOCAL_PATH_VERSION="v0.0.30"

echo "[harbor-install] KUBECONFIG=${KUBECONFIG:-<default>}"
echo "[harbor-install] namespace: ${HARBOR_NAMESPACE}"

# ── 1. local-path-provisioner (StorageClass) ─────────────────────────────────

if ! kubectl get storageclass local-path >/dev/null 2>&1; then
  echo "[harbor-install] installing local-path-provisioner ${LOCAL_PATH_VERSION}..."
  kubectl apply -f "https://raw.githubusercontent.com/rancher/local-path-provisioner/${LOCAL_PATH_VERSION}/deploy/local-path-storage.yaml"
  kubectl annotate storageclass local-path storageclass.kubernetes.io/is-default-class=true --overwrite
  echo "[harbor-install] waiting for local-path-provisioner..."
  kubectl rollout status deployment/local-path-provisioner -n local-path-storage --timeout=120s
else
  echo "[harbor-install] local-path StorageClass already present, skipping."
fi

# ── 2. harbor namespace ───────────────────────────────────────────────────────

kubectl create namespace "${HARBOR_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# ── 3. helm install ───────────────────────────────────────────────────────────

echo "[harbor-install] installing Harbor 2.x via Helm..."
helm upgrade --install harbor harbor/harbor \
  --namespace "${HARBOR_NAMESPACE}" \
  --values "${HARBOR_VALUES}" \
  --set harborAdminPassword="${HARBOR_ADMIN_PASSWORD}" \
  --timeout 10m \
  --wait

echo "[harbor-install] Harbor installed."
echo "[harbor-install] UI: http://192.168.122.7:30002  (admin / ${HARBOR_ADMIN_PASSWORD})"
