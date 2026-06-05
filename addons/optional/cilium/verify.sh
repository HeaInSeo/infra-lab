#!/usr/bin/env bash
# addons/optional/cilium/verify.sh
set -euo pipefail

CILIUM_NS="${CILIUM_NS:-kube-system}"
PASS=0
FAIL=0

check() {
  local label="$1"; shift
  # shellcheck disable=SC2294
  if eval "$@" >/dev/null 2>&1; then
    echo "  [OK]  ${label}"
    PASS=$((PASS + 1))
  else
    echo "  [NG]  ${label}"
    FAIL=$((FAIL + 1))
  fi
}

echo "== cilium =="

check "cilium DaemonSet exists" \
  "kubectl -n ${CILIUM_NS} get ds cilium"

check "cilium-operator Deployment exists" \
  "kubectl -n ${CILIUM_NS} get deployment cilium-operator"

check "all cilium pods Running" \
  "[[ \$(kubectl -n ${CILIUM_NS} get pods -l k8s-app=cilium -o jsonpath='{range .items[*]}{.status.phase}{\"\\n\"}{end}' | grep -vc '^Running$' || true) -eq 0 ]]"

check "Gateway API GatewayClass cilium registered" \
  "kubectl get gatewayclass cilium"

check "CiliumLoadBalancerIPPool exists" \
  "[[ \$(kubectl get ciliumloadbalancerippools.cilium.io --no-headers 2>/dev/null | wc -l) -ge 1 ]]"

check "CiliumL2AnnouncementPolicy exists" \
  "[[ \$(kubectl get ciliuml2announcementpolicies.cilium.io --no-headers 2>/dev/null | wc -l) -ge 1 ]]"

echo ""
echo "  PASS: ${PASS}  FAIL: ${FAIL}"
[[ "$FAIL" -eq 0 ]]
