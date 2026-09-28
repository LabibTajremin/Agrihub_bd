#!/usr/bin/env bash
# with-stack.sh <cmd...> — starts Postgres (Docker) and the real Go API with
# test settings (OTP exposed, rate limits lifted), exports E2E_API, runs <cmd>
# from the repo root, then tears everything down.
#   E2E_PORT (default 18080), E2E_PG_PORT (default 55432), KEEP=1 leaves both running.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
port="${E2E_PORT:-18080}"
pg_port="${E2E_PG_PORT:-55432}"
pg="agri-e2e-pg-$$"
api_log="$(mktemp)"

cleanup() {
  if [[ "${KEEP:-0}" != 1 ]]; then
    [[ -n "${api_pid:-}" ]] && kill "$api_pid" 2>/dev/null || true
    docker rm -f "$pg" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

docker run -d --rm --name "$pg" -p "$pg_port:5432" \
  -e POSTGRES_USER=agri -e POSTGRES_PASSWORD=agri -e POSTGRES_DB=agrismart postgres:16-alpine >/dev/null
for _ in $(seq 60); do docker exec "$pg" pg_isready -U agri -d agrismart >/dev/null 2>&1 && break; sleep 1; done

export AGRI_DATABASE_URL="postgres://agri:agri@localhost:$pg_port/agrismart?sslmode=disable"
export AGRI_AUTH_JWT_SECRET="e2e-secret-e2e-secret-e2e-secret-0123"
export AGRI_STORAGE_LOCAL_SIGNING_KEY="e2e-signing-e2e-signing-e2e-signing-01"
export AGRI_STORAGE_LOCAL_DIR="$(mktemp -d)"
export AGRI_APP_ENV=test AGRI_APP_BASE_URL="http://localhost:$port" AGRI_HTTP_ADDR=":$port"
export AGRI_FEATURES_EXPOSE_OTP=true AGRI_FEATURES_GUEST_MODE=true
export AGRI_HTTP_RATE_LIMIT_PER_MINUTE=100000 AGRI_HTTP_AUTH_RATE_LIMIT_PER_MINUTE=100000
export AGRI_AUTH_OTP_PER_IP_PER_HOUR=100000 AGRI_OBSERVABILITY_LOG_LEVEL=warn

cd "$root/backend"
for _ in $(seq 30); do go run ./cmd/migrate up -config config.example.yaml >/dev/null 2>&1 && break; sleep 1; done
go run ./cmd/seed -config config.example.yaml >/dev/null
go build -o "$AGRI_STORAGE_LOCAL_DIR/api" ./cmd/api
"$AGRI_STORAGE_LOCAL_DIR/api" -config config.example.yaml >"$api_log" 2>&1 &
api_pid=$!
for _ in $(seq 60); do curl -fsS "http://localhost:$port/readyz" >/dev/null 2>&1 && break; sleep 0.5; done
curl -fsS "http://localhost:$port/readyz" >/dev/null || { cat "$api_log"; exit 1; }

export E2E_API="http://localhost:$port"
cd "$root"
if ! "$@"; then
  echo "--- api log ---"; tail -50 "$api_log"; exit 1
fi
