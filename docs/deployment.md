# Deployment Guide

This guide covers building, configuring, and deploying Clawbake to a Kubernetes cluster.

## Prerequisites

- Docker
- Kubernetes cluster (k3d for local, EKS/GKE for production)
- Helm 3
- `kubectl` configured for your cluster
- PostgreSQL (or use the bundled internal instance)
- Google OAuth credentials (for OIDC login)

## Architecture Overview

Clawbake deploys two workloads plus a database:

| Component | Purpose | Port |
|-----------|---------|------|
| **Server** | Web app for managing instances | 8080 |
| **Operator** | Reconciles `ClawInstance` CRDs into Kubernetes resources | 8081 (metrics), 8082 (health) |
| **PostgreSQL** | Stores users and instance defaults | 5432 |

```
Ingress → Server (:8080) → creates ClawInstance CRDs
                                    ↓
                          Operator watches CRDs
                                    ↓
                   Creates: Namespace, Deployment, Service, PVC, Ingress
                          (per user instance)
```

## 1. Build

### Code Generation

Run code generation before building if you've changed CRD types, SQL queries, or templates:

```bash
make generate
```

This runs three generators:
- **sqlc**: `db/queries/*.sql` → `internal/database/`
- **controller-gen**: `api/v1alpha1/` → `config/crd/`, `config/rbac/`
- **templ**: `web/templates/*.templ` → `web/templates/*_templ.go`

### Docker Images

Build images for both binaries:

```bash
# Server
docker build -t ghcr.io/neurometricai/clawbake-server:0.1.0 --build-arg BINARY=server .

# Operator
docker build -t ghcr.io/neurometricai/clawbake-operator:0.1.0 --build-arg BINARY=operator .
```

The Dockerfile uses a multi-stage build:
1. **Builder**: `golang:1.26-alpine` compiles a static binary (`CGO_ENABLED=0`)
2. **Runtime**: `alpine:3.21` with only the binary, static assets, and CA certificates

For local k3d development, use `make k3d-import` to build and load images into the cluster.

## 2. Configuration

### Required Values

At minimum, configure these for a production deployment:

| Value | Description | Example |
|-------|-------------|---------|
| `auth.oidc.issuer` | OIDC provider URL | `https://accounts.google.com` |
| `auth.oidc.clientId` | OAuth client ID | `xxxxx.apps.googleusercontent.com` |
| `auth.oidc.clientSecret` | OAuth client secret | `xxxxx` |
| `auth.oidc.redirectUrl` | Callback URL | `https://clawbake.example.com/auth/callback` |
| `auth.sessionSecret` | Session encryption key | `$(openssl rand -base64 32)` |
| `ingress.host` | Server hostname | `clawbake.example.com` |
| `server.baseURL` | Public URL (defaults to `https://<ingress.host>`) | `https://clawbake.example.com` |

### Database Options

**Internal PostgreSQL** (default): The Helm chart deploys a PostgreSQL StatefulSet with a 10Gi PVC. Suitable for development and small deployments.

**External PostgreSQL**: For production, point to a managed database:

```yaml
database:
  external: true
  url: "postgresql://user:password@rds-host:5432/clawbake?sslmode=require"
  internal:
    enabled: false
```

### Full values.yaml Reference

See `charts/clawbake/values.yaml` for all configurable values. Key sections:

```yaml
server:
  replicas: 1
  image:
    repository: ghcr.io/neurometricai/clawbake-server
    tag: ""  # defaults to Chart.AppVersion
  resources:
    requests: { cpu: 100m, memory: 128Mi }
    limits: { cpu: 500m, memory: 256Mi }

operator:
  replicas: 1
  image:
    repository: ghcr.io/neurometricai/clawbake-operator
    tag: ""
  resources:
    requests: { cpu: 100m, memory: 128Mi }
    limits: { cpu: 500m, memory: 256Mi }

ingress:
  enabled: true
  className: ""
  host: clawbake.example.com
  tls: []

instanceDefaults:
  image: ghcr.io/openclaw/openclaw:latest
  cpuRequest: 500m
  cpuLimit: 2000m
  memoryRequest: 1Gi
  memoryLimit: 2Gi
  storageSize: 5Gi
  ttyd:
    enabled: true
    image: tsl0922/ttyd:alpine
    port: 7681

slack:
  enabled: false
  botToken: ""
  signingSecret: ""
```

## 3. Deploy

### Local Development (k3d)

See [docs/development.md](development.md) for the full local development guide, including devcontainer setup, port layout, and ingress DNS configuration.

Quick start for full-k3d deployment:

```bash
make k3d-create          # Create cluster + connect devcontainer network
make helm-install-local  # Builds images, and installs helm chart with local dev values (nip.io, http)
```

The k3d cluster maps `localhost:8080` → cluster port 80 and `localhost:8443` → port 443. Instance URLs use nip.io for wildcard DNS: `http://<name>.claw.127-0-0-1.nip.io:8080`.
You can also run just the server component directly with `make run-server`.  It requires the rest of the components running in k3d, and listens on port `8081`

### Production

```bash
# Install or upgrade
helm upgrade --install clawbake charts/clawbake \
  --namespace clawbake \
  --create-namespace \
  --set database.external=true \
  --set database.url="postgresql://user:pass@db-host:5432/clawbake?sslmode=require" \
  --set auth.oidc.issuer="https://accounts.google.com" \
  --set auth.oidc.clientId="YOUR_CLIENT_ID" \
  --set auth.oidc.clientSecret="YOUR_CLIENT_SECRET" \
  --set auth.oidc.redirectUrl="https://clawbake.example.com/auth/callback" \
  --set auth.sessionSecret="$(openssl rand -base64 32)" \
  --set ingress.host="clawbake.example.com" \
  --set ingress.className="nginx"
```

For sensitive values, use a values file instead of `--set`:

```bash
helm upgrade --install clawbake charts/clawbake \
  --namespace clawbake \
  --create-namespace \
  -f values-production.yaml
```

### Verify

```bash
# Check pods are running
kubectl get pods -n clawbake

# Check CRD is installed
kubectl get crd clawinstances.clawbake.io

# Check server health
kubectl port-forward -n clawbake svc/clawbake-server 8080:80
curl http://localhost:8080/healthz
```

## 4. Database Migrations

Migrations use [golang-migrate](https://github.com/golang-migrate/migrate) and live in `db/migrations/`.  They run as an init container in the server pod when deploying the helm chart to a kubernetes cluster.  When running the server directly for a tighter feedback loop, you may want to run migrations manually with:

```bash
# Apply all pending migrations
make migrate-up

# Rollback the last migration
make migrate-down

# Create a new migration
make migrate-create
```

The initial migration (`000001_init`) creates:
- **`users`**: Stores OIDC-authenticated users (email, name, role)
- **`instance_defaults`**: Singleton row with default resource limits for new ClawInstances

## 5. What Gets Deployed

The Helm chart creates these Kubernetes resources:

| Resource | Name | Purpose |
|----------|------|---------|
| Namespace | `clawbake` | All resources live here |
| CRD | `clawinstances.clawbake.io` | Custom resource definition |
| Deployment | `clawbake-server` | Web application |
| Deployment | `clawbake-operator` | CRD reconciler with leader election |
| StatefulSet | `clawbake-postgresql` | Database (if internal) |
| Service | `clawbake-server` | Routes to server pods (80→8080) |
| Service | `clawbake-postgresql` | Headless service for PostgreSQL |
| Ingress | `clawbake-server` | External access to the server |
| ServiceAccount | `clawbake-server`, `clawbake-operator` | Pod identities |
| ClusterRole | `clawbake-operator` | Operator permissions (namespaces, deployments, etc.) |
| Role | `clawbake-server` | Server permissions (ClawInstance CRUD) |
| ConfigMap | `clawbake` | Non-sensitive instance defaults |
| Secret | `clawbake` | Database URL, OIDC credentials, session secret |

### RBAC

The **operator** has cluster-wide permissions to:
- Manage namespaces, deployments, services, PVCs, and ingresses
- CRUD ClawInstance resources and their status
- Manage leases (leader election) and create events

The **server** has namespace-scoped permissions to:
- Create, read, update, delete ClawInstance resources
- Read ClawInstance status

## 6. Runtime Behavior

### Server Startup Sequence

1. Load configuration from environment variables
2. Connect to PostgreSQL
3. Initialize OIDC provider (Google)
4. Create Kubernetes client
5. Start Echo HTTP server with middleware (logger, recovery, CORS)
6. Serve on `:8080`
7. Graceful shutdown on SIGINT/SIGTERM (10s timeout)

### Operator Startup Sequence

1. Create controller-runtime manager with scheme registration
2. Register ClawInstanceReconciler
3. Enable leader election via Kubernetes leases
4. Start health probes (`:8082`)
5. Start metrics server (`:8081`)
6. Watch for ClawInstance changes and reconcile

### Reconciliation Loop

When a `ClawInstance` CRD is created or updated, the operator:
1. Creates a dedicated namespace for the instance
2. Creates a PersistentVolumeClaim for storage
3. Creates a Deployment running the openclaw image
4. Creates a Service exposing the deployment
5. Creates an Ingress for external access (if enabled)
6. Updates the CRD status with phase, URL, and conditions

## 7. Troubleshooting

```bash
# Server logs
kubectl logs -n clawbake -l app.kubernetes.io/component=server

# Operator logs
kubectl logs -n clawbake -l app.kubernetes.io/component=operator

# Check ClawInstance status
kubectl get clawinstances -n clawbake -o wide

# Describe a specific instance
kubectl describe clawinstance <name> -n clawbake

# Check operator leader election
kubectl get leases -n clawbake

# Render Helm templates without deploying
make helm-template
```
