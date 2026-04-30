#!/usr/bin/env bash
set -euo pipefail

vm="${1:?vm required}"
local_script="${2:?local script required}"
VM_USER="${VM_USER:-ubuntu}"
remote_path="${3:-/home/${VM_USER}/remote.sh}"
VM_RUNTIME="multipass" VM_USER="$VM_USER" bash "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/runtime/run-remote.sh" "$vm" "$local_script" "$remote_path"
