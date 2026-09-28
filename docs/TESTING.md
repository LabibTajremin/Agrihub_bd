# Testing

| Layer | Where | Command | Needs |
|---|---|---|---|
| Go unit | `backend/**/*_test.go` | `make unit` | Go |
| Go integration (+ coverage) | `//go:build integration` | `make integration coverage-gate` | Docker |
| Architecture laws | `internal/platform/archtest` | `make arch-test` | Go |
| Flutter widget/unit (+ coverage) | `mobile/test` | `make mobile-test mobile-coverage-gate` | Flutter |
| E2E: Flutter client ↔ Go API ↔ Postgres | `mobile/e2e` | `make e2e` | Docker, Go, Flutter |
| On-device first-run journey | `mobile/integration_test` | `cd mobile && flutter test integration_test` | emulator/phone |
| Load baseline | `scripts/loadtest.sh` | `make tools loadtest` | Docker, Go |
| Security | golangci (gosec) + govulncheck + flutter analyze | `make lint security` | Go, Flutter |

`make verify` runs everything CI gates on except E2E, which CI runs as its own job.

## Coverage gate
- **100%** per package (Go) and 100% of lines (Flutter): `make coverage-gate`, `make mobile-coverage-gate`.
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

## Mobile tests
- `TestKit` (`mobile/test/support/fakes.dart`) builds `AppServices` from fakes: an in-memory HTTP
  backend (`FakeServer`, `offline = true` disables the network), in-memory SQLite, a fixed clock, and
  platform fakes for camera, geolocator, audioplayers and connectivity. Canned server payloads:
  `stubAuth`, `stubHome`, `stubUpload`, `stubAdvisor`, `stubVoice`, `stubAssistant`.
- Screen tests live in `mobile/test/features/<feature>/`; journeys in `mobile/test/flows/`
  (the first-run journey runs both headless and on a device).
- Offline-first proofs: the full scan flow (`doctor_test.dart`) and the dashboard's offline variant
  (`home_test.dart`) run with the network layer disabled.
- Every chart is asserted to carry a narration control (`advisor_test.dart`).
- Goldens (`mobile/test/app/goldens`) pin layout and RTL mirroring; update with
  `flutter test --update-goldens test/app/app_test.dart`.

## E2E (`make e2e`)
`scripts/with-stack.sh` starts Postgres in Docker, migrates, seeds, builds and runs the API with test
settings (`features.expose_otp`, rate limits lifted), then `mobile/e2e/api_e2e_test.dart` drives the
**app's own repositories** against it: dictionary delta sync in all 7 languages; OTP sign-in, profile,
refresh rotation, sign-out; guest session; offline scan → signed upload → queue sync → history and
saved log (and idempotent replay); field → recommendations → crop → ROI → rotation → seasonal outlook;
alerts; the assistant stub; the voice manifest.

## Load baseline (`make tools loadtest`)
vegeta against the five hottest endpoints, one authenticated farmer, the Docker stack above.
Measured 2026-09-28 on a 4-vCPU / 16 GB Linux container, single API process, local Postgres 16:

| endpoint | rate | requests | success | p50 | p95 | p99 | max |
|---|---|---|---|---|---|---|---|
| `GET /v1/i18n/bn` | 200/s | 3000 | 100.00% | 2.6 ms | 3.4 ms | 6.2 ms | 79.2 ms |
| `GET /v1/weather` | 200/s | 3000 | 100.00% | 1.8 ms | 2.5 ms | 3.4 ms | 12.7 ms |
| `GET /v1/scans` | 200/s | 3000 | 100.00% | 1.7 ms | 2.4 ms | 4.0 ms | 29.7 ms |
| `GET /v1/advisory/fields/{id}/recommendations` | 200/s | 3000 | 100.00% | 2.5 ms | 3.5 ms | 5.0 ms | 19.4 ms |
| `POST /v1/assistant/ask` | 200/s | 3000 | 100.00% | 1.3 ms | 2.1 ms | 4.3 ms | 39.9 ms |
| `GET /v1/i18n/bn` | 1000/s | 9999 | 100.00% | 4.0 ms | 15.4 ms | 54.6 ms | 94.2 ms |
| `GET /v1/weather` | 1000/s | 10000 | 100.00% | 1.5 ms | 2.7 ms | 9.9 ms | 42.9 ms |
| `GET /v1/scans` | 1000/s | 10000 | 100.00% | 1.6 ms | 2.5 ms | 4.2 ms | 26.0 ms |
| `GET /v1/advisory/fields/{id}/recommendations` | 1000/s | 10000 | 100.00% | 2.6 ms | 5.5 ms | 9.2 ms | 26.2 ms |
| `POST /v1/assistant/ask` | 1000/s | 10000 | 100.00% | 1.1 ms | 1.7 ms | 3.5 ms | 24.9 ms |

The full dictionary (`i18n`, ~320 keys) is the heaviest payload; clients normally fetch deltas
(`?since=`) or get `304` via ETag. Re-run with `RATE=… DURATION=… make loadtest`.

## Security
- `gosec` runs inside `make lint` (golangci-lint). `make security` adds `govulncheck` and
  `flutter analyze --fatal-infos`; CI runs govulncheck on every push.
- Last govulncheck: no vulnerable code paths reached. One module-level advisory with no fix
  (`golang.org/x/crypto/openpgp`, GO-2026-5932) is not imported by any package here.
