# Khomo Security

## API keys
- Per-project `X-API-Key`, looked up per request (SQLite index). Rotate by creating a project + deleting the old row.
- Never log or list keys: `GET /gateway/admin/projects` omits them; audit log stores metadata only.
- Generate 32+ random chars; one key per client surface (web/mobile/partner/internal).

## Admin exposure (biggest risk)
- Admin endpoints have **no auth by design** (local ops surface). Never expose raw: bind localhost, firewall, or front with mTLS/basic-auth at the reverse proxy.
- Every admin mutation auto-reloads the router + proxy pools — treat admin access as deploy access.

## PII & audit
- Request/response **bodies are never logged**. Audit keeps method, path (no query string), status, sizes, timings, UA (truncated), IP.
- `LOG_SAMPLE_RATE` (default 1.0) samples writes under load; queue drops warn instead of blocking.

## Edge hardening
- `MAX_REQUEST_SIZE_BYTES` (10 MB): oversize → `413` before proxying.
- Per-route `timeout_ms`: slow backends can't hold connections (server WriteTimeout 120s backstop).
- Hop-by-hop headers (`Connection`, `Upgrade`, `Transfer-Encoding`…) stripped before proxying.
- CORS is wide-open (`*`) for the challenge — restrict `Access-Control-Allow-Origin` in `cmd/khomo/main.go` before production.

## Transport
- Terminate TLS in front (Caddy/Nginx) — Khomo listens plain HTTP. Add mTLS gateway→backends when traffic leaves Cloudflare.

## Checklist
- [ ] Admin unreachable from the internet (verify: `curl <public>/gateway/admin/routes` times out)
- [ ] `.env` gitignored, keys 32+ chars, unique per project
- [ ] `LOG_REQUEST_BODY=false`, `LOG_RESPONSE_BODY=false`
- [ ] CORS restricted for production
- [ ] `LOG_LEVEL=info` (no debug header dumps)
