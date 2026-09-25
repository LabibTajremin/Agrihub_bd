# Development

## Prerequisites
- Go 1.24+
- Docker (integration tests start Postgres via testcontainers)
- Flutter 3.47.5 (stable)
- golangci-lint v2.5.0

## Everyday commands
| Command | What it does |
|---|---|
| `make verify` | everything CI runs: lint, vet, arch-test, unit, integration, coverage gates (Go + Flutter) |
| `make verify-backend` | Go half only |
| `make verify-mobile` | Flutter half only |
| `make unit` | fast Go unit tests |
