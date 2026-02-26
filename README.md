# Clawbake

Manages multiple [OpenClaw](https://github.com/openclaw) instances in Kubernetes using a CRD+Operator pattern.

## How it works

A **web server** provides a dashboard where users log in via Google OIDC, create instances, and manage them. When a user creates an instance, the server writes a `ClawInstance` custom resource to Kubernetes. A **controller/operator** watches these CRDs and reconciles the actual cluster state — creating namespaces, deployments, services, PVCs, and gateway config for each instance.

```
User → Web UI (OIDC login) → ClawInstance CRD → Operator → K8s resources
              ↕                                              ↕
          PostgreSQL                                   OpenClaw pods
        (users, defaults)                          (per-user namespaces)
```

## Quick start

### Prerequisites

- Go 1.26+
- Docker
- [k3d](https://k3d.io) (local Kubernetes)
- [mise](https://mise.jdx.dev) (tool version manager)

### Local development

See [docs/development.md](docs/development.md) for full local development setup instructions.

### Build

```bash
make build          # Build server and operator binaries
make docker-build   # Build Docker images
make generate       # Regenerate sqlc, CRDs, and templ code
make test-unit      # Run unit tests
```

## Architecture

| Component | Description |
|-----------|-------------|
| `cmd/server` | Echo web server — dashboard, REST API, OIDC auth, Slack bot, reverse proxy to instances |
| `cmd/operator` | controller-runtime operator — reconciles `ClawInstance` CRDs into K8s resources |
| `charts/clawbake` | Helm chart for deploying both components |

### Key technologies

- **Go** with [Echo](https://echo.labstack.com) (web), [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) (operator)
- **PostgreSQL** for users and instance defaults ([sqlc](https://sqlc.dev) for type-safe queries)
- **[templ](https://templ.guide)** for server-rendered HTML templates
- **[htmx](https://htmx.org)** for dynamic UI updates (status polling, inline actions)
- **Helm** for Kubernetes deployment

### CRD: `ClawInstance`

```yaml
apiVersion: clawbake.io/v1alpha1
kind: ClawInstance
metadata:
  name: <user-id>
spec:
  userId: <user-id>
  image: ghcr.io/openclaw/openclaw:latest
  gatewayToken: <generated>
  gatewayConfig: <json>
  resources:
    requests: { cpu: 500m, memory: 1Gi }
    limits: { cpu: 2000m, memory: 2Gi }
  storage:
    size: 5Gi
status:
  phase: Pending | Creating | Starting | Running | Failed | Terminating
```

## Deployment

Releases are published as GitHub releases. The [release workflow](.github/workflows/release.yml) builds Docker images and a Helm chart, then pushes them to GHCR.

```bash
# Install from OCI registry
helm install clawbake oci://ghcr.io/neurometricai/charts/clawbake \
  --version 0.1.0-rc.3 \
  --namespace clawbake --create-namespace \
  --values my-values.yaml
```

### Required configuration

| Value | Description |
|-------|-------------|
| `server.baseURL` | Public URL of the web app |
| `auth.oidc.issuer` | OIDC provider URL (e.g. `https://accounts.google.com`) |
| `auth.oidc.clientId` | OIDC client ID |
| `auth.oidc.clientSecret` | OIDC client secret |
| `auth.oidc.redirectUrl` | OIDC callback URL |
| `auth.sessionSecret` | Session encryption secret |
| `ingress.host` | Ingress hostname |

See [`charts/clawbake/values.yaml`](charts/clawbake/values.yaml) for all available configuration.

## License

Proprietary.
