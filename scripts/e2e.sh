#!/usr/bin/env bash
# e2e.sh — end-to-end suite across both apps: the Flutter client code
# (mobile/e2e) against the real Go API and Postgres.
set -euo pipefail
cd "$(dirname "$0")/.."
exec ./scripts/with-stack.sh bash -c 'cd mobile && flutter test e2e --dart-define=E2E_API="$E2E_API"'
