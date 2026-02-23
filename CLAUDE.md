# Clawbake

System for managing multiple openclaw instances in Kubernetes.

## Architecture

- **CRD+Operator pattern**: Web app creates `ClawInstance` CRDs, operator reconciles cluster state
- **Go monolith**: Single repo with two binaries (`cmd/server`, `cmd/operator`)
- **PostgreSQL**: User management and settings
- **OIDC**: Google login authentication

## Project Structure

| Directory | Purpose |
|-----------|---------|
| `cmd/server/` | Web application entry point |
| `cmd/operator/` | K8s operator entry point |
| `internal/auth/` | OIDC authentication |
| `internal/database/` | sqlc-generated DB code |
| `internal/handler/` | HTTP route handlers |
| `internal/bot/` | Slack bot |
| `internal/operator/` | Operator reconciler |
| `api/v1alpha1/` | CRD type definitions |
| `web/templates/` | templ HTML templates |
| `charts/clawbake/` | Helm chart |
| `db/migrations/` | SQL migrations |
| `db/queries/` | sqlc query definitions |
| `config/crd/` | Generated CRD manifests |

## Commands

```bash
make build              # Build both binaries
make test               # Run all tests
make test-unit          # Unit tests only
make run-server         # Run web app
make run-operator       # Run operator
make generate           # Generate sqlc, CRD manifests, templ
make migrate-up         # Run database migrations
make k3d-create         # Create local K8s cluster
make helm-install       # Install via Helm
make docker-build       # Build Docker image
```

## Code Generation

Three code generators are used:
- **sqlc**: SQL queries → Go code (`db/queries/*.sql` → `internal/database/`)
- **controller-gen**: Go types → CRD manifests (`api/v1alpha1/` → `config/crd/`)
- **templ**: Templates → Go code (`web/templates/*.templ` → `web/templates/*_templ.go`)

Run `make generate` after modifying any source files for these generators.

## Environment Variables

See `.env.example` for all configuration. Key variables:
- `DATABASE_URL`: PostgreSQL connection string
- `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`: Google OIDC
- `SLACK_BOT_TOKEN`, `SLACK_SIGNING_SECRET`: Slack bot
- `INGRESS_DOMAIN`: Base domain for user instance ingresses

## Development Environment

- Devcontainer with Docker-outside-of-Docker (DooD)
- Automatic port mapping is disabled
- Port 8080: k3d cluster ingress load balancer (mapped via k3d's `--port "8080:80@loadbalancer"`, NOT via VS Code forwardPorts — VS Code forwarding would intercept the connection)
- Port 8081: running the server directly in the container (mapped via VS Code forwardPorts)

## Development Conventions

- Prefer refactoring over backwards compatibility
- Use latest versions of third-party packages
- Delete unused code, no deprecation layers
- Run `make generate` after changing CRD types, SQL queries, or templates
- Write tests for all business logic
- Use `mise` to install dev tools (see `mise.toml`)
- mise is already activated for bash in ~/.bashrc, so no need to activate for each command

## Database

PostgreSQL runs as a devcontainer sidecar. Connection: `postgresql://postgres:postgres@db:5432/clawbake`

Migrations use golang-migrate. Create new: `make migrate-create`

## Testing

- Unit tests: `make test-unit` (uses envtest for operator tests)
- Integration tests: `make test-integration` (requires k3d cluster)
- E2E tests: `make test-e2e` (requires k3d cluster + full deploy)
