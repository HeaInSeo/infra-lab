#!/usr/bin/env bash
set -euo pipefail

CILIUM_NS="${CILIUM_NS:-kube-system}"

echo "== nodes =="
kubectl get nodes -o wide

echo
echo "== helm values =="
helm -n "${CILIUM_NS}" get values cilium -o yaml

echo
echo "== cilium daemonset =="
kubectl -n "${CILIUM_NS}" get ds cilium -o wide

echo
echo "== cilium status =="
if cilium status --wait=false 2>/dev/null; then
  :
elif sudo cilium status --wait=false 2>/dev/null; then
  :
else
  kubectl -n "${CILIUM_NS}" exec ds/cilium -- cilium-dbg status
fi

echo
echo "== lb ip pool =="
kubectl get ciliumloadbalancerippools.cilium.io -A -o yaml

echo
echo "== l2 announcement policy =="
kubectl get ciliuml2announcementpolicies.cilium.io -A -o yaml

echo
echo "== network policies =="
kubectl get cnp,ccnp -A

echo
echo "== envoy configs =="
kubectl get ciliumenvoyconfig -A
