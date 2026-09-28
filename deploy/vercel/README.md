# Vercel (MVP target)

The Vercel project's **Root Directory is `backend/`**; its `vercel.json` routes every path to the
Go function `api/index.go`, which exports the same handler tree as `cmd/api` (built with
`-tags serverless`, so the HTTP server and background worker are compiled out).

Set these environment variables in the Vercel project (Production and Preview):

| Variable | Value |
|---|---|
| `AGRI_DATABASE_URL` | **pooled** connection string (Neon/Supabase pgbouncer, port 6543 / `-pooler` host) |
| `AGRI_DATABASE_POOL_MODE` | `transaction` (default) |
| `AGRI_AUTH_JWT_SECRET` | ≥ 32 random chars |
| `AGRI_STORAGE_BACKEND` | `s3` (Vercel's filesystem is ephemeral) |
| `AGRI_STORAGE_S3_ENDPOINT`, `…_BUCKET`, `…_REGION`, `…_ACCESS_KEY`, `…_SECRET_KEY`, `…_USE_SSL` | your S3/R2 bucket |
| `AGRI_REDIS_URL` | optional (Upstash); in-memory rate limiting otherwise (per instance) |
| `AGRI_APP_BASE_URL` | the deployment URL |

Run migrations and seed from CI or a workstation against the **direct** (non-pooled) URL:
`make migrate seed` with `AGRI_DATABASE_URL` set. See `docs/DEPLOYMENT.md`.
