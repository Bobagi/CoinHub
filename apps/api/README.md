# apps/api — Coin Hub backend (Go)

The core of Coin Hub: a multi-user **JSON REST API + trading engine + auth**, one Go binary
(module `coin-hub`), built **distroless**. It serves no HTML — the UI is the Svelte SPA in
`apps/web`; this process only speaks JSON (cookie session) and runs the automation worker. nginx
fronts it on `127.0.0.1:5020`. (Formerly "Coin Alert", a single-user template-rendered app — the
legacy `server.go` + `templates/` were removed in the 2026-06 hardening pass.)

## Build & test (Go is not in PATH — use Docker)
```bash
# from apps/api
docker run --rm -v "$PWD":/app -w /app -e GOTOOLCHAIN=local golang:1.22-alpine \
  sh -c "go build ./... && go vet ./..."
```
`golang.org/x/crypto` is pinned to v0.31.0 (newer needs Go ≥1.25). Run/deploy via the repo-root
`docker compose` (db + migrate + api) or `./deploy.sh api`. Migrations run in the separate `migrate`
service before the app starts. Config is env-only (see repo-root `.env.example`); a missing
`CREDENTIALS_ENCRYPTION_KEY` disables credential storage, unset `GOOGLE_OAUTH_*`/`SMTP_*` disable
those features.

## Layout (`internal/`)
- `config` — env loading. `database` — Postgres connector (bounded pool). `domain` — structs/constants.
- `repository` — Postgres persistence; **everything is user-scoped** (`WHERE user_id = $1`).
- `service` — business logic: auth (bcrypt + opaque hashed sessions, step-up re-auth), Google OAuth,
  transactional email (`internal/email`), `UserCredentialService` (per-user Binance keys, AES-256-GCM
  at rest), `UserTradingService` (market buy + take-profit limit sell), `RobotService`,
  `AutomationWorker` (per-user reconcile + stop-loss + daily DCA, 30s/5min loops, single process),
  `AccessLogService` (+ offline `internal/geoip`), `AgreementService` (Terms/Privacy consent), shared
  Binance price cache + rate-limit gate.
- `httpserver` — JSON handlers: `auth_handler` (email + Google, password reset, email verify, step-up),
  `account_handler` (profile/password/delete/avatar/access-log/agreement), `api_handler`
  (settings/credentials/price/symbols), `operations_handler`, `robots_handler`, `portfolio_handler`.
  Money/robot endpoints are gated by `enforceVerifiedAndAgreed` (verified email **and** accepted Terms).

## What it does today
- **Multi-user auth:** email + password and **Google sign-in**; enforced email verification; password
  reset; new-device access log + alert emails; step-up ("sudo") re-auth for money actions.
- **Consent:** records server-side acceptance of the versioned Terms+Privacy (`user_agreement_acceptances`,
  migration 0027) and refuses money/robot actions without it.
- **Trading:** per-user encrypted Binance credentials with **testnet/production isolation**; manual buy
  + take-profit + manual close; **robots** (per-coin DCA + take-profit + optional stop-loss) executed by
  the automation worker; per-order + per-robot spending caps; only successful executions logged to history.
- **Portfolio:** proxies the Python scraper for the Investidor10 wallet (admin-gated).

See the repo-root `CLAUDE.md` for the full API surface, the worker concurrency model (do **not** run >1
replica without a leader lock), the Binance per-IP rate-limit notes, and the backlog.
