# Development

From a clean machine to green tests in under 10 minutes (timed below).

## 1. Prerequisites (install once)
| Tool | Version | Check |
|---|---|---|
| Go | 1.26+ | `go version` |
| Docker | any recent (integration tests and E2E start Postgres) | `docker info` |
| Flutter | 3.47.5 stable | `flutter --version` |
| golangci-lint | v2.14.0 | `golangci-lint version` |
| GNU make, curl, python3 | — | used by the Makefile and scripts |

## 2. Clone and verify
```bash
git clone https://github.com/LabibTajremin/Agrihub_bd.git && cd Agrihub_bd
make verify          # lint, vet, arch laws, unit, integration, both 100% coverage gates
make e2e             # optional: Flutter client code against the real API + Postgres
```
Verified by following this page on a fresh clone (4-vCPU container, Go/pub caches warm):
`make verify` 2 min 22 s, `make e2e` 26 s. A cold module download adds a few minutes.

## 3. Run the API locally
```bash
docker run -d --name agri-pg -p 5432:5432 -e POSTGRES_USER=agri -e POSTGRES_PASSWORD=agri \
  -e POSTGRES_DB=agrismart postgres:16-alpine
cp backend/.env.example backend/.env && set -a && . backend/.env && set +a
make migrate seed
make run                          # http://localhost:8080/healthz
```
Or the whole stack (API, Postgres, Redis, MinIO): `docker compose -f deploy/docker/docker-compose.yml up --build`.

To sign in from a dev build without SMS, run the API with `AGRI_FEATURES_EXPOSE_OTP=true`
(forbidden in production): `POST /v1/auth/otp/request` then returns `dev_code`.

## 4. Run the app
```bash
cd mobile
flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080   # Android emulator → host
```
`10.0.2.2` is the Android emulator's alias for the host; use your machine's LAN IP on a phone.

## Everyday commands
| Command | What it does |
|---|---|
| `make verify` | everything CI runs: lint, vet, arch-test, unit, integration, coverage gates (Go + Flutter) |
| `make verify-backend` / `make verify-mobile` | one half only |
| `make unit` | fast Go unit tests |
| `make e2e` | cross-app E2E (Docker) |
| `make tools loadtest` | vegeta baseline on the top 5 endpoints (`RATE=`, `DURATION=`) |
| `make security` | govulncheck + flutter analyze (gosec runs in `make lint`) |
| `make docker` | build the production image |

## Configuration
All configuration is in one typed struct (`backend/internal/platform/config`). Every key is documented
in `backend/config.example.yaml`; every env var in `backend/.env.example` (tests assert both are
complete). Secrets (`AGRI_DATABASE_URL`, `AGRI_AUTH_JWT_SECRET`, …) come from the environment only.

```bash
go run ./cmd/api -config config.example.yaml -set http.addr=:9090
```

## Regenerating generated artefacts
| Artefact | Command |
|---|---|
| `backend/openapi/openapi.yaml`, error catalogue golden | `make openapi` |
| permission matrix golden | `cd backend && go test ./internal/platform/authz -update` |
| advisory golden fixtures | `cd backend && go test ./internal/modules/advisory -update` |
| mobile dictionaries (from the backend seed) | `make i18n-sync` |
| mobile goldens | `cd mobile && flutter test --update-goldens test/app/app_test.dart` |

## Where to go next
- `docs/ARCHITECTURE.md` — module map, laws, extraction guide, mobile structure
- `docs/TESTING.md` — test layers, harness, E2E, load baseline
- `docs/API.md`, `backend/openapi/openapi.yaml` — endpoints and errors
- `docs/LOCALIZATION.md`, `docs/AI_INTEGRATION.md`, `docs/DEPLOYMENT.md`, `docs/DECISIONS.md`
