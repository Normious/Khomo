# Khomo Integration

For Day 29+ services and all clients. Rule: **clients and services talk to Khomo, never to backends directly.**

## Clients (one base URL)

```javascript
// Before: 30 hosts. After: one.
const API = 'https://api.yourdomain.com';

await fetch(`${API}/api/auth/register`, {
  method: 'POST',
  headers: { 'X-API-Key': USER_API_KEY, 'Content-Type': 'application/json' },
  body: JSON.stringify({ email, password }),
});

// Trace across services:
const rid = res.headers.get('X-Request-ID'); // same ID reaches the backend
```

## Service-to-service (through Khomo)

```typescript
// In Dobadoba — needs Halla? Go through the gateway:
await fetch('https://api.yourdomain.com/api/notify/send', {
  method: 'POST',
  headers: { 'X-API-Key': env.INTERNAL_API_KEY, 'Content-Type': 'application/json' },
  body: JSON.stringify({ to: customerPhone, message: '...' }),
});
// Central rate limiting + circuit breaking (Halla down → fast 503) + one audit trail.
// Backends can read X-Project to know which tenant called.
```

## Adding a service (no client changes, no restart)

```bash
B=http://localhost:4019
curl -X POST $B/gateway/admin/routes -H 'Content-Type: application/json' \
  -d '{"path_prefix":"/api/pay","service_name":"Dobadoba","timeout_ms":30000,"max_retries":1}'
# → note id, then:
curl -X POST $B/gateway/admin/backends -H 'Content-Type: application/json' \
  -d '{"route_id":<id>,"base_url":"https://dobadoba.emmanueljin101.workers.dev"}'
curl $B/gateway/admin/routes   # verify backends: 1
```

Rules: prefix must be unique (`/api/pay` ≠ `/api/payments` boundary-safe — match needs `/` or exact). Longer prefix wins, so `/api/auth/admin` overrides `/api/auth`. `strip_prefix: true` (default) keeps backends gateway-agnostic.

## Per-project keys

```bash
curl -X POST $B/gateway/admin/projects -H 'Content-Type: application/json' \
  -d '{"name":"Mobile App","api_key":"<random-32+>","rate_limit_per_minute":600}'
```

Give web/mobile/partner each a key: independent buckets, per-project stats at `GET /gateway/stats`.
