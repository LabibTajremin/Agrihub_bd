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
