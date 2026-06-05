#!/usr/bin/env bash
set -euo pipefail

echo "== local-path-storage =="
kubectl -n local-path-storage get deployment local-path-provisioner >/dev/null 2>&1 || {
  echo "local-path-storage not installed" >&2
  exit 1
}
kubectl -n local-path-storage get deployment local-path-provisioner
