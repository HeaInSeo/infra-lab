#!/usr/bin/env bash
# harbor-resume.sh — harbor-suspend.sh 로 중단된 Harbor를 재개한다.
set -euo pipefail

KUBECONFIG="${KUBECONFIG:-$(dirname "$0")/../../kubeconfig}"
export KUBECONFIG

NAMESPACE="harbor"

DEPLOYMENTS=(
  harbor-core
  harbor-jobservice
  harbor-nginx
  harbor-portal
  harbor-registry
)

STATEFULSETS=(
  harbor-database
  harbor-redis
)

echo "[harbor-resume] scaling up Harbor in namespace '${NAMESPACE}'"

for sts in "${STATEFULSETS[@]}"; do
  if kubectl get statefulset "${sts}" -n "${NAMESPACE}" >/dev/null 2>&1; then
    kubectl scale statefulset "${sts}" -n "${NAMESPACE}" --replicas=1
    echo "  scaled: ${sts}"
  fi
done

# DB/Redis가 Ready 될 때까지 대기
echo "[harbor-resume] waiting for database and redis..."
kubectl rollout status statefulset/harbor-database -n "${NAMESPACE}" --timeout=120s || true
kubectl rollout status statefulset/harbor-redis    -n "${NAMESPACE}" --timeout=60s  || true

for dep in "${DEPLOYMENTS[@]}"; do
  if kubectl get deployment "${dep}" -n "${NAMESPACE}" >/dev/null 2>&1; then
    kubectl scale deployment "${dep}" -n "${NAMESPACE}" --replicas=1
    echo "  scaled: ${dep}"
  fi
done

echo "[harbor-resume] waiting for Harbor to be ready..."
kubectl rollout status deployment/harbor-core     -n "${NAMESPACE}" --timeout=120s || true
kubectl rollout status deployment/harbor-portal   -n "${NAMESPACE}" --timeout=60s  || true
kubectl rollout status deployment/harbor-nginx    -n "${NAMESPACE}" --timeout=60s  || true

echo "[harbor-resume] done — Harbor available at https://harbor.lab.local"
