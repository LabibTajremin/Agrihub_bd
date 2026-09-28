#!/usr/bin/env bash
# loadtest.sh — vegeta baseline on the five hottest endpoints against the real
# API (run via: ./scripts/with-stack.sh ./scripts/loadtest.sh, or make loadtest).
#   RATE (req/s, default 200), DURATION (default 15s)
set -euo pipefail
api="${E2E_API:?run through scripts/with-stack.sh}"
rate="${RATE:-200}"
duration="${DURATION:-15s}"
vegeta="${VEGETA:-$(go env GOPATH)/bin/vegeta}"
work="$(mktemp -d)"
json() { python3 -c "import json,sys; print(json.load(sys.stdin)$1)"; }

phone="+88017$(printf '%08d' $((RANDOM * RANDOM % 100000000)))"
code=$(curl -fsS -X POST "$api/v1/auth/otp/request" -d "{\"phone\":\"$phone\"}" | json "['dev_code']")
token=$(curl -fsS -X POST "$api/v1/auth/otp/verify" -d "{\"phone\":\"$phone\",\"code\":\"$code\"}" | json "['access_token']")
auth="Authorization: Bearer $token"
field=$(curl -fsS -X POST "$api/v1/fields" -H "$auth" -d '{"name":"Load","area":{"value_milli":1500,"unit":"bigha"},"location":{"lat":24.85,"lng":89.37},"irrigation":"partial","soil":{"texture":"loam","ph":6.5}}' | json "['id']")
echo '{"lang":"en","transcript":"will it rain this week?"}' >"$work/ask.json"

declare -A targets=(
  [i18n]="GET $api/v1/i18n/bn"
  [weather]="GET $api/v1/weather?lat=24.85&lng=89.37"
  [scans]="GET $api/v1/scans?limit=20"
  [recommendations]="GET $api/v1/advisory/fields/$field/recommendations"
  [assistant]="POST $api/v1/assistant/ask"
)
printf '| endpoint | rate | requests | success | p50 | p95 | p99 | max |\n|---|---|---|---|---|---|---|---|\n'
for name in i18n weather scans recommendations assistant; do
  {
    echo "${targets[$name]}"
    echo "$auth"
    [[ $name == assistant ]] && { echo "Content-Type: application/json"; echo "@$work/ask.json"; }
    echo
  } >"$work/$name.txt"
  "$vegeta" attack -targets="$work/$name.txt" -rate="$rate" -duration="$duration" >"$work/$name.bin"
  "$vegeta" report -type=json <"$work/$name.bin" | python3 -c "
import json,sys
r=json.load(sys.stdin); l=r['latencies']; ms=lambda n: f'{n/1e6:.1f} ms'
print(f\"| $name | $rate/s | {r['requests']} | {r['success']*100:.2f}% | {ms(l['50th'])} | {ms(l['95th'])} | {ms(l['99th'])} | {ms(l['max'])} |\")"
done
