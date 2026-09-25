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
