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
