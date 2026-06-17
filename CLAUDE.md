# CLAUDE.md — Coin Hub

Guidance for Claude Code (and humans) working in this repo. Read this first; it is the source of
truth so you don't have to re-derive the project each session.

## What this is

**Coin Hub** is a multi-user personal investing app served at **https://coin.bobagi.space**. It
merges two former projects into one repo:
- **Crypto** (was `Bobagi/Coin-Alert`, Go): connect Binance, log/automate trades — market buy +
  take-profit limit sell, daily DCA, stop-loss, price alerts.
- **B3 portfolio** (was `Bobagi/investidor10`, Python): read an Investidor10 public wallet to show
  stocks/FIIs and upcoming ex-dividend (data-com) dates.

Owner: Gustavo Perin ("Bobagi"). Brand palette is **warm dark + gold** (`#ffd43b` / `#fab005`,
text `#fff9db`) to match his other sites; UI is trilingual (pt-BR/en/es, auto-detected).

## Repo layout (monorepo)

```
apps/api/      Go backend: trading engine + REST API + auth (the core). Module `coin-alert`.
apps/web/      Svelte + Vite SPA (TypeScript). Builds to apps/web/dist (served by nginx).
apps/scraper/  Python/Flask + Selenium scraper for Investidor10 (internal-only service).
migrations/    golang-migrate SQL (0001..NNNN), applied by the compose `migrate` service.
deploy/nginx/  Reference copy of the live vhost.
docker-compose.yml   db + migrate + api (+ scraper under the `scraper` profile).
.env           Real secrets (gitignored, chmod 600). Copy from .env.example.
```

### apps/api internals (Go, SOLID-ish layering)
`cmd/server/main.go` wires everything. `internal/`:
- `config` — env loading.  `database` — Postgres connector.  `domain` — structs/constants.
- `repository` — Postgres persistence; **everything is user-scoped** (`WHERE user_id = $1`).
- `service` — business logic: auth (bcrypt + sessions), `UserCredentialService` (per-user Binance
  keys, AES-256-GCM at rest), `UserTradingService` (buy = market + take-profit limit sell),
  `AutomationWorker` (per-user reconcile + stop-loss + daily DCA, 30s poll), Binance REST clients,
  `PortfolioScraperClient`.
- `httpserver` — JSON handlers: `auth_handler` (email + Google OAuth), `account_handler`
  (profile/password/delete), `api_handler` (settings/credentials/price/symbols),
  `operations_handler`, `portfolio_handler`. (The legacy single-user `server.go` + HTML `templates/`
  were removed in the 2026-06 hardening pass; the legacy single-user *services* it used remain but
  are unwired/dead.) Google OAuth lives in `service/google_oauth_service.go` (stdlib
  only, no extra module); it is **config-driven** — unset `GOOGLE_OAUTH_*` ⇒ feature off & button hidden.

## API surface (cookie-authenticated except signup/login/providers and the Google redirect flow)
`/auth/{signup,login,logout,me,providers}` · `/auth/google/{login,callback}` · `/api/v1/settings` (GET/PUT) ·
`/api/v1/binance/{credentials,credentials/activate,price,symbols,symbol-filters,klines,open-orders}` ·
`/api/v1/operations` (GET list / POST buy) · `/api/v1/operations/sell` (POST close-now) · `/api/v1/operations/place-sell` (POST (re)place take-profit) · `/api/v1/operations/executions` ·
`/api/v1/portfolio/{source,assets,dividends}` ·
`/api/v1/account/profile` (PUT) · `/api/v1/account/password` (POST) · `/api/v1/account` (DELETE) · `/health`.
Sessions = opaque random token in a Secure httpOnly cookie (`coin_hub_session`); only its SHA-256
hash is stored.

## Build & run (IMPORTANT gotchas)

- **Go is NOT in PATH.** Build/test via Docker:
  `docker run --rm -v "$PWD":/app -w /app -e GOTOOLCHAIN=local golang:1.22-alpine sh -c "go build ./... && go vet ./..."`
  (run from `apps/api`). `golang.org/x/crypto` is **pinned to v0.31.0** (newer needs Go ≥1.25).
- **Frontend:** Node 18 + pnpm 9 via nvm. `cd apps/web && export PATH="$HOME/.nvm/versions/node/v18.20.5/bin:$PATH" && pnpm install && pnpm build`. nginx serves `dist/` directly, so after `pnpm build` the new UI is live (no container/nginx reload needed). `package.json` has `pnpm.onlyBuiltDependencies:["esbuild"]` so the build script runs.
- **Edit `.svelte` source lives in `apps/web/src/lib/`** — the repo-root `.gitignore` ignores `lib/`,
  so `apps/web/.gitignore` re-includes it (`!src/lib/`). Don't remove that or the UI source stops
  being committed.

## Deploy (production, on the VPS)

```bash
cp .env.example .env   # first time; fill DB_PASSWORD + CREDENTIALS_ENCRYPTION_KEY (openssl rand -base64 32)
docker compose up -d --build                    # db + migrate + api
docker compose --profile scraper up -d --build  # also build/start the scraper
cd apps/web && pnpm build                        # rebuild the SPA nginx serves
```
- Compose project name **`coin-hub`**: `coin-hub-db-1`, `coin-hub-api-1`, `coin-hub-scraper-1`
  (all `restart: always`). API listens on **127.0.0.1:5020** only; nginx fronts it.
- DB is **internal-only** (no host port). Volume `coin-hub_db_data`.
- nginx vhost: `/etc/nginx/sites-available/coin.bobagi.space` (TLS via certbot) serves
  `/opt/Coin-Alert/apps/web/dist` and proxies `/api`,`/auth`,`/health` → :5020. After edits:
  `nginx -t && systemctl reload nginx`.
- **`CREDENTIALS_ENCRYPTION_KEY` must stay stable** — regenerating it makes stored Binance secrets
  undecryptable. Never print/commit `.env`.
- `apps/api` runs on **distroless** (no shell): debug via `docker logs coin-hub-api-1`, not `exec`.

## Conventions
- Descriptive English identifiers (functions/vars), even when chatting in PT.
- Migrations are **additive** and versioned; the app enforces user scoping in code.
- **Testnet-first**: new users default to TESTNET; live (PRODUCTION) orders are refused unless the
  user set `live_trading_enabled`. Recommend trade-only Binance keys (no withdrawal).
- i18n: `apps/web/src/lib/i18n.ts` (dictionaries en/pt/es + `t` store + auto-detect). Add UI strings
  there, not inline.

## Status (2026-06)
Done & live: monorepo unification; multi-user auth (email + **Google OAuth**, migration 0009 makes
`password_hash` nullable + adds `google_subject`); **account settings page** (edit name, set/change
password, language, delete account — cascades via the user FKs); per-user encrypted Binance creds;
settings (incl. **daily-buy on/off toggle** `daily_purchase_enabled`, migration 0010); operations
(manual buy + take-profit + **manual close-now** `CloseOperationNow` + **(re)place take-profit**
`PlaceTakeProfitForOperation`; orders snap price→tickSize, qty→stepSize and pre-check minNotional via
`FetchSymbolFilters`, so -1013 PRICE_FILTER/NOTIONAL become clear messages); **per-environment isolation**
(migration 0011: `binance_environment` tags operations/executions and is part of the
`user_trading_settings` composite PK — listings, the worker and settings all scope to the user's active
environment via `UserCredentialService.ActiveEnvironmentName`); automation worker (reconcile + stop-loss
+ daily DCA, skipped when the toggle is off; also **detects external take-profit cancellation**
(Binance status CANCELED → operation CANCELED/released, dropped from Positions) and enforces an
**app-side sell-order validity** — migration 0013 `sell_order_validity_days` (0=GTC) +
`sell_order_expires_at`; on expiry it cancels the order and leaves the position ⚠ to re-place/sell);
only **successful** executions are logged to history (failed attempts surface live + as ⚠, not as
0/0/0 rows); Svelte SPA with a **design system** (rem type scale +
spacing tokens in `app.css`, sticky `TopNav`, **SVG flags** in `LanguageDropdown` — emoji flags break on
Windows, hash router in `stores.ts`), a **3-tab dashboard** (Binance connection [default] / Trade /
B3-Investidor10) with an **environment switcher** (buttons; selecting activates + reloads) + **symbol
autocomplete** (`SymbolAutocomplete`, via `/binance/symbols`), a **bot-status panel** with an on/off
button + **local-timezone** daily-buy picker, a rich **`AllocationPanel`** (wallet donut by current
value + total + legend on the left; selected-coin header with period change badge, value, a
price-history line chart, 24h/7d/1M/3M tabs and coin pills on the right — holdings × current price,
history via `/binance/klines`), an **operations history sub-tab** (executions, for auditing — with a **By** column showing who acted,
`initiated_by` USER/BOT, migration 0012; the take-profit is GTC/no-expiry, shown in the Sell-order column), a **non-custodial disclaimer/ToS** (`LegalFooter`),
explanations, gold theme, favicon, i18n; portfolio scraper integration. (Outstanding work is consolidated
in the **TODO / backlog** section below.) The old standalone `investidor10` container (:3054) +
its `investidor10.bobagi.space` vhost were **decommissioned** in the 2026-06 hardening pass (compose
project at `/opt/investidor10` left on disk + the vhost kept in `sites-available`, so it is reversible).

### 2026-06 robots + admin (multi-bot model)
- **Admin role** (migration 0014 `users.is_admin`, owner seeded): admins access the **B3 tab** (the
  whole portfolio API is now admin-gated, 403 otherwise) and get unlimited robots. Exposed on `/auth/me`.
- **Trading robots** (migration 0015 `trading_robots`): a "robot" is one automated bot per coin/pair
  per environment (`/api/v1/robots` GET/POST · `/robots/update` · `/robots/delete`,
  `service.RobotService`). It **replaced the single per-environment automation**: the `AutomationWorker`
  now iterates robots (per-coin daily DCA with **per-symbol** idempotency, per-robot stop-loss) instead
  of one `user_trading_settings` row. Existing settings rows were seeded into one robot each, preserving
  behavior. `user_trading_settings` now only carries account-level bits used by the worker’s gate
  (`live_trading_enabled`) + manual-buy defaults; its other fields are no longer read by automation.
  **Standard users: 1 robot per environment; admins unlimited** (`StandardUserRobotLimitPerEnvironment`,
  monetization hook — payment not built yet).
- UI: the **Trade tab is first/default**; the bot-settings panel is hidden behind a **Robots list**
  (create → click to edit). History now distinguishes a placed take-profit (`SELL_ORDER_PLACED`, blue)
  from a completed sale (`SELL`/“Sold”, green). **Google sign-in is live** (`GOOGLE_OAUTH_*` set in
  `.env`); the OAuth consent screen is in *Testing*, so test users must be allow-listed in Google Cloud.
  `AuthenticateWithGoogle` auto-links by verified email (a manual account keeps its password).

### Lock UI + environment guarantees (2026-06)
- The **Trade** tab is **locked** (padlock left of the name, dimmed content + `LockOverlay` alert) until
  the user has a configured Binance environment — the Connection tab stays open (it's where you set it
  up). The **B3** tab is shown to everyone but **locked for non-admins** ("under construction"), not hidden.
- DB enforces an environment on every trade: migration 0017 makes `binance_environment` NOT NULL +
  CHECK `IN ('TESTNET','PRODUCTION')` on `trading_operations` and `trading_operation_executions`.
- **Account deletion is a privacy-preserving hard delete** (migration 0018): all PII + encrypted keys are
  erased via cascade, but one non-identifying row is written to `account_deletion_audit` — a keyed
  **HMAC email fingerprint** (irreversible without the server key, never the raw email) + `auth_method`,
  `account_created_at`, `had_binance_credentials`, `operation_count`. Best-effort, never blocks deletion.
- **Transactional email** (integrated, not a separate service): `internal/email` (`Sender` iface + Gmail
  SMTP impl via `SMTP_*` env, no-op when unset). **Password reset** + **email verification** flows
  (migration 0019 `auth_tokens` storing only the token hash, like sessions; `users.email_verified_at`,
  existing users grandfathered verified, Google sign-ups pre-verified). Endpoints `/auth/password/forgot`,
  `/auth/password/reset` (revokes all sessions), `/auth/email/verify`, `/auth/email/resend`; SPA pages
  `#/reset` + `#/verify`. **Email verification is enforced**: the API returns 403 `code:email_unverified`
  on connect-Binance / trade / robot endpoints (`enforceEmailVerified`); unverified users may browse
  but every save is blocked and the SPA shows a **styled global modal** (`AppModal`, driven by the
  `appModal` store — also replaces the old `window.alert` in `LockOverlay`) plus a reminder banner.
  Sending is live (`SMTP_PASSWORD` set with a Gmail App Password); emails carry
  `Reply-To`/`Message-ID`/`Auto-Submitted` headers. If SMTP is unset, `Sender` is a no-op,
  `/auth/providers` reports `email:false`, and verification is not enforced.
- The **Rentabilidade** sub-tab (in the collapsible "Positions & performance" card) has a
  `ProfitabilityPanel` chart: the selected coin's price line vs a dashed **average-buy-price** line, an
  "if you sell everything now" unrealized P/L header, and per-coin P/L pills (green/red).

### Binance IP rate limit (multi-user scaling)
Binance enforces request weight **per IP**, not per key, and all users' traffic egresses from this one
VPS — so the IP weight ceiling (~6000/min spot) is the first wall as the user base grows. Mitigations:
- **Done:** a **shared process-wide price cache** (`binance_price_service.go`, 5s TTL keyed by env+symbol)
  so N users holding the same coin = 1 ticker call per window, not N; and a **shared rate-limit gate**
  (`binance_rate_limiter.go`): every Binance REST client is built with `newBinanceHTTPClient`, whose
  `RoundTripper` reads `X-MBX-USED-WEIGHT-1M` (logs a warning past ~80%) and, on **429/418**, parks ALL
  Binance requests until `Retry-After` — backing off as one IP.
- **TODO (bigger, architectural — do these to actually scale):**
  - **WebSocket user data stream** (per-user `listenKey`, 30-min keepalive): have Binance *push* order
    fills/cancellations instead of the worker polling `GetOrderStatus` every 30s. This removes the bulk of
    the REST weight (the take-profit is already a resting limit order, so polling only exists to reconcile).
  - **WebSocket market price stream** (one combined ticker stream for all symbols) to replace REST ticker
    polling that feeds stop-loss; pairs with the price cache above.
  - Only after those: consider **multiple egress IPs / proxy sharding** (the limit is per IP) and a
    **leader lock** before running >1 API replica (see worker single-process constraint).

### 2026-06 session (history accuracy, multi-user hardening, pagination)
- **Daily-buy history was double-logged**: the shared buy path always wrote a `BUY` row and the daily
  DCA added a second `DAILY_BUY` row (same Binance order id), so bot buys showed as both "Compra" and
  "Compra diária". Fixed: `executeBuyWithType` records the buy leg **once** — `BUY` for manual buys,
  `DAILY_BUY` for the daily DCA. Migration **0022** removed the legacy duplicate `BUY` rows (a `BUY`
  whose order_id+user+env matched an existing `DAILY_BUY`). The Profitability "spent by robots" split now
  counts `DAILY_BUY` (+ legacy `BUY`).
- **Positions sub-tab**: new **"Target profit"** column (`ops.targetPct`) showing each operation's
  configured `target_profit_percent` next to the Target (price) column.
- **Allocation total** has a **"?" help tooltip** (`alloc.walletTotalHelp`) explaining it is current
  market value (holdings × price) vs the Profitability "still invested" (cost basis); the gap is
  unrealized P/L — they are intentionally different, not a bug.
- **Header**: the `TopNav` brand is a `<button>` and the global `button{display:inline-flex;gap}` rule
  spaced "Coin"/"Hub"; set `gap:0` on `.brand` so it reads "CoinHub".
- **AutomationWorker hardening**: per-user `recover()` in both the monitor and daily-purchase loops
  (`runUserStepSafely`) — one user's panic can no longer crash the shared worker goroutine (an
  unrecovered panic terminates the whole process). See the single-process constraint below.
- **Postgres pool bounded** (`postgres_connector.go`): 25 open / 10 idle / 1h lifetime, so concurrent
  HTTP handlers + the two worker loops can't exhaust `max_connections`.
- **Pagination on every table**: reusable `Pagination.svelte` (page-size dropdown 10→50 default 10 +
  prev/next + "X–Y of Z"; hidden when ≤10 rows). Wired to Positions, History, and the B3 assets +
  dividends tables (they previously rendered every row forever). i18n `pager.*` (en/pt/es).

### Worker concurrency model (IMPORTANT before scaling)
The `AutomationWorker` runs **two goroutines** (monitor loop 30s, daily-purchase loop 5min) that iterate
all active users **sequentially** inside a **single** API process (one compose replica). Per-user data is
fully isolated (`WHERE user_id`, per-user encrypted keys/clients), so many users' robots run safely side
by side. **But there is no leader election / advisory lock** — running >1 API replica would double-execute
daily buys, stop-loss and reconciles. Daily-buy has partial cross-process protection (DAILY_BUY
idempotency check in shared DB); stop-loss/reconcile do not. So: **do not scale to >1 replica without a
leader lock first.** Other ceilings (fine at current scale): users processed serially (one slow user
delays the rest, bounded by the 8–10s Binance HTTP timeouts); all Binance REST egresses from one IP, so
the IP weight limit (above) bites first.

## TODO / backlog (roughly prioritized)
1. **Secret rotation + git-history purge** — Binance/DB/email creds were committed in history (commit
   `d891d08`); rotation still pending and history not purged. `CREDENTIALS_ENCRYPTION_KEY` must stay
   stable (rotating it makes stored Binance secrets undecryptable — plan a re-encrypt migration if ever
   rotated).
2. **WebSocket user-data + market-price streams** — the real fix for the per-IP Binance rate limit
   (replaces 30s `GetOrderStatus`/ticker polling). See the rate-limit section above.
3. **Leader lock** (e.g. `pg_advisory_lock` or a leadership row) so the worker is safe to run on >1
   replica; then optionally **multiple egress IPs / proxy sharding**.
4. **2FA** (TOTP) — step-up ("sudo") re-auth for money actions is already shipped; full 2FA still
   deferred.
5. **Per-user email price alerts** — `email_alerts` table exists, the route/UI were not rebuilt after the
   multi-user refactor.
6. **More charts** — PnL over time, dividend calendar, etc.
7. **Remove the now-unwired legacy single-user *services*** (`server.go` + `templates/` already deleted;
   the old single-user services they used remain as dead code).
8. **Robot monetization** — standard users are capped at 1 robot/environment
   (`StandardUserRobotLimitPerEnvironment`), admins unlimited; the payment/billing piece is not built.

## Don't print secrets
`.env`, `/root/commands_band_share.txt`, and any API keys. Never echo/commit them.
