#!/usr/bin/env bash
set -euo pipefail

LOCAL_PATH_VERSION="${LOCAL_PATH_VERSION:-v0.0.28}"

echo "[INFO] install optional addon: local-path-storage ${LOCAL_PATH_VERSION}"
kubectl apply -f "https://raw.githubusercontent.com/rancher/local-path-provisioner/${LOCAL_PATH_VERSION}/deploy/local-path-storage.yaml"
kubectl -n local-path-storage rollout status deployment/local-path-provisioner --timeout=180s || true
