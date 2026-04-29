#!/usr/bin/env bash
set -euo pipefail

METRICS_SERVER_VERSION="${METRICS_SERVER_VERSION:-v0.7.2}"

echo "[INFO] install base addon: metrics-server ${METRICS_SERVER_VERSION}"
kubectl apply -f "https://github.com/kubernetes-sigs/metrics-server/releases/download/${METRICS_SERVER_VERSION}/components.yaml"
kubectl -n kube-system rollout status deployment/metrics-server --timeout=180s || true
