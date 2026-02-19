#!/usr/bin/env bash

set -euo pipefail
workspace_dir=$(cd $(dirname $0)/../..; pwd)

# Use a symlink instead of a docker mount to avoid errors when one reverts git
# changes (unlinking files breaks the mount)
mkdir -p ~/.config/mise
ln -sf ${workspace_dir}/.devcontainer/config/mise-global.toml ~/.config/mise/config.toml

export MISE_YES=1
mise trust ~/.config/mise/config.toml
mise trust ${workspace_dir}
mise install
