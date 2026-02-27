# Local Development Guide

This guide covers running Clawbake locally using the devcontainer.

## Prerequisites

The devcontainer provides all tools via `mise` (Go, kubectl, k3d, Helm, etc). PostgreSQL runs inside the k3d cluster and is accessible from the devcontainer via `host.docker.internal:5432`.

## Two Development Modes

### Mode A: Server locally + Operator in k3d

Run the server directly for fast iteration (hot reload via `go run`). The operator runs in k3d and reconciles CRDs the server creates.

```bash
# Start k3d cluster (operator + ingress)
make k3d-create
make helm-install-local  # Builds and imports images into k3d, deploy operator into k3d

# Run server locally (port 8081)
make generate build
make run-server          # uses PORT from .env.local
```

| Component | Where it runs | URL |
|-----------|---------------|-----|
| Admin webui | devcontainer (go run) | `http://localhost:8081` |
| User instances | k3d (via operator) | `http://localhost:8081/proxy` |

If the OIDC configuration is blank, then authentication defaults to a simple admin/user login screen to ease in local development.

### Mode B: Everything in k3d

Deploy both server and operator into k3d via Helm. Closer to production but requires rebuilding images on each code change.

```bash
make k3d-create
make helm-install-local  # Builds and imports images into k3d, deploy operator into k3d
```

| Component | Where it runs | URL |
|-----------|---------------|-----|
| Admin webui | k3d | `http://clawbake.127-0-0-1.nip.io:8080` |
| User instances | k3d (via operator) | `http://claw.127-0-0-1.nip.io:8080/proxy` |

## Port Layout

| Port | Used by | Mapped where |
|------|---------|-------------|
| 8080 (host) | k3d loadbalancer | host -> k3d cluster ingress (traefik) |
| 8081 (container) | `make run-server` | devcontainer -> host via VS Code forwarding |
| 8443 (host) | k3d loadbalancer (TLS) | host -> k3d cluster ingress |
| 5432 (host) | k3d PostgreSQL (NodePort 30432) | host -> k3d server node, devcontainer reaches via `host.docker.internal` |

The server port is set to 8081 (via `PORT` in `.env.local`) to avoid colliding with k3d's host port 8080.

VS Code is configured to forward only port 8081 explicitly. All other auto-detected ports are suppressed (`otherPortsAttributes.onAutoForward: "ignore"` in `devcontainer.json`).

## Ingress and DNS

Instance ingress uses hostname-based routing via k3d's built-in Traefik ingress controller.

### How it works

1. `INGRESS_DOMAIN=claw.127-0-0-1.nip.io` uses [nip.io](https://nip.io) for wildcard DNS
2. Any `*.127-0-0-1.nip.io` resolves to `127.0.0.1`
3. The browser hits `http://claw.127-0-0-1.nip.io:8080`
4. Port 8080 on the host is mapped to k3d's loadbalancer (cluster port 80)
5. Traefik matches the `Host` header to the correct Ingress resource

### Configuration

The ingress hostname is configured via `ingress.host` in Helm values. The web app's own public URL is set via `BASE_URL` (env var) or `server.baseURL` (Helm value).

### Devcontainer networking

The devcontainer uses Docker-outside-of-Docker, so k3d containers run on the host Docker daemon. `make k3d-create` automatically connects the devcontainer to the k3d Docker network, allowing `kubectl` and direct cluster access.

From the browser (on the host machine), `localhost:8080` reaches k3d directly since k3d's port mapping is on the host.  This lets one run the full k3d setup with an external dns provider like `ngrok`

## Environment Variables

`.env.example` is checked in with local dev defaults (nip.io domain, http scheme, port 8081). The devcontainer loads it automatically via `docker-compose.yml` `env_file`, so `make run-server` works out of the box.

To override values, create `.env.local` (gitignored). It loads after `.env.example` and wins.

Changing env file values requires a container restart (not a full rebuild) for local terminal environment to see them, however, the env files get loaded by the Makefile for tasks that need them like `run-server`

## Makefile Targets

| Target | Purpose |
|--------|---------|
| `make run-server` | Run server locally (reads env vars from shell) |
| `make run-operator` | Run operator locally (needs kubeconfig) |
| `make k3d-create` | Create k3d cluster + connect devcontainer network |
| `make k3d-delete` | Delete k3d cluster |
| `make docker-build` | Build server + operator Docker images |
| `make k3d-import` | Build images + import into k3d cluster |
| `make helm-install` | Helm install with default values |
| `make helm-install-local` | Build images, import to k3d, Helm install with local dev values |
| `make helm-restart-local` | Restart server and operator deployments in k3d |
| `make helm-clean-local` | Uninstall Helm release and clean up all resources |
| `make helm-template` | Render Helm templates without deploying |
| `make migrate-up` | Run database migrations |
| `make generate` | Run all code generators (sqlc, controller-gen, templ) |
| `make test` | Run all tests |
| `make test-unit` | Run unit tests only |

## Full End-to-End Setup (OIDC + Slack)

To test OIDC login and Slack bot locally, you need an externally reachable URL:

1. Set up a tunnel like `ngrok`. Run `ngrok http 8080` from the **host** (not the devcontainer, since k3d binds through Docker-outside-of-Docker). This generates a public URL like `https://abc-11-22-33-111.ngrok-free.app`.
2. Configure the OIDC redirect URL in `charts/clawbake/values-local.yaml` as `https://abc-11-22-33-111.ngrok-free.app/auth/callback`.
3. Copy `slack-app-manifest.yaml` and replace all `YOUR_DOMAIN` placeholders with the ngrok hostname (`abc-11-22-33-111.ngrok-free.app`). [Create a Slack app](https://api.slack.com/apps) using the edited manifest, install the bot to your workspace, then copy the bot token and signing secret into `charts/clawbake/values-local.yaml`.
4. Start k3d if not already running: `make k3d-create`
5. Deploy the app: `make helm-install-local`

To run the server directly instead of in k3d, set the same config in `.env.local` (using port `8081` instead of `8080`), run `ngrok http 8081`, run `make generate build`, then `make run-server`.
