# Dev Container Configuration

This directory contains the development container configuration for this project.

## Features

- **Docker-outside-of-Docker**: Access to the host Docker daemon from within the container
- **mise Tool Management**: Project tools declared in `config/mise.toml`, symlinked to workspace root at container start
- **Custom Lifecycle Hooks**: Extensible scripts for initialization, post-create, and post-attach commands

## Usage

Open this project in VS Code with the Dev Containers extension installed. VS Code will automatically detect this configuration and prompt you to reopen in the container.

## Customization

- **devcontainer.json**: Main configuration file
- **Dockerfile**: Container image definition
- **docker-compose.yml**: Service definitions and env_file loading
- **commands-project.d/**: Committed lifecycle scripts that travel with the project (e.g., mise init)
- **commands.d/**: User-specific lifecycle scripts, gitignored, populated from `~/.config/devcontainer/` during init
- **scripts/**: Core lifecycle hook runner and utilities
- **config/**: Tool configuration (e.g., mise.toml)

## Environment Variables

Two mechanisms provide env vars to the container:

- **`.devcontainer/.env`** — Auto-loaded by docker compose for `${VAR}` interpolation in compose files. Auto-generated during init (e.g., API keys from macOS Keychain). Git-ignored.
- **`env_file`** entries in docker-compose.yml load vars into the container runtime:
  1. `.env.example` — Checked in, non-sensitive defaults
  2. `.env` — Git-ignored, local overrides. Module env vars are merged here at install time.

## Lifecycle Hooks

Scripts run via `custom-commands.sh` at devcontainer lifecycle points. Project scripts (`commands-project.d/`) run before user scripts (`commands.d/`).

Digit-prefixed scripts (`0-init-mise.sh`) run sequentially in order. Non-digit-prefixed scripts run in parallel.
