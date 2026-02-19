# Dev Container Configuration

This directory contains the development container configuration for this project, based on the [NeurometricAI devcontainer-template](https://github.com/NeurometricAI/devcontainer-template).

## Features

- **Docker-outside-of-Docker**: Access to the host Docker daemon from within the container
- **Claude CLI Integration**: Automatically mounts `~/.claude` and `~/.claude.json` for consistent Claude configuration
- **AWS CLI Configuration**: Initializes AWS CLI with environment-based configuration
- **Custom Lifecycle Hooks**: Extensible scripts for initialization, post-create, and post-attach commands
- **Optional PostgreSQL**: Uncomment `docker-compose-postgresql.yml` in `devcontainer.json` to enable

## Usage

Open this project in VS Code with the Dev Containers extension installed. VS Code will automatically detect this configuration and prompt you to reopen in the container.

## Customization

- **devcontainer.json**: Main configuration file
- **Dockerfile**: Container image definition
- **docker-compose.yml**: Service definitions
- **scripts/**: Lifecycle hook scripts for custom commands
- **config/**: Configuration templates (e.g., AWS CLI)

## Environment Variables

Set project-specific environment variables in `.devcontainer/.env` (create if needed). The Anthropic API key is automatically extracted from macOS Keychain during initialization.
