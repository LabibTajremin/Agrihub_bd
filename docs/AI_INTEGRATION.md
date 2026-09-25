# AI integration — the open slot

AgriSmart ships **no model**. The system depends on three ports declared in
`backend/internal/modules/aiadapter/port.go` and runs on a deterministic stub
(`aiadapter/stub`) selected by `ai.provider: stub` (the default). This document is the contract
for plugging a real model in later.

## The three ports

| Port | Used by | Purpose |
|---|---|---|
| `DiagnosisEngine.Analyze(ctx, AnalyzeInput) (AnalyzeResult, error)` | diagnosis (`POST /v1/scans`) | classify a leaf photo |
| `AdvisoryNarrator.Narrate(ctx, NarrateInput) (NarrateResult, error)` | advisory (`POST /v1/advisory/narrate`) | explain a chart |
| `ConversationalAgent.Ask(ctx, AskInput) (AskResult, error)` | assistant (`POST /v1/assistant/ask`) | voice Q&A |

### DiagnosisEngine
**Input** — `ScanID`, `CropCode` (farm catalogue code, e.g. `rice_aman`), `ContentType`
(`image/jpeg|png`), `Image` (verified bytes, ≤ `storage.max_upload_bytes`), `PHash` (64-bit dHash),
`Lang`.

**Output** — `DiseaseCode` (one of `rice_blast, brown_spot, bacterial_leaf_blight, sheath_blight,
tungro, healthy`; add new codes together with their dictionary keys `disease.<code>.name|description`
and treatment keys), `Confidence ∈ [0,1]`, `Severity ∈ {low, medium, high}`, `Alternatives`,
`ModelVersion`.

**Routing is not the model's job.** The diagnosis module applies
`ai.min_diagnosis_confidence` (0.60): `≥` → treatment plan, `<` → low-confidence screen.
Results are cached by dHash, so a re-scan of the same leaf does not call the engine again.

### AdvisoryNarrator
**Input** — `ChartKind ∈ {scores, forecast, yield, roi, rotation, weather}`, `Lang`, `Data`
(series label → value). **Output** — `TextKey`/`VoiceKey` (dictionary keys, so text and the
pre-recorded clip come from localization), `Params`, optional free `Text` for a generative model.

### ConversationalAgent
**Input** — `Lang`, `Transcript` (typed text or a tapped suggestion; there is no STT), optional
`AudioMediaID` (a recorded question uploaded via the media module, for a future speech model),
`Context`. **Output** — `Understood`, `Intent`, `AnswerKey` (dictionary key), `Params`,
`FollowUps` (dictionary keys of suggested questions).

## Failure modes (all implementations must honour)
| Situation | Return | Effect |
|---|---|---|
| Model/service down, timeout | `aiadapter.ErrUnavailable` (or any error) | scan → `failed` (retryable via `POST /v1/scans/{id}/retry`); narrate/ask → `503` |
| Input the model cannot use | `aiadapter.ErrInvalidInput` | `400 ai.invalid_input` |
| Low certainty | a normal result with low `Confidence` | routed to the low-confidence screen |

Honour `ctx` cancellation (requests carry `http.request_timeout`, default 15 s). Never return
confidence outside [0,1].

## The stub (Null Object)
`stub.Engine` implements all three ports as pure functions of the input: the disease and a
confidence in [0.50, 0.99] derive from the image dHash (or SHA-256), so every label and both
confidence routes are exercised; narration returns `narration.chart.<kind>`; the agent matches a
small multilingual keyword table. No network, no randomness — the whole app runs and tests at 100%.

## How to plug your model in
1. **Implement the ports** in a new package, e.g. `backend/internal/modules/aiadapter/mymodel`:
   ```go
   type Client struct{ BaseURL string; HTTP *http.Client }
   func (c Client) Analyze(ctx context.Context, in aiadapter.AnalyzeInput) (aiadapter.AnalyzeResult, error) { … }
   ```
   Map transport errors to `aiadapter.ErrUnavailable`. Keep SDKs inside this package only.
2. **Register it** in `aiadapter/registry/registry.go`:
   ```go
   "mymodel": func() Providers { c := mymodel.Client{…}; return Providers{Diagnosis: c, Narrator: stub.Engine{}, Agent: c} },
   ```
   (Mix and match: keep the stub for any port you have not built yet.)
3. **Configure** `AGRI_AI_PROVIDER=mymodel` (plus any keys your client needs, as env secrets added to
   `config.AI`, `config.example.yaml` and `.env.example` — the example-coverage test enforces this).
4. **Test** with the existing suites: diagnosis, advisory and assistant tests run against any
   implementation of the ports; add a contract test for your client with an `httptest` server.
5. For **on-device** inference in the app, implement the Flutter `DiagnosisEngine` interface in
   `mobile/lib/features/doctor/domain` the same way; the offline model manager downloads your model
   file (see `docs/ARCHITECTURE.md`).

Nothing else changes: no call site, handler or table depends on the implementation.
