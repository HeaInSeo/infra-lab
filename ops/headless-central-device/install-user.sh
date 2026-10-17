#!/bin/sh
set -eu

dc_source_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
dc_install_dir="$HOME/.local/share/desktop-commander-remote"
dc_unit_dir="$HOME/.config/systemd/user"

command -v node >/dev/null
command -v npm >/dev/null
command -v systemctl >/dev/null
node -e '
const [major, minor] = process.versions.node.split(".").map(Number);
if (major < 22 || (major === 22 && minor < 12)) {
  console.error("Node.js >=22.12.0 is required by the pinned production dependency graph.");
  process.exit(1);
}
'
[ "$(id -u)" -ne 0 ] || { echo 'Install as the server user, not root.' >&2; exit 1; }

mkdir -p "$dc_install_dir" "$dc_unit_dir"
chmod 700 "$dc_install_dir"
install -m 600 "$dc_source_dir/package.json" "$dc_install_dir/package.json"
install -m 600 "$dc_source_dir/package-lock.json" "$dc_install_dir/package-lock.json"
(cd "$dc_install_dir" && npm ci --omit=dev --no-audit --no-fund --engine-strict)
install -m 644 "$dc_source_dir/desktop-commander-remote.service" "$dc_unit_dir/desktop-commander-remote.service"
systemctl --user daemon-reload
systemd-analyze --user verify "$dc_unit_dir/desktop-commander-remote.service"
printf '%s\n' 'Installed. Start the service and complete its one-time device verification.' 'After authentication, enable it with: systemctl --user enable desktop-commander-remote.service'
