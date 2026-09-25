# Testing

- **Gate:** 100% per package (Go) and 100% of lines (Flutter). `make coverage-gate`, `make mobile-coverage-gate`.
- **Exclusions:** only those in `backend/.coverageignore` — asserted by
  `TestCoverageIgnore_IsExactlyThePermittedSet`. Flutter excludes `*.g.dart`, `*.freezed.dart`, `l10n/generated/`.
- **Output discipline:** `scripts/quiet.sh` prints test output only on failure.
