#!/usr/bin/env bash
set -euo pipefail

METRICS_SERVER_VERSION="${METRICS_SERVER_VERSION:-v0.7.2}"

echo "[INFO] uninstall base addon: metrics-server ${METRICS_SERVER_VERSION}"
kubectl delete -f "https://github.com/kubernetes-sigs/metrics-server/releases/download/${METRICS_SERVER_VERSION}/components.yaml" --ignore-not-found
