# CLAUDE.md - hypershell-api-server

REST + gRPC API microservice for the HyperShell platform. Built on the [rh-trex-ai](https://github.com/openshift-online/rh-trex-ai) framework with auto-generated Kind plugins providing CRUD, event-driven controllers, and OpenAPI client generation.

## Quick Reference

```bash
make test              # HYPERSHELL_ENV=integration_testing go test -p 1 -v ./...
make binary            # Build binary
make run               # Migrate + serve (with auth)
make run-no-auth       # Migrate + serve (no auth, dev mode)
make generate          # Regenerate OpenAPI client from specs
make proto             # Regenerate gRPC stubs from proto definitions
make db/setup          # Start PostgreSQL via Podman/Docker
make db/teardown       # Stop PostgreSQL
```

### Testing Prerequisites

- **Podman**: `systemctl --user start podman.socket`
- **DOCKER_HOST**: `export DOCKER_HOST=unix:///run/user/1000/podman/podman.sock`
- Tests use `testcontainers-go` to spin up PostgreSQL per test package
- Integration tests bind ephemeral ports (`localhost:0`) to avoid conflicts

### Lint / Format

```bash
go fmt ./...
golangci-lint run
```

## Architecture Overview

```
main.go → imports plugins (init side-effects) → registers routes, controllers, migrations
        → pkgcmd.NewServeCommand starts API server, metrics server, health check server
        → pkgcmd.NewMigrateCommand runs gormigrate migrations
```

## Domain Model

| Kind | Key Fields | Purpose |
|------|-----------|---------|
| **Fleet** | name, description, status | Top-level organizational unit |
| **Gateway** | name, fleet_id, cluster_id, release_id, database_id, namespace, tls_mode | API gateway instance |
| **GatewayNetwork** | name, fleet_id, topology, tunnel_mode, hub_gateway_id | Network connectivity mesh |
| **GatewayRelease** | name, fleet_id, image, rollout_strategy, canary_percent | Versioned gateway images |
| **ManagedCluster** | name, fleet_id, provider, region, kubeconfig_secret | Registered K8s cluster |
| **ManagedDatabase** | name, fleet_id, provider, region, engine, connection_secret | Provisioned database |

## Plugin System

Each Kind is a self-contained plugin in `plugins/{kinds}/` with uniform structure:

| File | Role |
|------|------|
| `plugin.go` | `init()` - registers service, routes, controller, presenter paths, migration |
| `model.go` | Gorm model + patch request struct |
| `handler.go` | HTTP handlers (Create, Get, List, Patch, Delete) |
| `grpc_handler.go` | gRPC handlers (Watch, Create, Get, List, Patch, Delete) |
| `grpc_presenter.go` | Protobuf ↔ model conversion |
| `service.go` | Business logic + event handlers (OnUpsert, OnDelete) |
| `dao.go` | Data access (Get, Create, Replace, Delete, FindByIDs, All) |
| `presenter.go` | OpenAPI ↔ model conversion |
| `migration.go` | Gormigrate migration with AutoMigrate |
| `mock_dao.go` | Mock DAO for unit tests |
| `*_test.go` | Integration tests + test factories |

## Upstream Framework (rh-trex-ai)

Key upstream packages consumed:
- `pkg/api` - Meta type, event types, ID generation
- `pkg/server` - API server, metrics, health check servers
- `pkg/environments` - Environment framework (dev, test, prod)
- `pkg/handlers` - HTTP handler patterns
- `pkg/services` - GenericService (List with TSL search)
- `pkg/db` - SessionFactory, advisory locks, migrations
- `pkg/cmd` - Root/Serve/Migrate cobra commands

## Environment System

Selected via `API_ENV` env var:

| Environment | API_ENV | Database | Auth | Ports |
|-------------|---------|----------|------|-------|
| Development | `development` | External PostgreSQL | Disabled | localhost:8000 |
| Integration Testing | `integration_testing` | Testcontainer PostgreSQL | Mock | Ephemeral |
| Production | `production` | External PostgreSQL | Enabled | Configured |

## API Endpoints

All routes under `/api/hypershell/v1/`:

| Method | Path | Operation |
|--------|------|-----------|
| GET | `/{kinds}` | List (supports `?search=`, `?page=`, `?size=`, `?orderBy=`, `?fields=`) |
| POST | `/{kinds}` | Create |
| GET | `/{kinds}/{id}` | Get |
| PATCH | `/{kinds}/{id}` | Patch |
| DELETE | `/{kinds}/{id}` | Delete |

Kinds: `fleets`, `gateways`, `gateway_networks`, `gateway_releases`, `managed_clusters`, `managed_databases`

## Conventions

- All Go code uses `go fmt`; `golangci-lint run` must pass
- No `panic()` in production code
- Table-driven tests with subtests
- OpenAPI client is generated - **Never** edit `pkg/api/openapi/` manually
- Plugin imports in `main.go` are side-effect imports (`_ "..."`)
- `api.Meta` provides `ID`, `CreatedAt`, `UpdatedAt`, `DeletedAt` to all models
- `BeforeCreate` gorm hook assigns `api.NewID()` (KSUID)
