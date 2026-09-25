# Testing

- **Gate:** 100% per package (Go) and 100% of lines (Flutter). `make coverage-gate`, `make mobile-coverage-gate`.
- **Exclusions:** only those in `backend/.coverageignore` — asserted by
  `TestCoverageIgnore_IsExactlyThePermittedSet`. Flutter excludes `*.g.dart`, `*.freezed.dart`, `l10n/generated/`.
- **Output discipline:** `scripts/quiet.sh` prints test output only on failure.

## Integration harness (`backend/test/harness`)
- Integration tests carry `//go:build integration` and need Docker: one `postgres:16-alpine`
  container per test binary (testcontainers), one freshly migrated database per binary.
- `harness.DB(t)` returns a handle bound to a transaction that is **rolled back** when the test ends.
- `harness.FaultyDB(t, harness.Fault{Op: "exec", Match: "UPDATE outbox", Err: boom})` injects
  failures so every repository error branch is testable against a real database.
- Use `idgen.UUIDv7{}` (not `idgen.Sequence`) when several transactions insert rows in one test:
  equal primary keys in concurrent uncommitted transactions block on each other.
