# Wi-Fi Man

A production-oriented Wi-Fi voucher/token platform for captive portal environments (MikroTik-style workflows), with:

- Token lifecycle management (generation, redemption, resume, revocation)
- Captive portal purchase and voucher redemption
- Embedded RADIUS auth + accounting server
- Data quota tracking and forced disconnects (CoA and HTTP fallback)
- Admin dashboard for plans and sales visibility
- Async background jobs, metrics, and log/metrics observability stack

## What This Project Contains

- `backend/` (Go + Gin): API, portal SSR pages, business services, RADIUS server, jobs, persistence
- `frontend-admin/` (React + TypeScript + Vite): lightweight admin console
- `infra/` (Nginx/Prometheus/Grafana/Loki/Promtail): reverse proxy and observability setup
- `docker-compose.yml`: full local stack orchestration

## High-Level Architecture

```text
Client Device
  -> Captive Portal (/portal)
  -> Redeem token OR buy package

Nginx (infra/nginx/default.conf)
  -> /admin/* -> frontend-admin (static app)
  -> /*       -> backend API + SSR portal

Backend (Go)
  - HTTP APIs (portal/admin/accounting/payments/gateway)
  - Services: token, plan, session, quota, payment, gateway integration, radius
  - Embedded RADIUS auth (1812/udp) + accounting (1813/udp)
  - Optional CoA disconnect client (3799/udp)
  - Optional async jobs (Asynq + Redis)
  - PostgreSQL for runtime state

Observability
  - Prometheus scrapes backend metrics
  - Loki + Promtail aggregates logs
  - Grafana dashboards over Prometheus/Loki
```

## How It Works (Detailed)

### 1. Startup Sequence

On boot (`backend/cmd/api/main.go`):

1. Loads config from environment (`backend/internal/config/config.go`).
2. Selects store backend:
- `postgres` (default): runs SQL migrations, opens connection pool.
- `memory`: in-memory store for quick tests.
3. Initializes Redis when rate limiting and/or Asynq is enabled.
4. Builds services in layers:
- `PlanService` seeds default plans if database is empty.
- `TokenService`, `SessionService`, `QuotaService`, `PaymentService`.
- `GatewayIntegrationService` with primary CoA controller and HTTP fallback controller.
5. Builds Gin router with middleware, portal/admin/payment/accounting endpoints.
6. Starts optional Asynq worker + scheduler.
7. Starts embedded RADIUS auth/accounting listeners when enabled.
8. Starts HTTP server and supports graceful shutdown.

### 2. Plan Management

- Plans define duration, quota, speed profile, device limit, and price.
- Admin endpoints:
- `GET /v1/admin/plans`
- `POST /v1/admin/plans`
- Support role is explicitly blocked from creating plans.

### 3. Token Lifecycle

Implemented primarily in `backend/internal/service/token_service.go`.

#### Generation

- Admin (or portal purchase flow) calls `GenerateTokens`.
- Plain token code is cryptographically generated, then hashed before storage.
- DB stores only `code_hash` (never plaintext token).
- Tokens are created as `unused`.

#### Redemption / Resume

- `RedeemToken` activates an `unused` token:
- Validates MAC address
- Binds token to first device MAC
- Sets `activated_at` and `expires_at`
- Creates active session
- Returns gateway policy (timeouts, speeds, quotas)
- `ResumeToken` allows reconnect only for already-active tokens on same device.
- Device MAC mismatch triggers denial (`token_device_bound`).

#### Revocation

- Admin can revoke token (`POST /v1/admin/tokens/:id/revoke`).
- Active session is marked ended and disconnect is requested via gateway integration.

#### Expiry Sweep

- `SweepExpiredTokens` marks expired active tokens appropriately.
- Triggered asynchronously via Asynq scheduler (`@every 1m` by default).

### 4. Session + Quota Accounting

Services: `session_service.go` + `quota_service.go`.

- Session tracks token usage context: MAC, IP, gateway, RADIUS IDs, byte counters.
- Interim accounting applies usage deltas and writes usage records.
- Quota logic compares cumulative bytes with plan `data_limit_mb`.
- On quota exceed:
- Session ends with `data_limit_reached`
- Token becomes `consumed`
- Disconnect is requested from gateway controller

### 5. Portal Purchase Flow

Handler: `backend/internal/http/handlers/portal_handler.go`

`POST /v1/portal/purchase` and SSR form flow (`POST /portal/purchase`) do:

1. Validate/select device MAC and gateway id.
2. Load selected plan.
3. Record payment (`PaymentService.RecordManualPayment`) with generated transaction ref.
4. Generate one token.
5. Immediately redeem token for same device.
6. Return session + policy (JSON) or portal success fragment (HTMX).

Result: customer can pay and connect in one action.

### 6. RADIUS Authentication + Accounting

Files: `radius_server.go`, `radius_service.go`, `gateway_client.go`.

#### Auth (UDP 1812)

- Receives Access-Request.
- Uses `User-Name` as token code and `Calling-Station-Id` as device MAC.
- Attempts resume first, then redeem.
- Returns Access-Accept with:
- Session timeout
- MikroTik rate-limit VSA (or computed fallback)
- Reply message
- Returns Access-Reject on failure.

#### Accounting (UDP 1813)

- Validates packet authenticator.
- Handles start/interim/stop accounting packets.
- Maps counters to per-session byte deltas.
- Invokes quota checks and may trigger disconnect.

### 7. Gateway Disconnect Strategy

`GatewayIntegrationService` supports controller fallback:

1. Primary: RADIUS CoA/Disconnect (UDP, usually port 3799)
2. Fallback: HTTP callback (`GATEWAY_DISCONNECT_URL`)

If disconnect fails and async jobs are enabled, request is queued for retry via Asynq.

### 8. Payments

`payment_service.go`

- Manual record endpoint: `POST /v1/payments/manual/record`
- Mobile money webhook: `POST /v1/payments/mobilemoney/webhook`
- Webhook can validate HMAC (`X-Signature`) when `MOBILE_MONEY_SECRET` is set.
- Idempotency by unique `transaction_ref`.

### 9. Admin Panel

`frontend-admin/src/main.tsx`

Capabilities:

- Switch role header (`super_admin`, `cashier`, `support`)
- View plans and sales snapshot
- Create new plans (except support role)
- Quick links to customer portal and health endpoint

Nginx serves it under `/admin/`.

### 10. Captive Portal UI

Templates: `backend/templates/portal.tmpl`, `plan_cards.tmpl`

- Server-rendered HTML + HTMX for incremental interactions.
- Auto-refreshes plan cards every 5 seconds.
- Supports MAC extraction from query params/headers and localStorage persistence.
- Preserves MikroTik login params (`link-login-only`, `dst`, `popup`) for auto-login handoff.

## Data Model

Primary tables (see `backend/migrations/*.sql`):

- `plans`
- `tokens`
- `sessions`
- `usage_records`
- `payments`
- `audit_logs`

Important state fields:

- Token status: `created | unused | active | expired | consumed | revoked`
- Session status: `active | ended`
- Payment status: `pending | paid | failed`

Extra RADIUS-aware session fields:

- `radius_session_id`
- `nas_ip`
- `nas_identifier`
- `disconnect_pending`

## HTTP API Surface

### Portal

- `GET /portal` (SSR page)
- `POST /portal/redeem` (HTMX fragment)
- `POST /portal/purchase` (HTMX fragment)
- `GET /portal/plans/fragment`
- `POST /v1/portal/redeem`
- `POST /v1/portal/resume`
- `GET /v1/portal/plans`
- `POST /v1/portal/purchase`

### Admin (RBAC protected)

- `GET /v1/admin/plans`
- `POST /v1/admin/plans`
- `POST /v1/admin/tokens/generate`
- `POST /v1/admin/tokens/:id/revoke`
- `GET /v1/admin/sessions/active`
- `GET /v1/admin/reports/sales`

### Accounting / Gateway / Payments

- `POST /v1/accounting/start`
- `POST /v1/accounting/interim`
- `POST /v1/accounting/stop`
- `POST /v1/gateway/disconnect`
- `POST /v1/payments/mobilemoney/webhook`
- `POST /v1/payments/manual/record`

### Ops

- `GET /healthz`
- `GET /metrics` (when enabled)

## RBAC Model

By default, admin routes read role from `X-Role` header (`ADMIN_TOKEN_HEADER`):

- `super_admin`
- `cashier`
- `support`

All can read admin data; `support` cannot create plans.

## Configuration

Use `.env.example` as baseline. Key variables:

- Core: `HTTP_ADDR`, `STORE_BACKEND`, `DB_DSN`, `MIGRATIONS_PATH`
- Redis/Jobs: `REDIS_ADDR`, `REDIS_PASSWORD`, `ASYNQ_*`
- Rate limit: `RATE_LIMIT_*`
- Gateway: `GATEWAY_DISCONNECT_URL`, `GATEWAY_AUTH_TOKEN`
- RADIUS: `RADIUS_ENABLED`, `RADIUS_AUTH_ADDR`, `RADIUS_ACCT_ADDR`, `RADIUS_SECRET`
- CoA: `RADIUS_COA_ADDR`, `RADIUS_COA_SECRET`
- RouterOS fallback: `ROUTEROS_API_URL`, `ROUTEROS_API_TOKEN`
- Payments: `MOBILE_MONEY_SECRET`
- UI/Auth header: `PORTAL_TITLE`, `ADMIN_TOKEN_HEADER`

## Local Development

### Option A: Run backend + frontend separately

```bash
cd backend
go mod tidy
go run ./cmd/api
```

```bash
cd frontend-admin
npm install
npm run dev
```

Backend default: `http://localhost:8080`

### Option B: Full stack (recommended)

```bash
docker compose up --build
```

Then open:

- Portal: `http://localhost/portal`
- Admin: `http://localhost/admin/`
- Health: `http://localhost/healthz`
- Metrics: `http://localhost/metrics`
- Grafana: `http://localhost:3000`
- Prometheus: `http://localhost:9090`

## MikroTik Integration Notes

Typical setup:

```text
/ip hotspot profile set [ find default=yes ] use-radius=yes
/radius add address=<BACKEND_IP> service=hotspot secret=<RADIUS_SECRET> authentication-port=1812 accounting-port=1813
/radius incoming set accept=yes port=3799
/ip hotspot profile set [ find default=yes ] interim-update=1m
```

For CoA disconnects, configure backend `RADIUS_COA_ADDR` and `RADIUS_COA_SECRET`.

## Security and Operational Notes

- Token plaintext is never stored, only hash.
- Webhook supports signature verification.
- Rate limiting can be enabled for portal and payment webhook paths.
- Keep production secrets out of committed files; use environment injection.
- If you expose RADIUS publicly, restrict source IPs at network/firewall level.

## Useful Test Calls

```bash
curl -H "X-Role: super_admin" http://localhost/v1/admin/plans
```

```bash
curl -X POST http://localhost/v1/admin/plans \
  -H "X-Role: super_admin" \
  -H "Content-Type: application/json" \
  -d '{"name":"4 Hour Pro","duration_minutes":240,"data_limit_mb":3072,"speed_down_kbps":6000,"speed_up_kbps":2500,"device_limit":1,"price":3.75,"active":true}'
```

```bash
curl -X POST http://localhost/v1/portal/redeem \
  -H "Content-Type: application/json" \
  -d '{"token":"<TOKEN_CODE>","device_mac":"AA:BB:CC:DD:EE:FF","ip":"192.168.10.23","gateway_id":"gw-1"}'
```

## Project Layout

```text
.
├── backend/
│   ├── cmd/api/                 # backend entrypoint
│   ├── internal/
│   │   ├── config/              # env config
│   │   ├── domain/              # core models/status enums
│   │   ├── http/                # gin router, middleware, handlers
│   │   ├── jobs/                # asynq worker/scheduler/tasks
│   │   ├── repository/          # postgres + memory stores
│   │   └── service/             # business logic + radius/gateway
│   ├── migrations/              # database schema
│   ├── templates/               # captive portal templates
│   └── web/static/              # portal CSS
├── frontend-admin/              # react admin UI
├── infra/                       # nginx, prometheus, loki, grafana, promtail
└── docker-compose.yml           # local stack
```

## License

No license file is currently included. Add one before public/open distribution.
