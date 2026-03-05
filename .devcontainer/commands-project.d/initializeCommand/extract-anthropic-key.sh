#!/usr/bin/env bash

# On macos, extracts host anthropic key from Keychain and places in
# .devcontainer/.env for use by docker-compose.yml, and thus being set in
# environment.  Not neccessary (?) on other platforms as rthe key will be in the
# ~/.claude settings directory

set -euo pipefail

env_file="${DEVCONTAINER_DIR}/.env"

if test -x /usr/bin/security && (! test -f ${env_file} || ! grep -q ANTHROPIC_API_KEY ${env_file}); then
    echo "Extracting anthropic key from macos keychain"
    api_key=$(security find-generic-password -s "Claude Code" -w)
    echo ANTHROPIC_API_KEY=${api_key} >> ${env_file}
fi
