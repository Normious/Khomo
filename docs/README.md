# Khomo docs

Mini API gateway — one endpoint in front of all 30 services.

**Local:** `http://localhost:4019` — `GET /health` → `{"status":"ok","version":"1.0.0"}`

| Doc | Covers |
|---|---|
| [Interactive Architecture](khomo-architecture.html) | Pan/zoom/trace diagram + light/dark + PNG export |
| [API.md](API.md) | Full endpoint spec with examples |
| [openapi.yaml](openapi.yaml) | OpenAPI 3.0.3 (fixed surface: health/admin/stats) |
| [DEPLOYMENT.md](DEPLOYMENT.md) | Local + Docker + VPS, 7-step |
| [INTEGRATION.md](INTEGRATION.md) | Clients + service-to-service + adding services |
| [SECURITY.md](SECURITY.md) | Keys, PII, limits, admin exposure |

Pipeline: `X-Request-ID` → longest-prefix match → `X-API-Key` auth → rate limit → header transforms → proxy (timeout + 5xx retries) → async audit log.
