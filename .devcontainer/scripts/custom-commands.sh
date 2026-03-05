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

# Make these variables available for use by hook scripts
export DEVCONTAINER_DIR=$(cd $(dirname $0)/..; pwd)
export WORKSPACE_DIR=$(cd $(dirname $0)/../..; pwd)

if [ -z "$ACTION" ]; then
    echo "Usage: $0 <action> [hook-name]"
    echo "  action: init or run"
    echo "  hook-name: required for 'run' action (postCreateCommand, postAttachCommand, etc.)"
    exit 1
fi

# Run scripts in a hook directory with ordered + parallel execution:
# - Digit-prefixed scripts ([0-9]*) run sequentially in alphabetical order
# - Non-digit-prefixed scripts run in parallel after all sequential scripts complete
# - Fails if any script fails
run_hook_scripts() {
    local hook_dir="$1"

    if [ ! -d "$hook_dir" ]; then
        return 0
    fi

    local sequential=()
    local parallel=()

    for script in "$hook_dir"/*; do
        [ -f "$script" ] || continue
        [ -x "$script" ] || continue
        local name
        name=$(basename "$script")
        if [[ "$name" == [0-9]* ]]; then
            sequential+=("$script")
        else
            parallel+=("$script")
        fi
    done

    # Run sequential scripts in order
    for script in "${sequential[@]+"${sequential[@]}"}"; do
        echo "Running lifecycle hook $(basename "$(dirname "$script")")/$(basename "$script")"
        "$script"
    done

    # Run non-digit-prefixed scripts in parallel
    if [ ${#parallel[@]} -gt 0 ]; then
        local pids=()
        for script in "${parallel[@]}"; do
            echo "Running lifecycle hook $(basename "$(dirname "$script")")/$(basename "$script") (parallel)"
            "$script" &
            pids+=($!)
        done

        local failed=0
        for pid in "${pids[@]}"; do
            if ! wait "$pid"; then
                ((failed++))
            fi
        done

        if [ "$failed" -gt 0 ]; then
            echo "Error: $failed parallel script(s) failed"
            return 1
        fi
    fi
}

case "$ACTION" in
    init)
        # Commands that run on the host during initializeCommand
        # Copy over global lifecycle command scripts if they exist
        echo "Initializing devcontainer lifecycle hooks"
        test -d ~/.config/devcontainer && cp -nrp ~/.config/devcontainer/* ${DEVCONTAINER_DIR}/ || true

        # Run initializeCommand hooks: committed project scripts first, then user scripts
        run_hook_scripts "${DEVCONTAINER_DIR}/commands-project.d/initializeCommand"
        run_hook_scripts "${DEVCONTAINER_DIR}/commands.d/initializeCommand"
        ;;

    run)
        # Commands that run inside the container
        if [ -z "$HOOK_NAME" ]; then
            echo "Error: hook-name is required for 'run' action"
            exit 1
        fi

        # Known devcontainer lifecycle hooks
        KNOWN_HOOKS="initializeCommand onCreateCommand updateContentCommand postCreateCommand postStartCommand postAttachCommand"
        if ! echo "$KNOWN_HOOKS" | grep -qw "${HOOK_NAME}"; then
            echo "Warning: '$HOOK_NAME' is not a known devcontainer lifecycle hook"
            echo "Known hooks: $KNOWN_HOOKS"
        fi

        # Run hooks: committed project scripts first, then user scripts
        run_hook_scripts "${DEVCONTAINER_DIR}/commands-project.d/${HOOK_NAME}"
        run_hook_scripts "${DEVCONTAINER_DIR}/commands.d/${HOOK_NAME}"
        ;;

    *)
        echo "Unknown action: $ACTION"
        echo "Valid actions: init, run"
        exit 1
        ;;
esac
