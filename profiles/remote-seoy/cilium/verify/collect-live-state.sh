#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
SNAPSHOT_DIR="${ROOT_DIR}/profiles/remote-seoy/cilium/live-snapshot"
CILIUM_NS="${CILIUM_NS:-kube-system}"

mkdir -p "${SNAPSHOT_DIR}"

echo "[INFO] collecting live remote-seoy Cilium snapshot into ${SNAPSHOT_DIR}"

kubectl get nodes -o wide \
  > "${SNAPSHOT_DIR}/nodes.txt"

helm -n "${CILIUM_NS}" get values cilium -o yaml \
  > "${SNAPSHOT_DIR}/cilium-helm-values.yaml"

if cilium status --wait=false > "${SNAPSHOT_DIR}/cilium-status.txt" 2>/dev/null; then
  :
elif sudo cilium status --wait=false > "${SNAPSHOT_DIR}/cilium-status.txt" 2>/dev/null; then
  :
else
  kubectl -n "${CILIUM_NS}" exec ds/cilium -- cilium-dbg status \
    > "${SNAPSHOT_DIR}/cilium-status.txt"
fi

kubectl get gateway -A -o yaml \
  > "${SNAPSHOT_DIR}/gateway.yaml"
kubectl get httproute -A -o yaml \
  > "${SNAPSHOT_DIR}/httproutes.yaml"
kubectl get grpcroute -A -o yaml \
  > "${SNAPSHOT_DIR}/grpcroutes.yaml"
kubectl get ciliumloadbalancerippools.cilium.io -A -o yaml \
  > "${SNAPSHOT_DIR}/lb-pool.yaml"
kubectl get ciliuml2announcementpolicies.cilium.io -A -o yaml \
  > "${SNAPSHOT_DIR}/l2-announcement-policy.yaml"
kubectl get cnp,ccnp -A \
  > "${SNAPSHOT_DIR}/network-policies.txt"
kubectl get ciliumenvoyconfig -A \
  > "${SNAPSHOT_DIR}/ciliumenvoyconfigs.txt"

echo "[INFO] snapshot collection complete"
