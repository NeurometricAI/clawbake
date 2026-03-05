#!/usr/bin/env bash

set -euo pipefail

# Uses $DEVCONTAINER_DIR and $WORKSPACE_DIR exported by custom-commands.sh

# Use a symlink instead of a docker mount to avoid errors when one reverts git
# changes (unlinking files breaks the mount)
mkdir -p ~/.config/mise
ln -sf ${DEVCONTAINER_DIR}/config/mise-global.toml ~/.config/mise/config.toml

export MISE_YES=1
mise trust ~/.config/mise/config.toml
mise trust ${WORKSPACE_DIR}
mise install
