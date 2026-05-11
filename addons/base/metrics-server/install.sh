#!/usr/bin/env bash
set -euo pipefail

METRICS_SERVER_VERSION="${METRICS_SERVER_VERSION:-v0.7.2}"
METRICS_SERVER_INSECURE_TLS="${METRICS_SERVER_INSECURE_TLS:-1}"

render_args_patch() {
  local args=("$@")
  local joined=""
  local arg

  for arg in "${args[@]}"; do
    if [[ -n "$joined" ]]; then
      joined+=", "
    fi
    joined+="\"${arg}\""
  done

  printf '[{"op":"replace","path":"/spec/template/spec/containers/0/args","value":[%s]}]' "$joined"
}

echo "[INFO] install base addon: metrics-server ${METRICS_SERVER_VERSION}"
kubectl apply -f "https://github.com/kubernetes-sigs/metrics-server/releases/download/${METRICS_SERVER_VERSION}/components.yaml"
mapfile -t current_args < <(kubectl -n kube-system get deployment metrics-server -o jsonpath='{range .spec.template.spec.containers[0].args[*]}{.}{"\n"}{end}')

filtered_args=()
for arg in "${current_args[@]}"; do
  if [[ "$arg" != "--kubelet-insecure-tls" ]]; then
    filtered_args+=("$arg")
  fi
done

case "$METRICS_SERVER_INSECURE_TLS" in
  1)
    echo "[INFO] enable metrics-server kubelet insecure TLS"
    filtered_args+=("--kubelet-insecure-tls")
    ;;
  0)
    echo "[INFO] disable metrics-server kubelet insecure TLS"
    ;;
  *)
    echo "METRICS_SERVER_INSECURE_TLS must be 0 or 1" >&2
    exit 1
    ;;
esac

kubectl -n kube-system patch deployment metrics-server --type='json' -p="$(render_args_patch "${filtered_args[@]}")"
kubectl -n kube-system rollout status deployment/metrics-server --timeout=180s
