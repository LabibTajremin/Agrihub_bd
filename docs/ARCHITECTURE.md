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

## Persistence
- `platform/database`: pgx pool. With `database.pool_mode=transaction` (default; required behind
  pgbouncer/Neon/Supabase poolers) server-side statement caching is disabled.
- Transactions travel in the context: `db.WithinTx(ctx, fn)`; repositories call `db.Q(ctx)` and
  automatically join the active transaction. Nested `WithinTx` = savepoint.
- Use cases depend only on `platform/tx.Manager` (no SQL types).
- Migrations: `backend/migrations/NNNNNN_<module>_<change>.{up,down}.sql`, embedded and run by
  `golang-migrate`. Each module owns its tables; no cross-module foreign keys or joins.

## Events: transactional outbox
1. A use case builds an event (`eventbus.Factory.New`) and publishes it through `outbox.Publisher`,
   which INSERTs into `outbox` **in the same transaction** as the state change.
2. `outbox.Relay.Flush` selects pending rows `FOR UPDATE SKIP LOCKED`, dispatches each to the
   in-process `eventbus.Local` inside its own savepoint, and marks it dispatched (or records
   `attempts`/`last_error`). Events stop retrying after `outbox.MaxAttempts`.
3. Delivery is at-least-once; consumers must be idempotent.
4. Flush runs after every request (both targets) and on a ticker (Docker target only).

Swapping the in-process bus for NATS/Kafka at extraction time replaces the `Dispatcher` only.

## Authentication & authorization
- **OTP**: 6 digits, 5-min TTL, argon2id-hashed at rest, max 5 attempts (failed attempts are
  committed even though the request fails), rate-limited per number and per IP.
- **Access token**: HS256 JWT (15 min) with `kid` header; claims `sub, sid, role, perms, iat, exp, jti`.
  Verification accepts any configured kid, so rotating to a new key (or RS256) is configuration.
- **Refresh token**: 256-bit random, stored as SHA-256, rotating. A session is a token family; presenting
  a used token revokes the whole family (`auth.refresh_reused`).
- **Guest**: `POST /v1/auth/guest` creates a guest account; signing in later upgrades it in place so
  its data keeps its owner.
- **RBAC**: roles `guest ⊂ farmer ⊂ field_officer ⊂ agronomist ⊂ admin`; static matrix in
  `platform/authz` (golden file `testdata/matrix.golden`). Use cases call `authn.Require(ctx, perm)`;
  ownership rules use `authz.Owned(role, subject, owner, ownPerm, anyPerm)`.

## Farm data
- **Area** is stored as integer nano-square-metres (10⁻⁹ m²): 1 decimal = 40 468 564 224, 1 bigha
  (Bangladesh standard, 33 decimals) = 1 335 462 619 392, 1 acre = 4 046 856 422 400,
  1 hectare = 10¹³. Conversion uses 128-bit intermediates (`math/bits`), so it is exact and a
  property test proves `Milli(FromMilli(x)) == x` for every unit.
- **Coordinates** are integer degrees × 10⁷.
- **Crop catalogue** is static reference data in `farm/domain/catalogue.go` (money in poisha,
  fractions in basis points). Seasons: `aman → boro → aus → aman`.
- Other modules read farm data only through `farm.Module.Field(ctx, id) FieldView` and
  `farm.Module.Crops() []CropView`; the composition root adapts them to the consumer's own port.

## Media storage
- `media/domain.Storage` port with two adapters: `repository.S3` (minio-go; AWS S3, MinIO, R2) and
  `repository.LocalDisk` (development). Both pass the same contract suite
  (`repository/contract_test.go`); S3 runs against an in-process S3 server (gofakes3).
- Clients upload directly to storage with a presigned URL; the API only verifies (`complete`).
  The local backend mimics presigned URLs with HMAC-signed `/v1/media/blob/{key}` routes.
- **Perceptual hash (dHash, 64-bit):** 9×8 box-averaged grayscale grid, one bit per horizontal
  neighbour comparison; near-duplicate photos differ by a few bits (Hamming distance). It keys the
  diagnosis result cache (§6.8).

## Diagnosis
- **State machine** (`diagnosis/domain`): an explicit legal-transition table; every other transition
  returns `scan.invalid_transition` (exhaustively table-tested).
- **Analysis** runs outside any transaction (engine latency must not hold a DB connection); the routed
  result and `diagnosis.scan.completed|failed` event are then written atomically.
- **Result cache** (§6.8): bounded LRU keyed by the image dHash — a re-scan of the same leaf skips
  the engine.
- **Offline sync** (§6.7): the client sends its append-only queue (monotonic `seq`, UUIDv7
  `idempotency_key`). Operations are applied in `seq` order; each outcome is stored in
  `diagnosis_sync_ops`, so replaying a batch returns `duplicate` and changes nothing. Client errors
  reject one operation; server errors fail the batch so the client retries it.
- **Conflicts** on annotations (note / saved) resolve **last-write-wins by server-received time**; the
  overwritten value is kept in `diagnosis_sync_audit` — nothing is silently discarded.
- The AI is reached only through `aiadapter.DiagnosisEngine` (stub by default).

## Advisory algorithms (`advisory/domain`, pure functions)
- **Scoring (§6.3):** criteria soil (texture 60% + pH 40%), water (rain + irrigation vs need, with a
  waterlogging penalty), pest (1 − risk), market, seed — each in [0,1];
  `score = 100·Σ wᵢ·criterionᵢ`. Weights come from `advisory.*` config and are validated to sum to
  1.0 at boot (`advisory.New`). Property tests assert every criterion ∈ [0,1] and score ∈ [0,100].
- **Ranking (§6.4):** bounded min-heap (`container/heap`) keeps the best N in O(n log N); ties break
  on crop code. A property test compares it with a full sort.
- **Yield & ROI (§6.6):** per-hectare catalogue values scaled to the exact field area with 128-bit
  integer math; money in poisha; `net = gross − Σcosts` (property-tested).
- **Rotation (§6.5):** seasons expand into a layered DAG (layer 0 = current crop). Kahn's topological
  sort guards against cycles (`advisory.rotation_cycle`, never loops); a greedy pass picks, per
  season, the best-scoring crop that keeps soil nitrogen ≥ 0, else the most nitrogen-restoring one
  (flagging `nitrogen_deficit`).
- Weather unavailable → climatological rainfall with `stale: true` (never an error).
- Golden files pin recommendations and rotation for a fixed field (`advisory/testdata`).
