#!/usr/bin/env bash

# Initializes ~/.claude.json from the host-mounted copy.
# If an existing ~/.claude.json exists (e.g. preserved in a home volume across
# rebuilds), deep-merge it over the host version so container-side state like
# refreshed tokens is preserved while picking up host-side changes.

set -euo pipefail

HOST_FILE=~/.claude.json.host
TARGET_FILE=~/.claude.json

if [ -f "$TARGET_FILE" ]; then
    echo "Merging existing ~/.claude.json over host copy"
    jq -s '.[0] * .[1]' "$HOST_FILE" "$TARGET_FILE" > "${TARGET_FILE}.tmp"
    mv "${TARGET_FILE}.tmp" "$TARGET_FILE"
else
    echo "Copying host ~/.claude.json"
    cp "$HOST_FILE" "$TARGET_FILE"
fi

mise use $MISE_SCOPE claude-code
