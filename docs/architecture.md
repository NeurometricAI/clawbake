# Clawbake Architecture

## Overview

Clawbake manages multiple openclaw instances in Kubernetes. It provides a multi-user
web application for administration and a Slack bot for user interaction. Each user
gets their own isolated openclaw instance in a dedicated namespace.

## Architecture Decision: CRD + Operator

**Decision**: Use a CRD+Operator pattern for instance provisioning.

**Rationale**: The operator pattern is the Kubernetes-native approach for managing
lifecycle of complex resources. It provides:
- Declarative state management (desired vs actual)
- Automatic reconciliation (self-healing)
- Clean separation: web app creates CRDs, operator handles provisioning
- Idempotent operations
- Status reporting back to the CRD

The web application creates/updates/deletes `ClawInstance` custom resources.
The operator watches these and reconciles the actual cluster state.

## Component Architecture

```
┌─────────────────────────────────────────────────┐
│                 clawbake namespace               │
│                                                  │
│  ┌──────────────┐    ┌──────────────────────┐   │
│  │  Web App      │    │  Operator             │   │
│  │  (Go/Echo)    │    │  (controller-runtime) │   │
│  │               │    │                       │   │
│  │  - Admin UI   │    │  Watches ClawInstance  │   │
│  │  - User UI    │    │  CRDs and reconciles:  │   │
│  │  - OIDC Auth  │    │  - Namespace           │   │
│  │  - REST API   │    │  - Deployment          │   │
│  │  - Slack Bot  │    │  - PVC                 │   │
│  └──────┬───────┘    │  - Service             │   │
│         │            │  - Ingress             │   │
│         │            └──────────────────────┘   │
│         │                                        │
│  ┌──────▼───────┐                               │
│  │  PostgreSQL   │                               │
│  │  - users      │                               │
│  │  - settings   │                               │
│  └──────────────┘                               │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│            clawbake-<user> namespace             │
│                                                  │
│  ┌──────────────┐  ┌─────┐  ┌──────────────┐   │
│  │  openclaw     │  │ PVC │  │  Ingress      │   │
│  │  Deployment   │──│     │  │  <user>.claw  │   │
│  │               │  └─────┘  └──────────────┘   │
│  └──────────────┘                               │
└─────────────────────────────────────────────────┘
```

## Technology Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Language | Go 1.25 | K8s ecosystem standard, client-go, controller-runtime |
| Web Framework | Echo v4 | Lightweight, middleware-friendly, OIDC support |
| ORM | sqlc | Type-safe SQL, no magic, fast |
| Database | PostgreSQL 18 | Robust, well-supported |
| Operator | controller-runtime | Standard K8s operator library |
| CRD | kubebuilder | Scaffolding, code generation |
| Templates | templ | Type-safe Go HTML templates |
| CSS | Pico CSS | Minimal, classless CSS framework |
| Auth | coreos/go-oidc | Standard OIDC library |
| Slack | slack-go | Official Slack SDK for Go |
| Helm | Helm 3 | Deployment packaging |
| Testing | k3d + envtest | Local K8s for integration tests |

## Project Structure

```
/workspaces/clawbake/
├── cmd/
│   ├── server/           # Web application entry point
│   └── operator/         # Operator entry point
├── internal/
│   ├── auth/             # OIDC authentication
│   ├── config/           # Server configuration
│   ├── database/         # sqlc queries and models
│   ├── handler/          # HTTP route handlers
│   ├── bot/              # Slack bot
│   ├── operator/         # Operator reconciliation logic
│   └── k8s/              # Shared K8s client utilities
├── api/
│   └── v1alpha1/         # CRD type definitions
├── web/
│   ├── templates/        # templ templates
│   └── static/           # Static assets (CSS)
├── charts/
│   └── clawbake/         # Helm chart
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
├── config/
│   ├── crd/              # Generated CRD manifests
│   ├── rbac/             # RBAC manifests
│   └── samples/          # Sample ClawInstance CRs
├── db/
│   ├── migrations/       # SQL migrations
│   ├── queries/          # sqlc query files
│   └── sqlc.yaml
├── tests/
│   ├── e2e/              # End-to-end tests
│   └── integration/      # Integration tests
├── docs/                 # Documentation
├── Makefile
├── Dockerfile
├── go.mod
└── go.sum
```

## CRD: ClawInstance

```yaml
apiVersion: clawbake.io/v1alpha1
kind: ClawInstance
metadata:
  name: <user-uuid>
  namespace: clawbake
spec:
  userId: "<user-uuid>"
  image: "ghcr.io/openclaw/openclaw:latest"
  gatewayToken: "<generated>"
  gatewayConfig: '{"gateway": {...}}'
  resources:
    requests:
      cpu: "500m"
      memory: "1Gi"
    limits:
      cpu: "2000m"
      memory: "2Gi"
  storage:
    size: "5Gi"
status:
  phase: Running  # Pending, Creating, Starting, Running, Failed, Terminating
  namespace: clawbake-<user-uuid>
  conditions:
    - type: Ready
      status: "True"
      reason: ReconcileComplete
```

## Database Schema

```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    picture TEXT DEFAULT '',
    role TEXT NOT NULL DEFAULT 'user',  -- 'admin' or 'user'
    oidc_subject TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE instance_defaults (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    image TEXT NOT NULL DEFAULT 'ghcr.io/openclaw/openclaw:latest',
    cpu_request TEXT NOT NULL DEFAULT '100m',
    memory_request TEXT NOT NULL DEFAULT '1Gi',
    cpu_limit TEXT NOT NULL DEFAULT '500m',
    memory_limit TEXT NOT NULL DEFAULT '2Gi',
    storage_size TEXT NOT NULL DEFAULT '5Gi',
    ingress_domain TEXT NOT NULL DEFAULT 'claw.example.com',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Note: The `instance_defaults` table has been extended with a `gateway_config` column (migration 000004) to store per-instance gateway configuration as JSON.

