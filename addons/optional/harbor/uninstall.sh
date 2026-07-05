#!/usr/bin/env bash
# addons/optional/harbor/uninstall.sh
set -euo pipefail

HARBOR_NS="${HARBOR_NS:-harbor}"

echo "[INFO] uninstall optional addon: harbor"

helm uninstall harbor --namespace "${HARBOR_NS}" --ignore-not-found || true

echo "[INFO] deleting PVCs in namespace ${HARBOR_NS}"
kubectl -n "${HARBOR_NS}" delete pvc --all --ignore-not-found || true

kubectl delete namespace "${HARBOR_NS}" --ignore-not-found || true

echo "[INFO] harbor uninstall complete"
