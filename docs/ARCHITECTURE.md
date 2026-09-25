# Architecture

AgriSmart is a Go **modular monolith** (API-only) plus a Flutter mobile app.

## Backend layout
```
backend/
  cmd/{api,migrate,seed}      entrypoints (thin mains)
  api/index.go                Vercel entrypoint — same handler tree
  internal/platform/          shared kernel, no domain logic
  internal/modules/<module>/  one future microservice each
    domain/      entities, value objects, errors, ports (stdlib + platform/errs only)
    usecase/     one interactor per operation; authorization lives here
    repository/  Postgres adapters implementing domain ports
    transport/   HTTP handlers + DTOs
    module.go    New(Deps) (*Module, error) and Routes() []httpx.Route
  internal/app/               composition root (the only place modules meet)
```

## Platform kernel
| Package | Responsibility |
|---|---|
| `config` | typed config; defaults → YAML → `AGRI_*` env → `-set` flags; validated at boot |
| `errs` | `*errs.Error{Code, Kind, Message(i18n key), Fields}` result-object errors |
| `httpx` | router, middleware chain, JSON/error writers, request decoding |
| `logger` | slog JSON logger, request-ID propagation, `HashPII` |
| `validator` | struct-tag validation → field-by-field `errs` validation error |
| `clock` / `idgen` | injected time and UUIDv7 generation (deterministic fakes for tests) |
| `cache` | generic bounded LRU + TTL `Store` (Redis / memory) |
| `ratelimit` | token bucket (Redis Lua / in-memory LRU), fallback decorator |
| `metrics` | request metrics recorder (in-process registry, Null Object) |
| `archtest` | architecture laws, enforced by `make arch-test` |

## Middleware chain
`RequestID → RealIP → Recoverer → Logger → CORS` (global) then, per route,
`RateLimit → Timeout → Authenticate → Authorize → handler`.
Errors returned by handlers are mapped by `httpx.WriteError` — the only Kind→status table.

## Architecture laws (enforced by `internal/platform/archtest`)
| Rule | Meaning |
|---|---|
| domain-purity | `domain` imports only stdlib and `platform/errs` |
| module-isolation | a module never imports another module (the `aiadapter` port package excepted) |
| platform-pure | `platform` never imports modules or the app |
| no-sql-usecase | use cases never import pgx or `platform/database` |
| transport-repo | transport never imports repository |
| no-time-now | `time.Now` only inside `platform/clock` |
| no-init / no-panic | no `init()` anywhere; no `panic` in modules |

`TestArchitecture_PlantedViolationsAreCaught` runs the checker over a fixture that breaks every rule.
