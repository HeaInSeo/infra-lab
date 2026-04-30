#!/usr/bin/env bash
set -euo pipefail

VM_USER="${VM_USER:-ubuntu}"
VM_HOME="/home/${VM_USER}"
POD_CIDR="${POD_CIDR:-10.244.0.0/16}"

wait_for_guest_bootstrap() {
  local bin deadline

  if command -v cloud-init >/dev/null 2>&1; then
    echo "[INFO] waiting for cloud-init"
    sudo cloud-init status --wait
  fi

  deadline=$((SECONDS + 300))
  for bin in kubeadm kubectl containerd; do
    until command -v "$bin" >/dev/null 2>&1; do
      if (( SECONDS >= deadline )); then
        echo "[ERROR] timed out waiting for ${bin}" >&2
        exit 1
      fi
      sleep 5
    done
  done
}

wait_for_guest_bootstrap

MASTER_IP="$(hostname -I | awk '{print $1}')"

if [[ -f /etc/kubernetes/admin.conf ]]; then
  echo "[INFO] kubeadm already initialized; skip init"
else
  sudo kubeadm init \
    --control-plane-endpoint "${MASTER_IP}:6443" \
    --upload-certs \
    --pod-network-cidr "${POD_CIDR}"
fi

mkdir -p "$HOME/.kube"
sudo cp /etc/kubernetes/admin.conf "$HOME/.kube/config"
sudo chown "$(id -u)":"$(id -g)" "$HOME/.kube/config"

if ! kubectl -n kube-flannel get ds kube-flannel-ds >/dev/null 2>&1; then
  kubectl apply -f https://raw.githubusercontent.com/flannel-io/flannel/v0.26.0/Documentation/kube-flannel.yml
fi
kubectl -n kube-flannel rollout status ds/kube-flannel-ds --timeout=180s || true

if [[ "${ALLOW_SCHEDULE_ON_CP:-0}" == "1" ]]; then
  kubectl taint nodes --all node-role.kubernetes.io/control-plane- || true
fi

JOIN_CMD="$(sudo kubeadm token create --print-join-command)"
echo "sudo ${JOIN_CMD}" | sudo tee "${VM_HOME}/join.sh" >/dev/null
sudo chmod +x "${VM_HOME}/join.sh"

CERT_KEY="$(sudo kubeadm init phase upload-certs --upload-certs | tail -n 1)"
JOIN_CP_CMD="${JOIN_CMD} --control-plane --certificate-key ${CERT_KEY}"
echo "sudo ${JOIN_CP_CMD}" | sudo tee "${VM_HOME}/join-controlplane.sh" >/dev/null
sudo chmod +x "${VM_HOME}/join-controlplane.sh"
