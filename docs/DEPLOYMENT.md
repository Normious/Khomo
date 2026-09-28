# Khomo Deployment

## 1. Local (7-step)

```bash
go mod download                                   # 1. deps
cp .env.example .env                              # 2. config (edit PORT, CB_*)
go run ./cmd/khomo                                # 3. run → routes=0
BASE=http://localhost:4019 ./scripts/seed.sh          # 4. seed 4 services
curl http://localhost:4019/health                   # 5. routes: 4, healthy: 4
curl -X POST localhost:4019/api/auth/register \   # 6. proxy smoke test
  -H "X-API-Key: client-khomo-key-2026" \
  -H "Content-Type: application/json" -d '{"email":"t@e.com"}'
curl -X POST localhost:4019/api/auth/register \   # 7. 401 without key
  -H "Content-Type: application/json" -d '{}'
```

Release binary: `CGO_ENABLED=0 go build -ldflags="-s -w" -o khomo ./cmd/khomo` (~8 MB) → `./khomo`.

## 2. Docker

```bash
docker compose up -d
BASE=http://localhost:4019 ./scripts/seed.sh
curl http://localhost:4019/health
```

Image is `FROM scratch` (~10 MB): binary + `/migrations` + CA certs. Migration auto-applies on boot, including from `/migrations` inside the image. Data lives in the `khomo_data` volume (`DATABASE_PATH=/data/khomo.db`), capped at 128M RAM.

## 3. VPS (binary)

```bash
scp khomo .env user@host:/opt/khomo/
ssh user@host "mkdir -p /opt/khomo/data && cd /opt/khomo && DATABASE_PATH=/opt/khomo/data/khomo.db ./khomo"
```

Front with Caddy/Nginx for TLS, e.g. `api.yourdomain.com → 127.0.0.1:4019`. Keep `/gateway/admin/*` on localhost (firewall or `bind 127.0.0.1` on a second listener — see SECURITY.md).

## Env reference

Defaults live in `.env.example`. Hot paths: `PORT`, `DATABASE_PATH`, `LOG_LEVEL`, `CB_FAILURE_THRESHOLD` (5), `CB_TIMEOUT_SECONDS` (30), `HEALTH_CHECK_INTERVAL_SECONDS` (30), `LOG_SAMPLE_RATE` (1.0), `MAX_REQUEST_SIZE_BYTES` (10 MB). Full table in README + `internal/config/config.go`.

## Checklist

- [ ] `.env` set, `LOG_LEVEL=info`
- [ ] Migration applied (check boot logs, no `migrate:` error)
- [ ] Seed run with real Workers URLs
- [ ] `/health` shows `routes: 4, backends_healthy: 4`
- [ ] Proxy + 401 smoke tests pass
- [ ] Admin not publicly reachable
