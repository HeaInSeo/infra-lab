#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "${SCRIPT_DIR}/lib.sh"

endpoint="${1:?endpoint required}"
local_script="${2:?local script required}"
remote_path="${3:-/home/${VM_USER}/remote.sh}"

require_runtime

if [[ ! -f "$local_script" ]]; then
  echo "local script not found: $local_script" >&2
  exit 1
fi

echo "[INFO] transfer $local_script -> ${endpoint}:${remote_path}"
vm_write_file "$endpoint" "$local_script" "$remote_path"

echo "[INFO] exec on $endpoint: sudo bash $remote_path"
vm_exec "$endpoint" "sudo chmod +x '$remote_path' && sudo VM_USER='${VM_USER}' ALLOW_SCHEDULE_ON_CP=0 bash '$remote_path'"
echo "[OK] ran $local_script on $endpoint"
