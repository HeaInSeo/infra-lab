#!/usr/bin/env bash
# Read-only Harbor Gateway reachability diagnosis.
#
# This intentionally checks both the current HTTPS endpoint and the old
# HTTP/nip.io endpoint because confusing those two has caused false routing
# diagnoses on seoy.
set -euo pipefail

HARBOR_HOSTNAME="${HARBOR_HOSTNAME:-harbor.lab.local}"
HARBOR_NAMESPACE="${HARBOR_NAMESPACE:-harbor}"
HARBOR_GATEWAY_NAME="${HARBOR_GATEWAY_NAME:-harbor-gateway}"
HARBOR_HTTPS_PORT="${HARBOR_HTTPS_PORT:-443}"
LEGACY_HARBOR_HOSTNAME="${LEGACY_HARBOR_HOSTNAME:-}"
CONNECT_TIMEOUT="${CONNECT_TIMEOUT:-5}"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[harbor-gateway-diagnose] missing command: $1" >&2
    exit 1
  }
}

section() {
  printf '\n== %s ==\n' "$1"
}

need_cmd kubectl
need_cmd curl
need_cmd ip

section "gateway"
kubectl get gateway "${HARBOR_GATEWAY_NAME}" -n "${HARBOR_NAMESPACE}" -o wide

GATEWAY_IP="$(kubectl get gateway "${HARBOR_GATEWAY_NAME}" -n "${HARBOR_NAMESPACE}" \
  -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)"

if [[ -z "${GATEWAY_IP}" ]]; then
  echo "[harbor-gateway-diagnose] gateway has no assigned address"
  exit 0
fi

echo "[harbor-gateway-diagnose] gateway IP: ${GATEWAY_IP}"

section "listener and route objects"
kubectl get httproute -A -o wide | grep -E "NAMESPACE|${HARBOR_NAMESPACE}|harbor" || true
kubectl get service -n "${HARBOR_NAMESPACE}" -o wide

section "host route"
ip route get "${GATEWAY_IP}" || true

section "current Harbor endpoint"
echo "[harbor-gateway-diagnose] expected registry endpoint: https://${HARBOR_HOSTNAME}"
if command -v nc >/dev/null 2>&1; then
  nc -vz -w "${CONNECT_TIMEOUT}" "${GATEWAY_IP}" "${HARBOR_HTTPS_PORT}" || true
fi
curl -skS --connect-timeout "${CONNECT_TIMEOUT}" \
  --resolve "${HARBOR_HOSTNAME}:${HARBOR_HTTPS_PORT}:${GATEWAY_IP}" \
  -o /dev/null -w "https %{http_code} %{time_connect}s %{time_total}s\n" \
  "https://${HARBOR_HOSTNAME}:${HARBOR_HTTPS_PORT}/v2/" || true

section "legacy HTTP endpoint check"
if [[ -z "${LEGACY_HARBOR_HOSTNAME}" ]]; then
  LEGACY_HARBOR_HOSTNAME="harbor.${GATEWAY_IP}.nip.io"
fi
echo "[harbor-gateway-diagnose] legacy endpoint should not be used for current Harbor: http://${LEGACY_HARBOR_HOSTNAME}:80"
if command -v nc >/dev/null 2>&1; then
  nc -vz -w "${CONNECT_TIMEOUT}" "${GATEWAY_IP}" 80 || true
fi
curl -sS --connect-timeout "${CONNECT_TIMEOUT}" \
  -o /dev/null -w "http %{http_code} %{time_connect}s %{time_total}s\n" \
  "http://${LEGACY_HARBOR_HOSTNAME}/v2/" || true

section "interpretation"
cat <<EOF
- If HTTPS returns 401, Harbor Gateway routing is working and authentication is the next expected step.
- If HTTP:80 times out while HTTPS:443 works, update clients to use ${HARBOR_HOSTNAME} over HTTPS.
- Ping/ICMP failure alone is not enough to prove Cilium Gateway or L2 LB failure.
EOF
