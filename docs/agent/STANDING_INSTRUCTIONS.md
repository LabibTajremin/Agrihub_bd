# Standing Instructions — Labib's default build rules

These are my reusable preferences for any AI-driven build. When I say *"give me my generic
instructions"*, return this file. Project-specific detail lives elsewhere; this file is the
portable part.

---

## 1. Stack

| Layer | Choice |
|---|---|
| Backend | **Go** |
| API style | **REST, API-only.** No server-rendered HTML |
| Frontend | **Flutter** |
| Priority | **Mobile-first** (Android primary, iOS parity) |
| MVP deploy | **Vercel** |
| Production deploy | **Docker image** |

---

## 2. Architecture

- **Clean architecture.** Domain layer depends on nothing outward.
- **Repository pattern.** Ports in the domain, adapters outside.
- **Modular monolith, microservice-ready.** Every module is a folder that can be lifted into its own
  service without a rewrite:
  - no module imports another module's internals
  - cross-module reads go through a published port interface
  - cross-module writes go through an event bus
  - each module owns its tables; no cross-module joins or foreign keys
  - an automated import-graph test enforces all of the above
- Explicit constructor dependency injection. No reflection containers, no service locators, no
  global mutable state, no `init()` side effects.

---

## 3. Auth & configuration

- Proper auth pipeline: OTP/credential → short-lived access token → rotating refresh token with
  reuse detection. Strong password/OTP hashing (argon2id).
- **Role-based access control**, enforced at the use-case boundary, not only at the HTTP layer.
- A single typed, validated config surface. Precedence: defaults → config file → env → flags.
  Ship a `config.example.yaml` documenting **every** key, with a test asserting the example covers
  100% of the config struct. Secrets in env only, never in files.

---

## 4. Localization (text and voice)

- **Dictionary per language**, key→value pairs. English dictionary holds all English strings, Bangla
  holds all Bangla strings, and so on.
- The user's preferred language is applied **in real time** — switching language never requires an
  app restart.
- Keys follow `screen.section.element`. A test asserts every key used in code exists in every
  dictionary.
- **Voice uses the identical mechanism**: same keys, audio assets instead of strings, delivered as a
  manifest, cached by checksum, playable offline.
- RTL languages carry an `is_rtl` flag; layout direction is driven by that flag, never hardcoded.

---

## 5. The AI slot stays open

I am building the farming model myself. The build agent must:

- define the AI **interfaces only** (diagnosis, narration, conversational agent)
- ship a deterministic **stub** so the whole app runs and tests fully without a model
- select the implementation by config
- document the contract and how to plug a real model in later
- **never** train, call, vendor, or add an SDK for any model, TTS, or STT

Everything else gets built completely.

---

## 6. Testing

- **100% test coverage**, unit *and* integration, backend and frontend. Gated in CI.
- Permitted exclusions are only: bootstrap `main` bodies, generated code, and thin deploy shims —
  listed explicitly in a `.coverageignore` that is itself asserted.
- Hard-to-cover code is a design smell: refactor it, never lower the gate.
- Deterministic tests only — injected clock, injected ID generator, fixed seeds. No sleeps, no
  network, no flakes. A flaky test is a failing test.
- Every bug fix ships with a regression test in the same commit.
- All pipelines and all code fall under coverage.

---

## 7. Phased delivery & git protocol

- Work in **phases**. A phase is complete **only when every test is green**.
- **One commit per phase**, pushed separately, **all on the same branch**.
- **All phases go into one PR.** I merge it myself after every phase is finished.
- CI must pass on every pushed commit. A red CI is the next thing to fix — not the next phase.
- Conventional commit messages. Never force-push.
- Use the `git` CLI (and `gh` where available) for testing, status, and PR work.

---

## 8. Autonomy

- **Do not stop before all phases are complete.**
- Do not pause for approval mid-build. Decisions are pre-made; if something is genuinely ambiguous,
  choose the option that satisfies the stated constraints, record it in `docs/DECISIONS.md`, and
  keep going.
- If a usage limit is hit: write the exact next action into the state ledger, commit, push, stop
  cleanly — then resume from the ledger when the limit resets.
- Maintain a state ledger (`.agent/STATE.md`) so any session can resume with no context loss.

---

## 9. Engineering quality bar

- Specify the algorithm and data structure rather than leaving it to taste; prefer the one with the
  right complexity for the access pattern.
- Money in integer minor units — never floats.
- Time-ordered IDs (UUIDv7), not random v4.
- All time via an injected clock.
- Named design patterns where they earn their place: repository, use-case interactor, ports &
  adapters, decorator, strategy, adapter, transactional outbox, circuit breaker, null object,
  builder for fixtures.
- Forbidden: business logic in handlers, SQL in use cases, panic as control flow.

---

## 10. Documentation

- Developer docs updated **in the phase that changes the behaviour**, never batched at the end.
- A new developer goes from clone to green tests in under 10 minutes by following the docs.
- Architecture doc includes the microservice extraction guide.

---

## 11. Token efficiency (for the AI doing the work)

- Each phase declares a read allowlist; read only those files.
- Locate code with `rg`, never by reading files speculatively.
- Never re-read an unchanged file in the same session.
- Write complete files in one pass.
- Print test output only on failure.
- Keep the state ledger short — a ledger, not a log.
</content>
