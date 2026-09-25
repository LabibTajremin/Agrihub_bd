# Development

## Prerequisites
- Go 1.26+
- Docker (integration tests start Postgres via testcontainers)
- Flutter 3.47.5 (stable)
- golangci-lint v2.14.0

## Everyday commands
| Command | What it does |
|---|---|
| `make verify` | everything CI runs: lint, vet, arch-test, unit, integration, coverage gates (Go + Flutter) |
| `make verify-backend` | Go half only |
| `make verify-mobile` | Flutter half only |
| `make unit` | fast Go unit tests |

## Configuration
All configuration is in one typed struct (`backend/internal/platform/config`). Every key is documented
in `backend/config.example.yaml`; every env var in `backend/.env.example` (tests assert both are
complete). Secrets (`AGRI_DATABASE_URL`, `AGRI_AUTH_JWT_SECRET`, …) come from the environment only.

```bash
go run ./cmd/api -config config.example.yaml -set http.addr=:9090
```

## Running locally
```bash
docker run -d --name agri-pg -p 5432:5432 -e POSTGRES_USER=agri -e POSTGRES_PASSWORD=agri -e POSTGRES_DB=agrismart postgres:16-alpine
cp backend/.env.example backend/.env && set -a && . backend/.env && set +a
make migrate seed
make run            # http://localhost:8080/healthz
```
Or the whole stack: `docker compose -f deploy/docker/docker-compose.yml up --build`.

## Regenerating generated artefacts
| Artefact | Command |
|---|---|
| `backend/openapi/openapi.yaml`, error catalogue golden | `make openapi` |
| permission matrix golden | `cd backend && go test ./internal/platform/authz -update` |
| advisory golden fixtures | `cd backend && go test ./internal/modules/advisory -update` |
| mobile dictionaries | `make i18n-sync` |
