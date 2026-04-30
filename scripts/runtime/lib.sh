#!/usr/bin/env bash
set -euo pipefail

VM_RUNTIME="${VM_RUNTIME:-multipass}"
VM_USER="${VM_USER:-ubuntu}"
SSH_PRIVATE_KEY_PATH="${SSH_PRIVATE_KEY_PATH:-}"

ssh_opts() {
  local opts=(
    -o StrictHostKeyChecking=no
    -o UserKnownHostsFile=/dev/null
    -o BatchMode=yes
    -o ConnectTimeout=10
  )

  if [[ -n "$SSH_PRIVATE_KEY_PATH" ]]; then
    opts+=(-i "$SSH_PRIVATE_KEY_PATH")
  fi

  printf '%s\0' "${opts[@]}"
}

require_runtime() {
  case "$VM_RUNTIME" in
    multipass)
      command -v multipass >/dev/null 2>&1 || {
        echo "multipass not found" >&2
        exit 1
      }
      ;;
    ssh)
      command -v ssh >/dev/null 2>&1 || {
        echo "ssh not found" >&2
        exit 1
      }
      command -v scp >/dev/null 2>&1 || {
        echo "scp not found" >&2
        exit 1
      }
      ;;
    *)
      echo "unknown VM_RUNTIME: $VM_RUNTIME" >&2
      exit 1
      ;;
  esac
}

vm_exec() {
  local endpoint="$1"
  local cmd="$2"

  case "$VM_RUNTIME" in
    multipass)
      multipass exec "$endpoint" -- bash -lc "$cmd"
      ;;
    ssh)
      local opts=()
      while IFS= read -r -d '' opt; do
        opts+=("$opt")
      done < <(ssh_opts)
      ssh "${opts[@]}" "${VM_USER}@${endpoint}" "bash -lc $(printf '%q' "$cmd")"
      ;;
  esac
}

vm_read_file() {
  local endpoint="$1"
  local remote_path="$2"

  case "$VM_RUNTIME" in
    multipass)
      multipass exec "$endpoint" -- cat "$remote_path"
      ;;
    ssh)
      local opts=()
      while IFS= read -r -d '' opt; do
        opts+=("$opt")
      done < <(ssh_opts)
      ssh "${opts[@]}" "${VM_USER}@${endpoint}" "cat $(printf '%q' "$remote_path")"
      ;;
  esac
}

vm_write_file() {
  local endpoint="$1"
  local local_path="$2"
  local remote_path="$3"

  case "$VM_RUNTIME" in
    multipass)
      < "$local_path" multipass exec "$endpoint" -- sudo bash -c "cat > '$remote_path'"
      ;;
    ssh)
      local opts=()
      while IFS= read -r -d '' opt; do
        opts+=("$opt")
      done < <(ssh_opts)
      scp "${opts[@]}" "$local_path" "${VM_USER}@${endpoint}:$remote_path"
      ;;
  esac
}
