#!/usr/bin/env bash
# Seed Khomo with the 4 spec (§8) services via the admin API.
# Usage: BASE=http://localhost:4019 ./scripts/seed.sh
set -euo pipefail
BASE="${BASE:-http://localhost:4019}"

api() { curl -sf -X POST "$BASE$1" -H 'Content-Type: application/json' -d "$2"; echo; }

echo "— project"
api /gateway/admin/projects '{"name":"Primary Client","api_key":"client-khomo-key-2026","rate_limit_per_minute":600}' || true

echo "— routes"
api /gateway/admin/routes '{"path_prefix":"/api/auth","service_name":"AaaS","timeout_ms":10000,"max_retries":2,"strip_headers":["Cookie"]}' || true
api /gateway/admin/routes '{"path_prefix":"/api/notify","service_name":"Halla","timeout_ms":15000,"max_retries":2,"add_headers":{"X-Service-Caller":"khomo"}}' || true
api /gateway/admin/routes '{"path_prefix":"/api/payment","service_name":"Dobadoba","timeout_ms":30000,"max_retries":1}' || true
api /gateway/admin/routes '{"path_prefix":"/api/files","service_name":"FuM","timeout_ms":60000,"max_retries":2}' || true

echo "— backends (edit URLs to your real Workers)"
ROUTES=$(curl -sf "$BASE/gateway/admin/routes")
rid() { echo "$ROUTES" | python3 -c "import json,sys; rs=json.load(sys.stdin)['routes']; print([r['id'] for r in rs if r['path_prefix']=='$1'][0])"; }
api /gateway/admin/backends "{\"route_id\":$(rid /api/auth),\"base_url\":\"https://aas.emmanueljin101.workers.dev\"}" || true
api /gateway/admin/backends "{\"route_id\":$(rid /api/notify),\"base_url\":\"https://halla.emmanueljin101.workers.dev\"}" || true
api /gateway/admin/backends "{\"route_id\":$(rid /api/payment),\"base_url\":\"https://dobadoba.emmanueljin101.workers.dev\"}" || true
api /gateway/admin/backends "{\"route_id\":$(rid /api/files),\"base_url\":\"https://fum.emmanueljin101.workers.dev\"}" || true

echo "— done:"; curl -sf "$BASE/gateway/admin/routes"; echo
