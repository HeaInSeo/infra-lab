#!/usr/bin/env bash
set -euo pipefail

LOCAL_PATH_VERSION="${LOCAL_PATH_VERSION:-v0.0.28}"

echo "[INFO] uninstall optional addon: local-path-storage ${LOCAL_PATH_VERSION}"
kubectl delete -f "https://raw.githubusercontent.com/rancher/local-path-provisioner/${LOCAL_PATH_VERSION}/deploy/local-path-storage.yaml" --ignore-not-found
