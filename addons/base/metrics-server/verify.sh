#!/usr/bin/env bash
set -euo pipefail

echo "== metrics-server =="
kubectl -n kube-system get deployment metrics-server >/dev/null 2>&1 || {
  echo "metrics-server not installed" >&2
  exit 1
}

kubectl -n kube-system get deployment metrics-server
kubectl -n kube-system rollout status deployment/metrics-server --timeout=180s
