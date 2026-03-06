#!/usr/bin/env bash

set -euo pipefail

# Uses $DEVCONTAINER_DIR and $WORKSPACE_DIR exported by custom-commands.sh

# Symlink the project mise config to the workspace root so `mise use <tool>`
# writes to the committed config while `mise use -g` writes to the home volume.
# Linking here prevents annoying mise trust warnings when accessing the repo
# outside of a devcontainer
ln -sf ${DEVCONTAINER_DIR}/config/mise.toml ${WORKSPACE_DIR}/.mise.toml

export MISE_YES=1
mise trust ${WORKSPACE_DIR}
mise install
