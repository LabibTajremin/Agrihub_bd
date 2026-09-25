# API

REST, JSON, versioned under `/v1`. The OpenAPI document (`backend/openapi/openapi.yaml`) is
generated from the route table and diff-tested (Phase 11).

## Conventions
- Auth: `Authorization: Bearer <access token>`. Requests without a token run as an anonymous guest
  and may only call public routes.
- Errors — one wire format everywhere:
  `{"error":{"code":"scan.not_found","message":"errors.scan.not_found","fields":{…},"request_id":"…"}}`.
  `message` is an i18n key; clients translate it.
- Every response carries `X-Request-ID`. Rate-limited responses carry `Retry-After`.

## Identity
| Method | Path | Permission | Notes |
|---|---|---|---|
| POST | `/v1/auth/otp/request` | public (auth rate class) | `{phone}` → 202 `{expires_at, dev_code?}` |
| POST | `/v1/auth/otp/verify` | public | `{phone, code}` → session; a guest caller is upgraded in place |
| POST | `/v1/auth/guest` | public | → 201 guest session (first scan without an account) |
| POST | `/v1/auth/refresh` | public | `{refresh_token}` → rotated pair; replay revokes the family |
| POST | `/v1/auth/logout` | `profile:read` | revokes the session |
| GET | `/v1/me` | `profile:read` | profile |
| PATCH | `/v1/me` | `profile:write` | `{name?, language?, district?}` |

## Localization
| Method | Path | Permission |
|---|---|---|
| GET | `/v1/i18n/languages` | public |
| GET | `/v1/i18n/{lang}?since=` | public (ETag/304) |
| PUT | `/v1/i18n/{lang}/entries` | `dictionary:write` |
| GET | `/v1/voice/{lang}?since=` | public (ETag/304) |
| PUT | `/v1/voice/{lang}/assets` | `voice:write` |
See `docs/LOCALIZATION.md`.

## Farm
| Method | Path | Permission | Notes |
|---|---|---|---|
| GET | `/v1/crops`, `/v1/crops/{code}` | public | crop catalogue (names are i18n keys `crop.<code>.name`) |
| GET | `/v1/fields?owner_id=` | `field:read` | own fields; another owner needs `field:read_any` |
| POST | `/v1/fields` | `field:create` | `{name, area:{value_milli, unit}, location:{lat,lng}, district, irrigation, soil?}` |
| GET/PATCH/DELETE | `/v1/fields/{id}` | `field:read` / `field:write` | ownership policy applies |
| PUT | `/v1/fields/{id}/soil` | `field:write` | soil test |
| POST | `/v1/fields/{id}/plots` | `field:write` | plots may not exceed the field area |

Areas are sent as `value_milli` (thousandths of `unit` ∈ decimal, bigha, acre, hectare) and returned
in every unit.

## Media
Upload flow: **ticket → PUT bytes → complete**.
| Method | Path | Permission | Notes |
|---|---|---|---|
| POST | `/v1/media/tickets` | `media:upload` | `{content_type, size_bytes, checksum_sha256}` → `{media_id, upload_url, method:"PUT", headers, expires_at}` |
| PUT | `upload_url` | signed URL | S3 presigned URL, or `/v1/media/blob/{key}` (local backend, HMAC-signed) |
| POST | `/v1/media/{id}/complete` | `media:upload` (owner) | verifies size + SHA-256, computes the image dHash; mismatches are deleted |
| GET | `/v1/media/{id}` | `media:read` (owner or `scan:read_any`) | metadata + presigned `download_url` |

## Diagnosis (Plant Doctor)
| Method | Path | Permission | Notes |
|---|---|---|---|
| POST | `/v1/scans` | `scan:create` | `{media_id, crop_code, field_id?, idempotency_key, captured_at, lang?}` → 201 new, 200 replay |
| GET | `/v1/scans?saved=&owner_id=&limit=&before=` | `scan:read` | newest first; `next_before` cursor |
| GET | `/v1/scans/{id}` | `scan:read` | owner or `scan:read_any` |
| PATCH | `/v1/scans/{id}` | `scan:create` (owner) | `{note?, saved?}` — last-write-wins, loser audited |
| POST | `/v1/scans/{id}/retry` | `scan:create` (owner) | failed → queued → analysed again |
| POST | `/v1/scans/sync` | `scan:sync` | offline queue `{operations:[{idempotency_key, seq, kind, create?/scan_id+annotation?}]}` |

Scan `status`: `queued → analysing → completed | low_confidence | failed` (`failed → queued` on retry).
`diagnosis.confidence ≥ ai.min_diagnosis_confidence (0.60)` → `completed` with chemical and organic
plans (steps are dictionary keys); below → `low_confidence` (client shows the escalation screen).
