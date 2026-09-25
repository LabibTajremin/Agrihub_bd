# AgriSmart — Autonomous Build Instruction

**Audience:** an autonomous coding agent (Claude Opus 5 or equivalent).
**Mode:** execute phases in order, without stopping, until Phase 18 is green.
**Repo:** `LabibTajremin/Agrihub_bd` · **Branch:** `claude/sleepy-mccarthy-h8hv7s` · **One PR for all phases.**

---

## 0. OPERATING CONTRACT — read this first, every session

1. **First action of every session:** read `.agent/STATE.md`. It is the single source of truth for
   what is done and what is next. Then run `git log --oneline -5` and `make verify`. Do not re-read
   files you do not need.
2. **Never stop before Phase 18 is complete.** A phase is complete only when `make verify` exits 0
   (all tests green, coverage gate met, lint clean) AND the phase commit is pushed.
3. **If you hit a usage/context limit:** update `.agent/STATE.md` with exact next action, commit and
   push it, then stop cleanly. On resume, step 1 puts you back on the rails. Never leave the tree
   dirty or the branch un-pushed.
4. **Never ask the user for permission mid-build.** Decisions are pre-made in this document. If
   something is genuinely ambiguous, pick the option that satisfies the constraints in §2, write the
   decision into `docs/DECISIONS.md` with a one-line rationale, and continue.
5. **Do not build anything in §14 (open slots).** Leave the interfaces and stubs exactly as specced.
6. **Scope discipline:** build what the phase says. No extra features, no speculative abstraction.

### Token budget rules (enforced)

- Each phase lists a **READ ALLOWLIST**. Read only those paths plus files you create. Nothing else.
- Use `rg` / `grep` to locate code. Never read a file to "have a look".
- Never re-read a file already read this session unless you changed it.
- Write complete files in one shot. Do not stream partial edits of a file you are authoring.
- Print test output only on failure. On success print the one-line summary.
- Keep `.agent/STATE.md` under 120 lines. It is a ledger, not a log.
- Do not paste generated code back into chat. The diff is the record.

---

## 1. PRODUCT

AgriSmart — an offline-first, voice-navigable agriculture app for smallholder rice farmers and
agricultural extension officers in Bangladesh. Design reference: the Figma file
`AgriSmart — Farmer App (User App) UI Kit & Flows` (58 screens, 25 components, 37 design tokens).

Five product modules: **Home/Dashboard**, **Plant Doctor** (AI diagnosis), **Crop Advisor**
(predictive planning), **Voice Assistant** (hands-free), **Settings & Offline**.

Primary loop: scan a sick leaf → on-device diagnosis → chemical or organic treatment plan → saved to
log. Everything must work with no network.

---

## 2. NON-NEGOTIABLE CONSTRAINTS

| # | Constraint |
|---|---|
| C1 | Backend in **Go**. API-only. No server-rendered HTML. |
| C2 | Frontend in **Flutter**. Mobile-first (Android primary, iOS parity). |
| C3 | **Clean architecture** — domain has zero outward dependencies. |
| C4 | **Repository pattern** — ports declared in `domain`, adapters in `repository`. |
| C5 | **Auth pipeline + RBAC**, enforced at the use-case boundary (not only at transport). |
| C6 | **One config surface** — typed, validated, documented, env-overridable. |
| C7 | **Developer documentation** kept current within the phase that changes behaviour. |
| C8 | **MVP deploys to Vercel**; production later deploys as a **Docker image**. Same handler tree. |
| C9 | **Dictionary-based i18n** — key→value per language, switched at runtime, instantly. |
| C10 | **Voice assets use the same dictionary mechanism**. The AI model itself is NOT integrated. |
| C11 | **100% test coverage**, unit + integration, backend and Flutter, gated in CI. |
| C12 | **Modular monolith** whose modules extract to microservices without rewrites. |
| C13 | One commit + push per phase, all on one branch, **one PR**, merged by the user at the end. |

### Constraint reality-check (read once, then follow the design)

Vercel runs Go as **serverless functions**, not a long-lived server. This is fine and the
architecture below already accounts for it, but the rules are:

- All HTTP handlers are assembled into a `http.Handler` tree by `internal/platform/httpx.Router`.
  `cmd/api/main.go` serves it with `net/http`; `api/index.go` exports the *same* tree to Vercel.
  Zero handler code is duplicated or conditional.
- **No in-process background workers, no long-lived DB connections, no websockets on the Vercel
  target.** Anything long-running goes behind the event bus (§5.7) and runs only on the Docker
  target. The Vercel build compiles it out via build tag `!serverless`.
- Postgres must be reached through a **connection pooler** (Neon/Supabase pgbouncer) on Vercel.
  `DATABASE_POOL_MODE=transaction` is the serverless default.
- Flutter's mobile app is **not** deployed to Vercel. Vercel hosts the API plus (optionally) the
  Flutter **web** build for demos. Mobile ships as APK/AAB artifacts from CI.

Write this into `docs/DEPLOYMENT.md` in Phase 11.

---

## 3. ARCHITECTURE

### 3.1 Layering (per module, strictly inward-pointing)

```
transport/   HTTP handlers, DTOs, request validation, OpenAPI annotations
     ↓ depends on
usecase/     application services, orchestration, authorization checks, transactions
     ↓ depends on
domain/      entities, value objects, domain errors, PORT interfaces. ZERO imports outside stdlib.
     ↑ implemented by
repository/  Postgres/Redis/S3 adapters implementing domain ports
```

**Hard rule:** `domain` imports nothing from `usecase`, `transport`, `repository`, or any third-party
package except stdlib. Enforced by an import-linter test in Phase 1.

### 3.2 Repository tree

```
agrihub/
├── .agent/STATE.md                  # build ledger (agent-maintained)
├── .github/workflows/ci.yml
├── Makefile
├── docs/
│   ├── agent/BUILD_INSTRUCTION.md   # this file
│   ├── agent/STANDING_INSTRUCTIONS.md
│   ├── ARCHITECTURE.md  API.md  DEPLOYMENT.md  DEVELOPMENT.md
│   ├── AI_INTEGRATION.md            # the open slot, documented
│   ├── LOCALIZATION.md  TESTING.md  DECISIONS.md
├── backend/
│   ├── cmd/api/main.go              # Docker/local entrypoint
│   ├── cmd/migrate/main.go
│   ├── cmd/seed/main.go
│   ├── api/index.go                 # Vercel serverless entrypoint (same router)
│   ├── internal/
│   │   ├── platform/                # shared kernel — NO domain logic
│   │   │   ├── config/  logger/  database/  httpx/  authn/  authz/
│   │   │   ├── errors/  validator/  cache/  eventbus/  idgen/  clock/
│   │   │   └── outbox/
│   │   └── modules/                 # each folder = one future microservice
│   │       ├── identity/  localization/  farm/  media/
│   │       ├── diagnosis/  advisory/  weather/  alert/
│   │       └── aiadapter/           # OPEN SLOT — interfaces + stub only
│   ├── migrations/
│   ├── test/                        # integration harness, fixtures, testcontainers
│   └── openapi/openapi.yaml
├── mobile/                          # Flutter
│   ├── lib/
│   │   ├── core/                    # di, router, theme, network, storage, l10n runtime
│   │   └── features/<feature>/{domain,data,presentation}
│   ├── assets/i18n/                 # seed dictionaries
│   └── test/ · integration_test/
└── deploy/{docker,vercel}/
```

### 3.3 Module boundary law (this is what makes microservice extraction trivial)

1. A module **never** imports another module's `domain`, `usecase`, or `repository`.
2. Cross-module reads go through a **published port**: the consumer declares the interface it needs
   in its own `domain/ports.go`; the provider module supplies an adapter in `module.go` wiring.
3. Cross-module writes go through the **event bus** (§5.7), never a direct call.
4. Each module owns its tables. **No cross-module SQL joins, no foreign keys across module
   boundaries.** Reference other modules by ID only.
5. Each module exposes exactly one public constructor:
   `func New(deps Deps) (*Module, error)` and one `func (m *Module) Routes() []httpx.Route`.

A test in Phase 1 (`internal/platform/archtest`) walks the import graph and fails the build on any
violation. This is not optional — it is the guarantee that C12 holds.

---

## 4. MODULE CATALOGUE

| Module | Owns | Key entities | Extractable as |
|---|---|---|---|
| `identity` | users, sessions, roles, OTP | User, Session, Role, Permission | auth-service |
| `localization` | dictionaries, voice manifests | Dictionary, Entry, VoiceAsset | i18n-service |
| `farm` | fields, plots, soil, crops | Field, Plot, SoilProfile, Crop | farm-service |
| `media` | images, audio blobs | MediaObject, UploadTicket | media-service |
| `diagnosis` | scans, results, treatments | Scan, Diagnosis, TreatmentPlan | diagnosis-service |
| `advisory` | recommendations, ROI, rotation | Recommendation, YieldEstimate, RotationPlan | advisory-service |
| `weather` | forecasts, microclimate | Forecast, Observation | weather-service |
| `alert` | pest/season alerts, notifications | Alert, Subscription | alert-service |
| `aiadapter` | **ports only** | — | ai-service (user builds later) |

---

## 5. CROSS-CUTTING SPECIFICATIONS

### 5.1 Configuration (C6)

Single typed struct, loaded once, validated at boot, immutable afterwards.

- `internal/platform/config/config.go` — `type Config struct` with nested sections:
  `App, HTTP, Database, Redis, Auth, Storage, Localization, AI, Observability, Features`.
- Source precedence: **defaults → `config.yaml` → env vars → flags**. Env var names are derived:
  `AGRI_DATABASE_HOST`, `AGRI_AUTH_ACCESS_TTL`, etc.
- Every field carries `default:` and `validate:` struct tags. Boot fails loudly on invalid config
  with a field-by-field report. Never `panic` mid-request because of config.
- Ship `config.example.yaml` documenting **every** key with a comment and its default. Ship
  `.env.example` mirroring it. A test asserts the example file covers 100% of struct fields — this
  keeps documentation honest automatically.
- Secrets never land in `config.yaml`. Env only.

### 5.2 Auth pipeline (C5)

**Flow:** phone number → OTP (6 digits, 5-min TTL, hashed at rest, max 5 attempts, per-number and
per-IP rate limit) → access JWT (15 min) + refresh token (30 days, rotating, reuse-detected) →
refresh rotation invalidates the whole family on replay.

- Hashing: **argon2id** (`golang.org/x/crypto/argon2`), params in config.
- JWT: HS256 for MVP, key from config; `kid` header present from day one so RS256 rotation is a
  config change, not a refactor. Claims: `sub, sid, role, perms, iat, exp, jti`.
- Guest mode: an anonymous principal with role `guest` — the app must allow a first scan with no
  account (this is a product requirement, not an oversight).

**Middleware chain, in this exact order:**
`RequestID → RealIP → Recoverer → Logger → CORS → RateLimit → Timeout → Authenticate → Authorize → handler`

### 5.3 RBAC (C5)

Roles: `guest`, `farmer`, `field_officer`, `agronomist`, `admin`.

- Permissions are strings `resource:action` (`scan:create`, `field:read`, `dictionary:write`).
- A static permission matrix lives in `internal/platform/authz/matrix.go` with a golden-file test.
- **Authorization is checked inside the use case**, taking the `Principal` from context, not in the
  handler. Transport-level checks are a fast-fail convenience only. Rationale: when a module becomes
  a microservice its use cases keep their guarantees.
- Ownership rules (`a farmer may read only their own fields`) are expressed as policy functions,
  table-tested for every role × action × ownership combination.

### 5.4 Dictionary-based i18n (C9)

**Storage:** table `dictionary_entries(lang_code, namespace, key, value, version, updated_at)`,
primary key `(lang_code, namespace, key)`.

**Key convention:** `screen.section.element` — e.g. `home.quickscan.title`,
`diagnosis.result.confidence_label`. Lowercase, dot-separated, no spaces. A lint test rejects keys
that do not match `^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`.

**Languages (7):** `bn` (Bangla, default), `en`, `hi`, `es`, `fr`, `ar`, `pt`.
`ar` is **RTL** — carry an `is_rtl` flag on the language record; the Flutter app must flip
directionality from that flag, never from a hardcoded list.

**Serving:**
- `GET /v1/i18n/languages` → list with `code, name, native_name, is_rtl, has_voice, version`.
- `GET /v1/i18n/{lang}?since={version}` → full snapshot, or delta when `since` is supplied.
  Strong `ETag` = content hash. `304` on match. `Cache-Control: public, max-age=300`.
- In-memory: one `map[string]string` per language, built once, published via
  `atomic.Pointer[Snapshot]`. Lookup is **O(1)**, allocation-free, lock-free on the read path.
  Hot-swap on update replaces the pointer — readers never block. (§6.1)

**Client (Flutter):**
- Seed JSON bundled in `assets/i18n/{lang}.json` so the very first launch is instant and offline.
- On boot and on reconnect, fetch delta, persist to local store, publish a new snapshot.
- `LocalizationRepository.watch()` exposes `Stream<LanguageSnapshot>`; the app root listens and
  rebuilds. **Changing language must not require a restart** — this is an explicit acceptance test.
- Lookup helper `context.t('home.quickscan.title')`. Missing key → returns the key itself in release
  and **fails the test suite** in debug. A CI test asserts every key used in Dart source exists in
  every one of the 7 dictionaries.

### 5.5 Voice assets (C10)

Identical mechanism, different payload. Table
`voice_assets(lang_code, key, media_id, duration_ms, checksum, version)`.

- Same key namespace as text, so `diagnosis.result.advisory` has both a string and an audio clip.
- `GET /v1/voice/{lang}?since=` returns a manifest of `key → {url, duration_ms, checksum}`.
- Client downloads clips lazily, caches by checksum, plays from cache offline.
- **This is pre-recorded audio delivery only. No TTS, no STT, no model inference.** Anything that
  needs generated speech goes through the §14 open slot.

### 5.6 Errors

- `internal/platform/errors`: typed `*Error{Code, Kind, Message, Fields, cause}` with
  `Kind ∈ {Validation, NotFound, Conflict, Unauthorized, Forbidden, RateLimited, Internal, Unavailable}`.
- Domain returns typed errors; transport maps `Kind → HTTP status` in exactly one place.
- Wire format is identical for every endpoint:
  `{"error":{"code":"scan.not_found","message":"…","fields":{…},"request_id":"…"}}`.
- `message` is an **i18n key** where the client will show it — never a raw English sentence.
- Internal errors never leak cause text to the client; they are logged with the request ID.

### 5.7 Event bus + outbox (C12 enabler)

- `internal/platform/eventbus` with interface `Publish(ctx, Event) error` and
  `Subscribe(topic, handler)`.
- MVP implementation: **in-process, synchronous, transactional outbox**. Events are written to an
  `outbox` table in the same transaction as the state change, then dispatched.
- Because publication is already at-least-once with idempotent consumers, swapping the transport for
  NATS/Kafka at extraction time is a single adapter, no call-site changes.
- Event naming: `<module>.<entity>.<past_tense_verb>` — `diagnosis.scan.completed`.
- Every event carries `id, occurred_at, actor_id, trace_id, version, payload`.

### 5.8 Observability

Structured logs (`log/slog`, JSON), request ID propagated through context and returned in
`X-Request-ID`. Metrics via `expvar` or Prometheus behind an interface. No PII in logs — phone
numbers are hashed before logging.

---

## 6. ALGORITHMS & DATA STRUCTURES (specified, not left to taste)

**6.1 Dictionary lookup** — immutable `map[string]string` snapshot behind `atomic.Pointer`.
Read O(1), zero lock, zero alloc. Rebuild-and-swap on change. Do **not** use `sync.Map` (wrong
access pattern) and do **not** use a trie (no prefix queries needed).

**6.2 Dictionary delta** — client sends `since=version`; server returns entries with
`version > since`. Version is a monotonic `bigint` per language from a sequence. O(changed) transfer
rather than O(all).

**6.3 Crop suitability scoring** — deterministic weighted multi-criteria decision analysis.
Criteria (soil match, water need vs forecast, pest pressure, market demand, seed availability) are
each normalised to `[0,1]`; weights are config-driven and **must sum to 1.0** (asserted at boot).
`score = Σ(wᵢ · normalisedᵢ) · 100`. Pure function, no I/O — trivially 100% testable, and the
property test asserts `score ∈ [0,100]` for any input in range.

**6.4 Ranking** — partial selection with `container/heap` for top-N (N≈5) over the candidate crop
set; O(n log N) rather than sorting the whole set. Ties broken by deterministic secondary key (crop
ID) so output is stable and golden-testable.

**6.5 Rotation planning** — model crop sequences as a **DAG**; valid rotations are paths that satisfy
a nitrogen-budget constraint. Use topological ordering plus a greedy nitrogen-balance pass. Guard
against cycles explicitly and return a typed error, never loop forever.

**6.6 ROI estimation** — pure function over a cost vector and a yield triple (low/likely/high).
Integer arithmetic in the smallest currency unit (poisha); **never use floats for money**. Property
test: `net = gross − Σcosts` holds for all generated inputs.

**6.7 Offline sync queue** — client holds an append-only queue with a monotonic local sequence and a
UUIDv7 **idempotency key** per operation. Server dedupes on that key. Conflict policy:
last-write-wins by server-received timestamp, with the loser preserved in an audit row. Document
this in `docs/ARCHITECTURE.md`; do not silently discard data.

**6.8 Scan result cache** — bounded **LRU** (`hashicorp/golang-lru/v2`) keyed by image perceptual
hash, so a re-scan of the same leaf is instant offline.

**6.9 Confidence routing** — single threshold constant `MinDiagnosisConfidence = 0.60` in config.
`≥ 0.60` → show treatment plan; `< 0.60` → low-confidence screen + escalation path. Boundary values
(0.599 / 0.600 / 0.601) are explicit test cases.

**6.10 Rate limiting** — token bucket per `(principal, route-class)` in Redis, with an in-memory
fallback when Redis is absent so local dev and tests need no external service.

**6.11 ID generation** — **UUIDv7** everywhere (time-ordered → index-friendly, unlike v4). Behind
`platform/idgen.Generator` so tests inject a deterministic sequence.

**6.12 Clock** — all time through `platform/clock.Clock`. No direct `time.Now()` in business code; a
lint test enforces it. This is what makes TTL/expiry logic 100% testable.

---

## 7. DESIGN PATTERNS REGISTER

| Pattern | Where | Why |
|---|---|---|
| Repository | every module | C4; swap Postgres without touching use cases |
| Use-case / Interactor | `usecase/` | one struct per operation, one `Execute` method |
| Ports & Adapters | module boundaries | microservice extraction (§3.3) |
| Dependency Injection (constructor) | `module.go` | explicit wiring, no reflection container, no magic |
| Options / functional options | platform constructors | optional deps without constructor explosion |
| Decorator | repo caching, logging, metrics | cross-cutting without touching the adapter |
| Strategy | scoring, language packs, storage backends | swap algorithm by config |
| Adapter | `aiadapter`, storage, SMS | isolate every third party |
| Transactional Outbox | `platform/outbox` | reliable events across future service boundaries |
| Circuit Breaker | outbound HTTP | a failing weather API must not take the app down |
| Result-object errors | `platform/errors` | typed, mappable, translatable |
| Builder | test fixtures | readable table-driven tests |
| Null Object | `aiadapter` stub | app fully works with AI absent |

**Forbidden:** global mutable state, `init()` side effects, service locators, reflection-based DI,
`panic` as control flow, business logic in handlers, SQL in use cases.

---

## 8. TESTING CONTRACT (C11)

### 8.1 The gate

`make verify` = `lint` + `vet` + `arch-test` + `unit` + `integration` + `coverage-gate`.
**A phase is not done until this exits 0.** CI runs the identical command — no divergence.

### 8.2 Coverage: 100%, enforced

- Go: `go test -race -covermode=atomic -coverprofile=coverage.out ./...`, then a
  `scripts/coverage_gate.go` that parses the profile and **fails below 100.0%** for every package.
- The only permitted exclusions, listed in `.coverageignore` and asserted to be exactly this set:
  - `cmd/*/main.go` bootstrap bodies (covered by a smoke test that boots the router)
  - generated code (`*_gen.go`, `*.pb.go`, mocks)
  - `api/index.go` Vercel shim (thin export, covered by router tests)
- Flutter: `flutter test --coverage`, `lcov` filtered to exclude `*.g.dart`, `*.freezed.dart`,
  `l10n/generated/*`; gate at 100%.
- **If code is hard to cover, that is a design signal — refactor it, do not lower the gate.** Every
  branch must be reachable through a public entry point. Unreachable branches must be deleted.

### 8.3 Test taxonomy

- **Unit** — pure, no I/O, table-driven. Use hand-written fakes (deterministic, no codegen
  dependency) for ports. Every use case: happy path + every typed error + every authz denial.
- **Integration** — real Postgres via `testcontainers-go`, real migrations, per-test transaction
  rollback. Covers repositories, middleware chain, and full HTTP round-trips via `httptest`.
- **Contract** — every handler asserted against `openapi/openapi.yaml`; the spec is generated from
  code and diffed, so it can never drift.
- **Property** — scoring, ROI, rotation, dictionary delta (`testing/quick` or `gopter`).
- **Golden** — permission matrix, error-code catalogue, OpenAPI, dictionary key lists.
- **Flutter** — unit (domain/usecases), widget (every screen), golden (theme + RTL Arabic layout),
  integration (`integration_test/` driving the real app against a mocked API).

### 8.4 Rules

- Tests are deterministic: injected clock, injected ID generator, fixed seeds. **No `time.Sleep`,
  no network, no flakes.** A flaky test is a failing test.
- Every bug fixed gets a regression test **in the same commit**.
- Test names state behaviour: `TestRefreshToken_ReusedToken_RevokesFamily`.

---

## 9. GIT & PR PROTOCOL (C13)

- One branch: `claude/sleepy-mccarthy-h8hv7s`. Never force-push. Never branch off it.
- **One commit per phase.** Conventional commits:
  `feat(identity): auth pipeline with rotating refresh tokens (phase 3)`
- Push after every phase: `git push -u origin claude/sleepy-mccarthy-h8hv7s` (retry on network
  error: 2s, 4s, 8s, 16s).
- **Open the PR at the end of Phase 0** and keep it open. Title:
  `AgriSmart: full-stack build (Go API + Flutter, phases 0–18)`. Update the PR body checklist after
  each phase so the user can watch progress. **Do not merge** — the user merges after Phase 18.
- CI must be green on every pushed commit. If CI fails, the next action is fixing it — not the next
  phase.
- Use the `gh` CLI where available for PR creation and CI status; otherwise the GitHub MCP tools.
- Commit trailers on every commit:
  ```
  Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
  ```

---

## 10. RESUME PROTOCOL

`.agent/STATE.md` is maintained by the agent and committed with every phase:

```markdown
# BUILD STATE
Updated: <ISO timestamp>
Branch: claude/sleepy-mccarthy-h8hv7s
PR: #<n>
Last commit: <sha> (<phase>)
CI: green|red|pending

## PHASES
| # | Name | Status | Commit |
|---|------|--------|--------|
| 0 | Bootstrap | done | abc1234 |
| 1 | Platform kernel | in_progress | — |
| … |

## NEXT ACTION
<one precise sentence: the exact file or command to start with>

## BLOCKERS
<none | description + what was tried>

## DECISIONS THIS PHASE
<one line each, mirrored into docs/DECISIONS.md>
```

On resume: read it, verify `git status` is clean, run `make verify`, continue from NEXT ACTION.

---

## 11. PHASE PLAN

Each phase: **goal → read allowlist → deliverables → definition of done**. DoD always includes
`make verify` green, docs updated, commit pushed, `.agent/STATE.md` updated, CI green.

---

### Phase 0 — Bootstrap & guardrails
**Read:** this file only.
**Deliver:** repo skeleton; `go.mod` (Go 1.23+); `Makefile` (`verify, lint, test, unit, integration,
coverage-gate, arch-test, run, migrate, docker, openapi`); `.golangci.yml` (strict: errcheck,
gosec, revive, gocritic, bodyclose, sqlclosecheck, noctx, exhaustive); `.github/workflows/ci.yml`
(matrix: Go build+test+coverage gate, Flutter analyze+test+coverage gate); `scripts/coverage_gate.go`;
`.coverageignore`; `.gitignore`; `.editorconfig`; `docs/` skeleton; `.agent/STATE.md`;
`config.example.yaml`; `.env.example`; open the PR.
**DoD:** `make verify` green on an empty-but-real test; CI green; PR open.

### Phase 1 — Platform kernel
**Read:** `backend/internal/platform/**`, `Makefile`, `.golangci.yml`.
**Deliver:** `config` (§5.1), `logger`, `errors` (§5.6), `validator`, `httpx` (router, middleware
chain §5.2, response writers), `clock`, `idgen` (§6.11–12), `cache` (LRU + Redis iface),
`archtest` (import-graph law §3.3 + no-`time.Now` lint + domain-purity check).
**DoD:** archtest fails on a deliberately planted violation, then passes when removed. 100% coverage.

### Phase 2 — Persistence & migrations
**Read:** `backend/internal/platform/{database,config}`, `backend/migrations`, `backend/test`.
**Deliver:** `pgx` pool with pooler-aware config; migration runner (`golang-migrate`);
transaction manager with context propagation + nested-savepoint support; base repository helpers;
`testcontainers-go` harness with per-test rollback; `outbox` table + dispatcher (§5.7); `eventbus`.
**DoD:** integration suite spins Postgres, applies migrations up and down cleanly, 100% coverage.

### Phase 3 — Identity module (auth + RBAC)
**Read:** `backend/internal/modules/identity/**`, `platform/{authn,authz,errors,httpx}`.
**Deliver:** User/Session/Role domain; OTP request + verify; argon2id; JWT issue/verify with `kid`;
rotating refresh with reuse detection; guest principal; permission matrix + golden test (§5.3);
`Authenticate`/`Authorize` middleware; endpoints `POST /v1/auth/{otp/request,otp/verify,refresh,logout}`,
`GET /v1/me`, `PATCH /v1/me`.
**DoD:** every role × action × ownership combination tested; reuse-detection test proves family
revocation; 100% coverage.

### Phase 4 — Localization module (§5.4, §5.5)
**Read:** `backend/internal/modules/localization/**`, `platform/cache`.
**Deliver:** dictionary + voice-asset domain, repos, snapshot cache with `atomic.Pointer`;
delta endpoint with ETag/304; 7 languages seeded (`bn` default, `ar` flagged RTL); admin write
endpoints behind `dictionary:write`; key-format lint test; `docs/LOCALIZATION.md`.
**DoD:** benchmark proves lock-free O(1) lookup; delta returns only changed entries; 100% coverage.

### Phase 5 — Farm module
**Read:** `backend/internal/modules/farm/**`.
**Deliver:** Field, Plot, SoilProfile, Crop catalogue; GPS coordinate value object with validation;
area unit conversion (bigha/decimal/hectare, integer-safe); CRUD endpoints scoped by ownership.
**DoD:** ownership policy tested for all roles; unit conversion property-tested; 100% coverage.

### Phase 6 — Media module
**Read:** `backend/internal/modules/media/**`, `platform/config`.
**Deliver:** storage port + S3-compatible adapter + local-disk adapter (dev/test); presigned upload
tickets; content-type/size validation; perceptual-hash helper for §6.8; checksum verification.
**DoD:** both adapters pass the same contract test suite; 100% coverage.

### Phase 7 — Diagnosis module
**Read:** `backend/internal/modules/diagnosis/**`, `modules/aiadapter/port.go`, `modules/media`.
**Deliver:** Scan lifecycle (`queued→analysing→completed|low_confidence|failed`) as an explicit state
machine with illegal transitions rejected; Diagnosis + TreatmentPlan (chemical | organic variants);
confidence routing (§6.9); idempotent offline-sync ingest (§6.7); LRU result cache (§6.8);
endpoints `POST /v1/scans`, `GET /v1/scans`, `GET /v1/scans/{id}`, `POST /v1/scans/sync`.
**Calls the AI only through `aiadapter.DiagnosisEngine` — stub implementation.**
**DoD:** state machine exhaustively table-tested; duplicate sync is a no-op; 100% coverage.

### Phase 8 — Advisory module
**Read:** `backend/internal/modules/advisory/**`, `modules/farm`, `modules/weather`.
**Deliver:** scoring (§6.3), top-N ranking (§6.4), yield + ROI (§6.6), rotation DAG (§6.5);
weights validated to sum to 1.0 at boot; endpoints for recommendations, crop detail, ROI, rotation.
**DoD:** property tests on score range and ROI identity; cycle in rotation graph returns typed error;
golden test on a fixed field fixture; 100% coverage.

### Phase 9 — Weather & Alert modules
**Read:** `backend/internal/modules/{weather,alert}/**`, `platform/eventbus`.
**Deliver:** forecast provider port + stub/live adapter behind a circuit breaker; observation cache
with staleness metadata (`data_age_seconds` — the UI shows it); alert rules engine driven by events;
subscription + read/unread endpoints.
**DoD:** breaker opens/half-opens/closes under table-driven fault injection; stale data is flagged
not hidden; 100% coverage.

### Phase 10 — AI adapter (OPEN SLOT — interfaces only)
**Read:** `backend/internal/modules/aiadapter/**`, `docs/AI_INTEGRATION.md`.
**Deliver:** `port.go` with three interfaces and nothing else:
```go
type DiagnosisEngine interface {
    Analyze(ctx context.Context, in AnalyzeInput) (AnalyzeResult, error)
}
type AdvisoryNarrator interface { // chart narration text, per language
    Narrate(ctx context.Context, in NarrateInput) (NarrateResult, error)
}
type ConversationalAgent interface { // voice Q&A
    Ask(ctx context.Context, in AskInput) (AskResult, error)
}
```
Plus `stub/` — a deterministic Null-Object implementation returning fixture data so the whole app
runs and tests at 100% with no model present. Selected by `AI_PROVIDER=stub` (default).
`docs/AI_INTEGRATION.md` documents the contract, the payload shapes, the failure modes, and a
step-by-step "how to plug your model in later" guide.
**DO NOT** implement, train, call, or vendor any model. **DO NOT** add an SDK dependency.
**DoD:** swapping provider is config-only; stub covered 100%; docs complete.

### Phase 11 — API assembly, OpenAPI, deployment targets
**Read:** `backend/cmd/**`, `backend/api/**`, `deploy/**`, `backend/openapi`.
**Deliver:** module wiring in `cmd/api/main.go`; identical tree exported from `api/index.go`
(build-tagged); OpenAPI generated from code + diff test; `Dockerfile` (distroless, multi-stage,
non-root); `docker-compose.yml` (api + postgres + redis + minio); `vercel.json`; health/ready
endpoints; graceful shutdown (Docker target only); `docs/{API,DEPLOYMENT}.md` including the §2
reality-check rules.
**DoD:** container builds and serves; Vercel handler unit-tested; OpenAPI diff clean; 100% coverage.

### Phase 12 — Flutter foundation
**Read:** `mobile/lib/core/**`, `docs/LOCALIZATION.md`.
**Deliver:** project setup; DI (`get_it` + `injectable` or manual — manual preferred, fewer
generated files to exclude from coverage); routing (`go_router`); theme built from the **37 Figma
design tokens** (colour, spacing, radius, typography) as a single source of truth; Dio client with
auth interceptor + refresh-on-401 + retry; secure token storage; Drift/Isar local store; offline
queue (§6.7); **runtime language switching (§5.4)** with `Directionality` driven by `is_rtl`.
**DoD:** golden tests for light theme and for Arabic RTL mirroring; language switch test asserts no
restart; 100% coverage.

### Phase 13 — Flutter: onboarding & auth (Figma 00.1–00.10)
**Deliver:** splash, language picker (7 languages), 3 value slides, phone sign-in, OTP, profile,
permissions primer, offline model download, guest path.
**DoD:** widget test per screen; integration test covers the full first-run flow; 100% coverage.

### Phase 14 — Flutter: Home & Dashboard (Figma 1.0–1.4b)
**Deliver:** dashboard, offline variant with data-age banner, weather detail, alerts, alert detail,
scan history, saved log, Bangla-UI parity check.
**DoD:** offline variant test with the network layer disabled; 100% coverage.

### Phase 15 — Flutter: Plant Doctor (Figma 2.1–2.4)
**Deliver:** camera viewfinder with alignment guide, preview/retake, analysing, result with
confidence meter and chemical/organic tabs, low-confidence escalation, healthy result, offline sync
queue screen.
**DoD:** full scan flow integration test **with the network disabled** — proving offline-first;
100% coverage.

### Phase 16 — Flutter: Crop Advisor + chart narration (Figma 3.0–3.3.3, 3.2b)
**Deliver:** field list + empty state, field setup, GPS pin, seasonal forecast, ranked
recommendations, crop detail, ROI, rotation plan; **chart narration player on every chart**, playing
the pre-recorded clip for the selected language (§5.5) — no TTS.
**DoD:** every chart widget has a narration control asserted present by test; 100% coverage.

### Phase 17 — Flutter: Voice shell + Settings (Figma 4.1–4.4, 5.0–5.5)
**Deliver:** tap-to-talk UI states (idle/listening/understanding/response/not-understood/help) wired
to `ConversationalAgent` **stub**; settings, language selector, offline model manager, expert help,
profile, privacy.
**DoD:** voice states driven by the stub, fully tested; no STT/TTS dependency added; 100% coverage.

### Phase 18 — Hardening & handoff
**Deliver:** E2E suite across both apps; load test on the top 5 endpoints (`k6`/`vegeta`) with
documented baseline numbers; `gosec` + `govulncheck` + `flutter analyze` clean; final
`docs/DEVELOPMENT.md` (setup in under 10 minutes on a clean machine, verified by following it);
`docs/TESTING.md`; `docs/ARCHITECTURE.md` with the module map and the extraction guide; README with
a quickstart; PR body finalised with the full phase checklist.
**DoD:** everything green; PR ready to merge; `.agent/STATE.md` shows all 18 phases done.
**Then stop and report.**

---

## 12. DEFINITION OF DONE (whole build)

- [ ] 18/18 phases committed and pushed, one commit each, one branch, one PR
- [ ] CI green on the head commit
- [ ] 100% coverage, backend and mobile, gate enforced in CI
- [ ] No AI model integrated; the three ports and the stub exist and are documented
- [ ] 7 languages, runtime switching, Arabic RTL handled
- [ ] Runs locally via `docker compose up` and deploys to Vercel from the same handler tree
- [ ] A new developer can go from clone to running tests in under 10 minutes using `docs/DEVELOPMENT.md`

---

## 13. DEVELOPER DOCUMENTATION (C7)

Updated **in the phase that changes the behaviour**, never batched at the end:
`README.md`, `docs/ARCHITECTURE.md` (+ microservice extraction guide), `docs/API.md` (+ OpenAPI),
`docs/DEVELOPMENT.md`, `docs/DEPLOYMENT.md`, `docs/LOCALIZATION.md`, `docs/TESTING.md`,
`docs/AI_INTEGRATION.md`, `docs/DECISIONS.md` (ADR-lite, one entry per non-obvious choice).

---

## 14. OPEN SLOTS — DO NOT BUILD

1. **The AI/ML model.** No training, no inference, no model SDK, no vendored weights. Only the three
   interfaces in Phase 10 plus the deterministic stub. The user is building this separately.
2. **TTS / STT engines.** Voice is pre-recorded asset delivery (§5.5). The voice UI talks to the
   stub agent.
3. **Payments, subscriptions, ads.**
4. **Admin web console.** API endpoints only.
5. **Push notification provider integration** — port + stub only.

If a phase seems to need one of these, implement against the port and use the stub. Never reach for
a real provider.

---

## 15. FIRST MESSAGE TO THE BUILD AGENT

> Read `docs/agent/BUILD_INSTRUCTION.md` in full, then `.agent/STATE.md`. Execute every phase from
> the current state through Phase 18 without stopping. A phase is complete only when `make verify`
> is green and the commit is pushed. Do not merge the PR. Do not build anything listed in §14.
> If you hit a usage limit, update `.agent/STATE.md`, commit, push, and stop cleanly.
</content>
