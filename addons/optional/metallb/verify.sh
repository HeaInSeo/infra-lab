#!/usr/bin/env bash
set -euo pipefail

echo "== metallb =="
kubectl -n metallb-system get deployment controller >/dev/null 2>&1 || {
  echo "metallb not installed" >&2
  exit 1
}
kubectl -n metallb-system get deployment controller
