# Coin Hub

A personal investing platform served at **https://coin.bobagi.space**. It unifies two
previously separate projects into one multi-user application:

- **Crypto** — log and automate Binance trades (market buy, take-profit limit sell, daily
  DCA, price alerts) with per-user, encrypted API credentials. Testnet by default; live
  trading is an explicit per-user opt-in.
- **B3 portfolio** — read an [Investidor10](https://investidor10.com.br) public wallet to
  show holdings, order history, and upcoming dividend (data-com) dates.

> Merges two previously separate projects — a Go crypto trading app and the Python
> `Bobagi/investidor10` reader — into one repo.

## How the trading robots work (and what they are *not*)

Coin Hub's automation is built around **trading robots**. A robot is one automated bot bound to a
single coin/pair in one environment (e.g. `BTCBRL` on Testnet). It is **not** a black box that
"prints money" — it mechanically executes a strategy *you* configure, and it carries real risk.

### The strategy each robot runs
A robot combines three well-known, conservative mechanics:

- **DCA (Dollar-Cost Averaging) — the daily buy.** Once a day, at the time you choose, the robot
  market-buys a **fixed amount** ("Capital per buy") of the coin, regardless of price. Buying steadily
  smooths out volatility and your average entry price instead of trying to time the market.
- **Take-profit — the automatic sale.** Every buy immediately places a limit **sell** order a set
  percentage above its purchase price ("Target profit %"). When the market reaches it, that lot is
  sold for a profit, automatically.
- **Stop-loss (optional) — the safety exit.** If a position's price falls a chosen percentage below
  its buy price ("Stop-loss %"), the robot cancels the take-profit and sells at market to cap the
  loss. Leave it empty to disable.

So each daily DCA buy becomes its own tracked position with its own take-profit (and optional
stop-loss). The robot reviews open positions about every 30 seconds.

### What Coin Hub is **not**
- **Not an arbitrage bot.** Arbitrage means buying on one exchange and selling on another to pocket
  the price gap. Coin Hub connects to **one exchange per user (Binance)**, so cross-exchange arbitrage
  is impossible here by design — and on the major exchanges that gap is now < 0.5% anyway.
- **Not a grid-trading bot.** Grid bots place many staggered buy/sell orders across a price range to
  scalp sideways markets. Coin Hub does scheduled DCA + one take-profit per lot, not grid scalping.
- **Not a profit guarantee.** "3% a day on autopilot" is marketing, not reality. Returns depend on the
  market, the coin and your settings. Only allocate money you could tolerate losing.

### Each robot setting, in plain terms
| Setting | What it does |
|---|---|
| **Coin/pair** | The market the robot trades (e.g. `BTCBRL`). It buys using the *quote* currency — the asset at the **end** of the pair (`BTCBRL` → BRL, `BTCUSDT` → USDT) — so keep enough of it in your Binance spot wallet. |
| **Capital per buy** | How much of the quote currency each daily DCA buy spends. `0` disables the daily buy. |
| **Max invested** | The most the robot keeps invested in this coin at once (cost basis of open positions). When it's reached, the daily buy pauses and resumes only after a position sells to free up room. `0` = no limit. |
| **Target profit %** | How far above the buy price the automatic take-profit sell is placed. |
| **Stop-loss %** | How far below the buy price to market-sell as a safety exit. Empty = disabled. |
| **Daily buy time** | The time (your timezone) the one daily DCA buy runs. |
| **Sell-order validity (days)** | How long a take-profit order stays open before auto-cancelling. `0` = never expires (GTC). |
| **Environment** | **Testnet** (practice, fake money) or **Production** (real money, requires opt-in). |

### Spending limits — both active
- **Per-order ceiling:** every buy — manual or robot — is capped server-side at
  `MAX_ORDER_QUOTE_AMOUNT` (default 100,000 in the quote currency). It's a global anti-mistake /
  anti-abuse guardrail, not a per-robot setting; its value is shown as a help line in the robot editor.
- **Per-robot max-invested ceiling:** the **Max invested** setting (above) caps the total kept invested
  in a coin at once. The robot's daily buy pauses when open positions for that coin reach the cap and
  resumes once one sells to free capital. `0` = no limit. (Manual buys are not blocked by this cap —
  only the robot's automatic daily buy is.)

> Standard accounts get **1 robot per environment**; admins get unlimited (a monetization hook —
> billing isn't built yet).

## Accounts, security & legal

- **Sign-in:** email + password (bcrypt) **or Google sign-in**. Email **verification is enforced** —
  unverified accounts can browse but every save (connect Binance, trade, robots) is blocked. Password
  reset + email verification go out over Gmail SMTP. Sessions are opaque tokens in a Secure httpOnly
  cookie; only the SHA-256 hash is stored. Money actions require a fresh **step-up ("sudo") re-auth**.
- **Consent (Terms of Use + Privacy Policy):** a new account must **accept the Terms + Privacy** before
  using anything — a blocking gate, with the acceptance **recorded server-side** (version, timestamp,
  IP, user-agent). The version is bumped when the text changes materially, forcing re-acceptance. The
  full documents are public and permanent at `#/terms` and `#/privacy`; the account page shows which
  version you accepted and when. The API also refuses money/robot actions without an on-record
  acceptance (`403 terms_not_accepted`).
- **Your data & privacy (LGPD-aligned):** Binance API keys are encrypted at rest (AES-256-GCM) and used
  only to place the orders you configure — **use trade-only keys with withdrawals disabled**. Account
  deletion is a **privacy-preserving hard delete** (PII + keys erased; only a non-identifying HMAC audit
  row remains). A durable **sign-in/access log** (IP + offline city geolocation + device) powers
  new-device email alerts. See the full Privacy Policy in-app and the risk/compliance review in
  [`legal-audit-2026-06-21.md`](legal-audit-2026-06-21.md).
- **Not custodial, not advice:** Coin Hub never holds or can withdraw your funds; it is software you
  configure, **not** a broker, fund manager or investment adviser, and is not affiliated with Binance.

## Architecture

```
coin.bobagi.space ──nginx(TLS)──▶ web (Svelte+Vite SPA, static dist/) 
                            └────▶ api (Go, 127.0.0.1:5020) ──▶ Postgres (internal)
                                                                   │
                                                                   └─▶ scraper (Python/Flask + Selenium)
```

nginx serves the SPA's static `dist/` directly and reverse-proxies `/api`,`/auth`,`/health` to the Go
API on `127.0.0.1:5020`. The DB has no host port (internal only).

| Path          | Service   | Stack                      | Role |
|---------------|-----------|----------------------------|------|
| `apps/api`    | `api`     | Go (SOLID, single binary, distroless) | Trading engine + REST API + auth (core) |
| `apps/scraper`| `scraper` | Python 3.11 / Flask + Selenium | Scrapes Investidor10 wallets (internal-only service) |
| `apps/web`    | `web`     | Svelte 4 + Vite + TypeScript (static SPA) | Dashboard SPA with real charts; built to `dist/`, served by nginx |
| `migrations`  | `migrate` | golang-migrate SQL         | Versioned DB schema (0001..00NN) |
| `deploy`      | —         | nginx vhost reference      | Ops reference |

### Reliability, the automation worker & scaling

The robots are driven by a background **automation worker** inside the API process: a monitor loop
(~30s) that reconciles take-profit fills, runs stop-loss and expires stale sell orders, plus a
daily-purchase loop that runs each robot's DCA buy once a day. Without it, robots are just config rows.

- **Liveness & alerting.** The worker writes a **heartbeat** every tick (`worker_heartbeat` table).
  `GET /health/worker` returns **503** when the heartbeat is stale — point an external uptime monitor at
  it — and a built-in **watchdog emails admins** if the worker stalls while the process is still alive.
- **Operational status in the UI.** `GET /api/v1/system/status` aggregates worker liveness + the shared
  Binance rate-limit gate. The light next to "Binance" in the header turns **red** (green = OK) whenever
  automation is paused, with the reason on hover plus a banner; a manual buy/sell during a Binance
  cooldown **fails fast** with a clear `binance_busy` message instead of hanging. The Terms disclose that
  automated operation may pause and resumes automatically once the condition clears.
- **Singleton worker + scaling.** The worker runs **only on the replica that holds a Postgres advisory
  lock** (`LeaderLock`), so the stateless HTTP API can be scaled behind a load balancer **without** every
  replica double-executing daily buys/stop-loss. A load balancer alone does *not* parallelize the worker
  — it would multiply it; the right path is the leader-lock singleton now, and user **sharding** later
  (today's bottleneck is the per-IP Binance request-weight limit, not CPU).

## Local development

```bash
cp .env.example .env          # then fill in real values
docker compose up --build     # db + migrate + scraper + api
# API:    http://localhost:5020
# Scraper is internal-only (http://scraper:5000 from the API)
```

Apply a new migration: add `NNNN_name.up.sql` / `.down.sql` under `migrations/`, then
`docker compose up migrate`.

## Status

**Live today** at https://coin.bobagi.space: monorepo + compose; multi-user auth (email + Google,
enforced email verification, password reset, step-up re-auth, access/sign-in history); per-user
encrypted Binance credentials with testnet/production isolation; manual buy + take-profit + manual
close; **trading robots** (per-coin DCA + take-profit + optional stop-loss) run by a single-process
automation worker; the Svelte SPA with the design system, real charts (allocation donut, price/profit
history), pagination, toasts and trilingual i18n; the B3/Investidor10 portfolio (admin-only); the
**Terms + Privacy consent gate** with public legal pages; and a privacy-preserving account deletion.

### To do / to fix (engineering — see `CLAUDE.md` "TODO / backlog" for the full list)
- **Secret rotation + git-history purge** — Binance/DB/email creds were committed in history; rotation
  still pending (do **not** rotate `CREDENTIALS_ENCRYPTION_KEY` without a re-encrypt migration).
- **WebSocket user-data + market-price streams** — replace the 30s REST polling; the real fix for the
  per-IP Binance rate limit as the user base grows. (Planned as its own carefully-tested phase.)
- ~~**Leader lock** before running >1 API replica~~ — **done**: the worker is now a singleton via a
  Postgres advisory lock (`LeaderLock`), so the API is safe to scale. Next, only when volume needs it:
  shard users across worker instances.
- Real server-side pagination for Positions/History; drop the vestigial
  `trading_robots.daily_purchase_enabled` column; remove dead legacy single-user services.

### Future plans
- **Monetization:** paid robots beyond the free tier (standard users are capped at 1 robot/env today)
  + advertising — both need the items in `legal-audit-2026-06-21.md` first (a billing/subscription
  system with a 7-day-withdrawal cancel/refund flow, a cookie-consent banner — already built and
  dormant behind `stores.adsEnabled` — and the legal/fiscal setup below).
- **2FA (TOTP)**, per-user email price alerts, more charts (PnL over time, dividend calendar).
- **Operator/legal (not code):** lawyer review + a CVM opinion on the paid robot, a CNPJ + invoicing,
  and AdSense eligibility — tracked with status in `legal-audit-2026-06-21.md` §3/§3a.

## Security posture

- Per-user Binance secrets are encrypted at rest (AES-256-GCM) and never logged.
- Use **trade-only** Binance API keys (withdrawals disabled).
- New users start on **Binance Testnet**; live trading requires explicit opt-in.
- Automated trading carries real financial risk; risk controls (stop-loss, per-order + per-robot caps)
  are built in. Returns are never guaranteed.
- See **Accounts, security & legal** above and [`legal-audit-2026-06-21.md`](legal-audit-2026-06-21.md)
  for the full security/compliance picture and the operator action list.
