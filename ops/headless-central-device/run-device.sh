#!/bin/sh
set -eu

dc_runtime_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
dc_node_path=$(cat "$dc_runtime_dir/node-path")
case "$dc_node_path" in
  /*) ;;
  *) echo 'The installed Node path must be absolute; rerun install-user.sh.' >&2; exit 1 ;;
esac
dc_node_bin_dir=${dc_node_path%/*}
PATH="$dc_node_bin_dir:$PATH"
export PATH
exec "$dc_node_path" "$dc_runtime_dir/node_modules/@wonderwhy-er/desktop-commander/dist/index.js" remote --disable-no-sleep
