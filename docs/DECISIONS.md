# Decisions (ADR-lite)

One line per non-obvious choice: **decision** — rationale.

## Phase 0
- **Work on branch `claude/festive-maxwell-n5xdmi`, not `claude/sleepy-mccarthy-h8hv7s`** — the session
  that executes the build is bound to this branch; everything else in the protocol is unchanged.
- **Go module lives in `backend/`** (`github.com/labibtajremin/agrihub_bd/backend`) — keeps the
  Vercel root (`backend/`) self-contained and lets `api/` import `internal/`.
- **Coverage gate logic is a tested package (`internal/tools/covgate`); `scripts/coverage_gate.go`
  is an `//go:build ignore` launcher** — the gate itself falls under 100% coverage.
- **Coverage is measured in one `-tags integration -coverpkg=./...` run, merged per block by max
  count** — unit and integration tests together form the suite the gate judges.
- **`make verify` = `verify-backend` + `verify-mobile`; CI runs each half in its own job** — same
  targets, parallel jobs.
- **Flutter pinned to 3.47.5 (Dart 3.13)** — current stable at build start.

## Phase 1
- **Go 1.26** — current validator/redis releases require ≥1.26; golangci-lint v2.14.0 to match.
- **Error package is `platform/errs`** (spec: `platform/errors`) — avoids shadowing the stdlib `errors`
  package in every file that needs both.
- **Domain may import `platform/errs`** (stdlib-only shared kernel) — domain errors must be typed
  `*errs.Error` for the single Kind→status mapping; archtest allows exactly this one import.
- **Modules may import the `aiadapter` root package** — it is a published port contract (stdlib-only
  interfaces); every other cross-module import is rejected by archtest.
- **Config adds `advisory` and `weather` sections** — weights and breaker settings must be config-driven.
- **Config flags are `-config <path>` and repeatable `-set section.key=value`** — generic, covers every key.
- **Defaults are parsed through the same layer code as env** — one parsing path, no unreachable branch.
- **Rate limiting runs before authentication, keyed by a hash of the bearer token, else client IP** —
  the spec's middleware order puts RateLimit before Authenticate; a token hash approximates the principal.
- **Rate limiter fails open** — a Redis outage degrades to the in-memory bucket, then to allow.
- **Router uses stdlib `http.ServeMux` patterns** — no router dependency; unmatched paths return the
  standard JSON error body.

## Phase 2
- **Repositories read rows via `database.Collect` (pgx `CollectRows`)** — scan and iteration errors
  surface through one return, keeping repositories nearly branch-free.
- **Fault injection wraps a real `pgx.Tx`** — error branches are tested against real Postgres
  rather than mocks.
- **The harness keeps one container + one migrated DB per test binary in package state** — the only
  shared mutable state in the codebase, confined to test support.
- **Outbox relay runs per-event savepoints inside one locking transaction** — a failing consumer
  is rolled back alone; its row records the attempt.

## Phase 3
- **Use cases are tested against real repositories inside rolled-back transactions** (plus fakes for
  SMS/limiter/issuer and fault injection for storage) instead of hand-written in-memory repository
  fakes — the same guarantees with half the code, and the SQL is exercised too.
- **`POST /v1/auth/guest`** added — guest mode needs an identity for guest-owned scans.
- **Failure writes that must survive an error (OTP attempts, family revocation) commit first, then the
  use case returns the error.**
- **Refresh tokens hashed with SHA-256, OTP codes with argon2id** — tokens are high-entropy; codes are not.
- **SMS is a port with a logging stub** — provider integration is out of scope; `features.expose_otp`
  returns the code in development.
- **Harness pool raised to 60 connections; tests use distinct unique keys (phones) per open
  transaction** — concurrent uncommitted inserts on a unique key block each other.

## Phase 4
- **Dictionary tables use the spec's names (`dictionary_entries`, `voice_assets`, `languages`)**;
  other modules prefix tables with the module name.
- **Catalog is copy-on-write with a CAS loop** — concurrent publishers never lose an update.
- **Voice ships with an empty manifest** — clips are recorded content, not generated; the app shows
  the text and disables play until a clip exists (`common.audio_unavailable`).
- **Seed dictionaries are authored once for the whole app (319 keys)** so later phases rarely touch
  seven files; new keys are added to all seven in the phase that needs them.

## Phase 5
- **Canonical area unit is 10⁻⁹ m²** — the smallest unit in which an acre (and so a decimal and a
  bigha) is an exact integer; mm² is not.
- **Bigha = 33 decimals** — the Bangladesh standard; regional bighas differ and are out of scope.
- **Crop catalogue is code, not a table** — immutable reference data, versioned with the code.
- **Updates and deletes are owner-or-admin (`field:write_any`)**; officers and agronomists can read
  any field but not edit it.

## Phase 6
- **S3 contract tests use gofakes3 (in-process) instead of a MinIO container** — MinIO images are not
  pullable from this environment; gofakes3 is deterministic and needs no network.
- **Upload verification reads the blob back** (≤ `storage.max_upload_bytes`) to check SHA-256 and
  compute the dHash — simple and correct for leaf photos; large media would stream instead.
- **Router supports a per-route body limit** (`Route.MaxBodyBytes`) for binary uploads.

## Phase 7
- **`aiadapter` ports and stub were created in Phase 7** (diagnosis depends on them); Phase 10 adds
  provider selection, the assistant endpoint and the integration guide.
- **A failed analysis is a scan state, not an HTTP error** — the client receives `status: failed`
  and may retry; the event `diagnosis.scan.failed` is published.
- **Scan analysis is synchronous within the request** — works on serverless (no workers); the
  engine port can later be backed by a queue without changing call sites.
- **Diagnosis cache is shared across users** — a diagnosis of a leaf image is not personal data.

## Phase 8
- **Advisory routes live under `/v1/advisory/...`** — a path prefix per module keeps gateway routing
  trivial when the module is extracted.
- **Advisory owns no tables** — it is computation over farm, weather and AI ports.
- **Rotation scores use each crop's first season's rainfall** — a crop appears once per plan layer; a
  per-layer score adds complexity without changing typical plans.

## Phase 9
- **Weather cache refreshes at `stale_after/3`** — one threshold in config drives both freshness and
  the stale flag.
- **Seasonal outlook = regional climatology scaled by this week's rainfall anomaly (±30%)** — the
  forecast API has no seasonal product; the method is documented and deterministic.
- **Alert consumers reference topics by name, never other modules' types** — each consumer decodes
  its own payload struct, so extraction needs no shared code.
- **Notifier failures are logged, not retried** — the alert is already stored and visible in the inbox.
