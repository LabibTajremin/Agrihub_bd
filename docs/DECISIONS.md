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

## Phase 10
- **Provider selection lives in `aiadapter/registry`**, not in the port package — the stub imports the
  port types, so the port package cannot import the stub (cycle); a registry keeps "swap = config".
- **`ai.provider` is validated against the registry at boot**, not by a config `oneof` — registering a
  model must not require editing the config validator.
- **Bug fix + regression test:** OTP rejection sampling was only covered by chance (crypto/rand);
  `TestRandomDigits_RejectionSamplingIsDeterministic` pins it.

## Phase 11
- **Bug fix + regression coverage:** jsonb parameters were sent as `[]byte`, which pgx encodes as
  bytea in the pgbouncer-safe exec mode (`pool_mode=transaction`) → Postgres rejected the JSON. All
  jsonb writes now send text, and the test harness uses the production pool mode so the whole suite
  guards against this class of bug.
- **Outbox is flushed after every request** (not only writes) — reads can publish (fresh weather
  fetch) and serverless has no worker; with nothing pending it is a single indexed query.
- **`Serve` stops the outbox worker when the server exits on its own**, not only on cancellation.
- **Module constructor errors are joined (`errors.Join`)** in the composition root — one failure path.
- **Config files are read with `os.ReadFile`** (absolute paths work, e.g. mounted in a container).
- **Docker image defaults to `pool_mode=session`** (direct Postgres); Vercel uses the `transaction`
  default behind a pooler.

## Phase 12
- **Local store: drift over SQLite with raw SQL (no codegen)** — Drift as specified, without generated
  files to exclude from coverage.
- **Manual DI (`AppServices` + `AppScope`)** — preferred by the spec; nothing generated.
- **State: `ChangeNotifier` + `ListenableBuilder`** — no state-management dependency.
- **Design tokens are defined in code** (`core/theme/tokens.dart`, 37 tokens) — the Figma file was not
  available to this build; values follow the kit's structure (colour 18, spacing 8, radius 4,
  elevation 1, type 6) and can be updated in one place.
- **Goldens are rendered with the test font (Ahem)** — they pin layout and mirroring, not glyphs.

## Phase 13 — Flutter onboarding & auth
- **Permissions primer, not permission requests** — the primer explains camera/location/microphone;
  the OS prompt appears on first use of each (camera, geolocator, recorder). No extra plugin.
- **Offline model download is a `ModelSource` port + `StubModelSource`** (§14 open slot): progress
  and install state are real (`OfflineModelManager`), the package itself is not.
- **One first-run flow, two runners** — `test/flows/first_run.dart` runs headless in CI
  (`test/features/onboarding/first_run_flow_test.dart`) and on a device (`integration_test/`).
- **Unknown server error keys fall back to `errors.internal`** (`context.tError`), so a newer server
  never crashes an older app in strict mode.

## Phase 14 — Flutter home & dashboard
- **`CachedReader` for offline-first reads** — every GET is remembered in the local store; offline,
  the copy is served with its age (server-reported age + time since fetch) and the banner shows it.
- **Dashboard sections load independently** — a failed or forbidden section (e.g. a guest's inbox)
  renders empty rather than blanking the page.
- **Home location defaults to Bangladesh's centre** until a field is pinned (phase 16).
- **Dates are formatted with `MaterialLocalizations`** — localized without adding `intl`.

## Phase 15 — Flutter Plant Doctor
- **Diagnosis runs on the phone first** (`DiagnosisEngine` port, `StubDiagnosisEngine` — §14 open
  slot). The result is shown immediately, online or not; the photo and scan are stored locally.
- **Upload happens at sync time**: ticket → signed PUT (no bearer header) → complete → queue
  `create_scan` → flush. Saves made offline follow as `annotate_scan` once the server id is known.
- **Client dHash cache** (64-bit, Hamming ≤ 4, same crop, newest 50) skips the model for repeat photos.
- **Photos live in a `blobs` table** (local schema v2, upgraded in place) — no file-path bookkeeping.
- **Camera behind `CameraGateway`**; the plugin adapter is tested through a `CameraPlatform` fake.
- **No gallery picker** — it would add a plugin; the camera is the product path.
- **Sync is manual ("Sync now") in this phase**; automatic sync on reconnect is deferred to hardening.

## Phase 16 — Flutter Crop Advisor + chart narration
- **One chart widget (`BarChart`) that always renders a `NarrationControl`** — narration cannot be
  omitted; a test walks every chart screen and asserts each chart carries exactly one control.
- **Narration is client-side clip delivery** (§5.5): chart kind → `narration.chart.<kind>` key →
  clip from the language's voice manifest, downloaded lazily, verified by SHA-256 and cached by
  checksum in the local blob table; the text is always shown. `/v1/advisory/narrate` (AI slot) is
  not called. A missing clip shows "audio not available".
- **Audio and GPS behind ports** (`AudioOut`, `LocationGateway`), adapters tested through the
  audioplayers and geolocator platform fakes. The audio position ticker is disabled (no progress UI).
- **Money is poisha end to end**; the UI formats taka with the dictionary's `unit.taka` pattern.
- **Creating a field also sets the home location** used by weather and the seasonal outlook.

## Phase 17 — Flutter voice shell + settings
- **Voice capture is a `VoiceInput` port (§14 open slot)**; the shipped `NoVoiceInput` captures
  nothing, so tap-to-talk falls through to the help state and its suggested questions, which are
  sent as transcripts to `/v1/assistant/ask` (the server's agent stub). No STT, no TTS.
- **Answers play the pre-recorded clip of their answer key** through the same narration pipeline.
- **Understood intents link to the matching module** (weather detail, Plant Doctor, Crop Advisor).
- **Expert help copies the helpline number** — no dialer plugin; the number is shown and copied.
- **Sign-out always completes locally** (tokens + cached profile dropped) even when the server
  revoke call cannot be made.
- **Language changes are pushed to the profile best-effort**; offline, the local choice stands.
