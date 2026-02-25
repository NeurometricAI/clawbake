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
make k3d-import          # build + import images into k3d
make helm-install-local  # deploy operator into k3d

# Run server locally (port 8081)
make run-server          # uses PORT from .env.local
```

| Component | Where it runs | URL |
|-----------|---------------|-----|
| Admin webui | devcontainer (go run) | `http://localhost:8081` |
| User instances | k3d (via operator) | `http://<name>.claw.127-0-0-1.nip.io:8080` |

### Mode B: Everything in k3d

Deploy both server and operator into k3d via Helm. Closer to production but requires rebuilding images on each code change.

```bash
make k3d-create
make k3d-import       # builds + imports images into k3d
make helm-install-local
```

| Component | Where it runs | URL |
|-----------|---------------|-----|
| Admin webui | k3d | `http://clawbake.127-0-0-1.nip.io:8080` |
| User instances | k3d (via operator) | `http://<name>.claw.127-0-0-1.nip.io:8080` |

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
3. The browser hits `http://<name>.claw.127-0-0-1.nip.io:8080`
4. Port 8080 on the host is mapped to k3d's loadbalancer (cluster port 80)
5. Traefik matches the `Host` header to the correct Ingress resource

### Configuration

Instance ingress hostnames are configured via `ingress.host` in Helm values. The web app's own public URL is set via `BASE_URL` (env var) or `server.baseURL` (Helm value).

### Devcontainer networking

The devcontainer uses Docker-outside-of-Docker, so k3d containers run on the host Docker daemon. `make k3d-create` automatically connects the devcontainer to the k3d Docker network, allowing `kubectl` and direct cluster access.

From the browser (on the host machine), `localhost:8080` reaches k3d directly since k3d's port mapping is on the host.

## Environment Variables

`.env.example` is checked in with local dev defaults (nip.io domain, http scheme, port 8081). The devcontainer loads it automatically via `docker-compose.yml` `env_file`, so `make run-server` works out of the box.

To override values, create `.env.local` (gitignored). It loads after `.env.example` and wins.

Changing env file values requires a container restart (not a full rebuild).

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
| `make helm-install-local` | Helm install with `values-local.yaml` (nip.io, http) |
| `make migrate-up` | Run database migrations |
| `make generate` | Run all code generators (sqlc, controller-gen, templ) |
| `make test` | Run all tests |
| `make test-unit` | Run unit tests only |
