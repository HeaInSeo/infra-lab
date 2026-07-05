#!/usr/bin/env bash
# addons/optional/harbor/verify.sh
set -euo pipefail

HARBOR_NS="${HARBOR_NS:-harbor}"
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

echo "== harbor =="

check "harbor namespace exists" \
  "kubectl get namespace ${HARBOR_NS}"

check "harbor-core Deployment exists" \
  "kubectl -n ${HARBOR_NS} get deployment harbor-core"

check "harbor-portal Deployment exists" \
  "kubectl -n ${HARBOR_NS} get deployment harbor-portal"

check "harbor-registry Deployment exists" \
  "kubectl -n ${HARBOR_NS} get deployment harbor-registry"

check "harbor-jobservice Deployment exists" \
  "kubectl -n ${HARBOR_NS} get deployment harbor-jobservice"

check "harbor-database StatefulSet exists" \
  "kubectl -n ${HARBOR_NS} get statefulset harbor-database"

check "harbor-redis StatefulSet exists" \
  "kubectl -n ${HARBOR_NS} get statefulset harbor-redis"

check "all harbor pods Running" \
  "[[ \$(kubectl -n ${HARBOR_NS} get pods --no-headers 2>/dev/null | grep -vc ' Running ') -eq 0 ]]"

echo ""
echo "  PASS: ${PASS}  FAIL: ${FAIL}"
[[ "$FAIL" -eq 0 ]]
