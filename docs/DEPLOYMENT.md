# Deployment

One handler tree, two targets:

| Target | Entrypoint | Use |
|---|---|---|
| **Vercel** (MVP) | `backend/api/index.go` → `app.NewHandler` | serverless functions |
| **Docker** (production) | `backend/cmd/api` → `app.RunAPI` | long-running server |

Both call `app.Build`, which assembles every module into the same `httpx.Router`. No handler code is
duplicated or conditional.

## Serverless reality check (read before deploying to Vercel)
- Vercel runs Go as **serverless functions**, not a long-lived server. `internal/app/server.go` (the
  HTTP server, graceful shutdown and the outbox worker) carries `//go:build !serverless` and is
  compiled out of the Vercel build (`GO_BUILD_FLAGS=-tags serverless` in `backend/vercel.json`).
- **No in-process background workers, no long-lived DB connections, no websockets on Vercel.**
  Outbox events are relayed synchronously after each request (`App.flushAfterWrite`); on Docker a
  ticker (`database.outbox_poll_interval`) also sweeps retries.
- Postgres **must** be reached through a connection pooler (Neon / Supabase pgbouncer).
  `AGRI_DATABASE_POOL_MODE=transaction` (the default) disables server-side prepared-statement caching,
  which transaction poolers require. The integration suite runs in this mode too.
- The app is built once per cold start (`sync.Once` in `api/index.go`); keep `database.max_conns`
  small (e.g. 2–5) on Vercel.
- Media must use S3-compatible storage on Vercel (the function filesystem is ephemeral).
- The Flutter **mobile** app is not deployed to Vercel; it ships as APK/AAB from CI. Vercel may host
  the Flutter **web** build for demos.

## Vercel
See `deploy/vercel/README.md` for the project settings and environment variables. Migrations and seed
run from CI or a workstation against the direct (non-pooled) database URL:
```bash
cd backend
AGRI_DATABASE_URL=... AGRI_AUTH_JWT_SECRET=... go run ./cmd/migrate up
AGRI_DATABASE_URL=... AGRI_AUTH_JWT_SECRET=... go run ./cmd/seed
```

## Docker
```bash
make docker                                              # builds agrismart-api:local
docker compose -f deploy/docker/docker-compose.yml up    # api + postgres + redis + minio
curl localhost:8080/readyz
```
The image is multi-stage and distroless (`gcr.io/distroless/static-debian12:nonroot`), runs as a
non-root user and contains three binaries: `api` (entrypoint), `migrate`, `seed`. Configuration comes
from `AGRI_*` environment variables (optionally `-config /path/config.yaml`). The server shuts down
gracefully on SIGTERM within `http.shutdown_timeout`.

## Probes
- `GET /healthz` — liveness (process up).
- `GET /readyz` — readiness (database reachable) → 503 otherwise.
- `GET /v1/admin/metrics` — per-route request counts and latencies (`metrics:read`).
