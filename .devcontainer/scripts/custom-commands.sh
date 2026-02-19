#!/usr/bin/env bash

set -euo pipefail

# Usage: custom-commands.sh <action> [hook-name]
# action: init or run
# hook-name: required for 'run' action (postCreateCommand, postAttachCommand, etc.)
#
# This script handles running custom commands for devcontainer lifecycle hooks.
# - init: Runs during initialization (on host)
# - run: Runs inside the container

ACTION="${1:-}"
HOOK_NAME="${2:-}"

devcontainer_dir=$(cd $(dirname $0)/..; pwd)

if [ -z "$ACTION" ]; then
    echo "Usage: $0 <action> [hook-name]"
    echo "  action: init or run"
    echo "  hook-name: required for 'run' action (postCreateCommand, postAttachCommand, etc.)"
    exit 1
fi

case "$ACTION" in
    init)
        # Commands that run on the host during initialization
        # Copy over global lifecycle command scripts if they exist
        echo "Initializing devcontainer lifecycle hooks"
        test -d ~/.config/devcontainer && cp -nrp ~/.config/devcontainer/* ${devcontainer_dir}/ || true
        ;;

    run)
        # Commands that run inside the container
        if [ -z "$HOOK_NAME" ]; then
            echo "Error: hook-name is required for 'run' action"
            exit 1
        fi

        # Known devcontainer lifecycle hooks
        KNOWN_HOOKS="onCreateCommand updateContentCommand postCreateCommand postStartCommand postAttachCommand"
        if ! echo "$KNOWN_HOOKS" | grep -qw "${HOOK_NAME}"; then
            echo "Warning: '$HOOK_NAME' is not a known devcontainer lifecycle hook"
            echo "Known hooks: $KNOWN_HOOKS"
        fi

        # Run local lifecycle command scripts if any exist
        test -d ${devcontainer_dir}/commands.d/${HOOK_NAME}/ && \
            for customCmd in ${devcontainer_dir}/commands.d/${HOOK_NAME}/*; do
                echo "Running lifecycle hook ${HOOK_NAME}/$(basename $customCmd)"
                $customCmd
            done || true
        ;;

    *)
        echo "Unknown action: $ACTION"
        echo "Valid actions: init, run"
        exit 1
        ;;
esac
