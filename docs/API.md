# Khomo API

Base: `http://localhost:4019`. Gateway paths are dynamic (registered via admin); everything else is fixed. All JSON unless proxied.

## Gateway (dynamic)

Any path matching a registered `path_prefix` is proxied. Non-matching paths → `404`.

```
GET|POST|PUT|PATCH|DELETE /<route-prefix>/<rest>
X-API-Key: <key>              # required when route.require_auth = 1
```

**Request pipeline:** `X-Request-ID` assigned if absent → longest-prefix match (`404`) → auth (`401`) → token bucket (`429`) → inject `X-Request-ID`, `X-Gateway: Khomo/1.0`, `X-Forwarded-For/Host/Proto`, `X-Project` → per-route add/strip headers → `ReverseProxy` with per-route timeout → retry next backend on 5xx → async audit log.

**Upstream sees:** prefix stripped (`/api/auth/register` → `/register`), `Cookie` stripped on `/api/auth/*` (configurable per route).

**Response gains:** `X-Request-ID`, `X-Gateway`, `X-Backend`.

**Gateway codes:** `401` missing/invalid key · `404` no route · `413` body over limit · `429` rate limited · `502` backend 5xx exhausted · `503` all backends circuit-open.

## `GET /health`

No auth. `200`:
```json
{ "service": "Khomo — API Gateway", "version": "1.0.0", "language": "Go",
  "status": "ok", "routes": 4, "backends_total": 4, "backends_healthy": 4,
  "last_reload": "2026-09-28T11:30:05Z", "timestamp": "2026-09-28T09:30:08Z" }
```

## Admin (no auth — localhost only)

### `GET /gateway/admin/routes`
`200` → `{ success, count, routes: [{ id, path_prefix, service_name, strip_prefix, require_auth, timeout_ms, max_retries, websocket, backend_list: [{ id, base_url, weight }], backends }] }`

### `POST /gateway/admin/routes`
```json
{ "path_prefix": "/api/pay", "service_name": "Dobadoba",
  "strip_prefix": true, "require_auth": true,
  "timeout_ms": 30000, "max_retries": 1,
  "add_headers": {"X-Service-Caller": "khomo"}, "strip_headers": [],
  "websocket": false }
```
Only `path_prefix` + `service_name` required (defaults: strip/auth on, 30s, 0 retries). `200` reloads. `400` bad body · `409` prefix exists.

### `DELETE /gateway/admin/routes/{id}`
Cascades to backends, then reloads. `200` · `400` bad id.

### `POST /gateway/admin/backends`
```json
{ "route_id": 3, "base_url": "https://dobadoba.emmanueljin101.workers.dev", "weight": 1 }
```
`route_id` + `base_url` required. `200` reloads. `400` bad body.

### `DELETE /gateway/admin/backends/{id}`
`200` reloads · `400` bad id.

### `POST /gateway/admin/projects`
```json
{ "name": "Mobile App", "api_key": "<random-32+>", "rate_limit_per_minute": 600 }
```
`201` → `{ success, id }` · `400` missing fields · `409` key exists.

### `GET /gateway/admin/projects`
`200` → `{ success, count, projects: [{ id, name, rate_limit_per_minute, is_active }] }` (keys never listed).

### `POST /gateway/admin/reload`
Re-reads DB, re-syncs proxy pools without restart. `200` → `{ success, reloaded_at }`.

## `GET /gateway/stats?days=30` (needs `X-API-Key`)

`200`:
```json
{ "success": true, "project": "Primary Client", "days": 30,
  "daily": [{ "date": "2026-09-28", "total_requests": 16, "success_2xx": 8,
    "client_errors_4xx": 0, "server_errors_5xx": 8, "gateway_errors": 7,
    "avg_duration_ms": 0.875, "total_bytes_out": 1678, "retries": 2 }],
  "totals": { "...same keys summed..." },
  "top_routes": { "/api/auth": { "count": 7, "avg_duration_ms": 1.57 } } }
```
`days` defaults 30, max 365. `401` bad key.
