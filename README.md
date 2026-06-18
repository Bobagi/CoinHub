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
| **Target profit %** | How far above the buy price the automatic take-profit sell is placed. |
| **Stop-loss %** | How far below the buy price to market-sell as a safety exit. Empty = disabled. |
| **Daily buy time** | The time (your timezone) the one daily DCA buy runs. |
| **Sell-order validity (days)** | How long a take-profit order stays open before auto-cancelling. `0` = never expires (GTC). |
| **Environment** | **Testnet** (practice, fake money) or **Production** (real money, requires opt-in). |

### Spending limits — what exists today
- **Per-order ceiling (active):** every buy — manual or robot — is capped server-side at
  `MAX_ORDER_QUOTE_AMOUNT` (default 100,000 in the quote currency). It's an anti-mistake / anti-abuse
  guardrail, not a per-robot setting, and is **not yet surfaced in the UI**.
- **Total-invested ceiling (not active):** an earlier single-user version had a "capital threshold"
  that capped how much could be invested at once — the bot would wait for a position to sell before
  buying again. The multi-user robot rewrite **removed that behaviour**; the leftover `CapitalThreshold`
  field now simply means "Capital per buy". If you want a max-exposure cap back, it must be rebuilt
  (tracked in the CLAUDE.md backlog).

> Standard accounts get **1 robot per environment**; admins get unlimited (a monetization hook —
> billing isn't built yet).

## Architecture

```
coin.bobagi.space ──nginx(TLS)──▶ web (SvelteKit, Phase 4) ──▶ api (Go) ──▶ Postgres
                                                                   │
                                                                   └─▶ scraper (Python/Flask + Selenium)
```

| Path          | Service   | Stack                      | Role |
|---------------|-----------|----------------------------|------|
| `apps/api`    | `api`     | Go (SOLID, single binary)  | Trading engine + REST API + auth (core) |
| `apps/scraper`| `scraper` | Python 3.11 / Flask + Selenium | Scrapes Investidor10 wallets (internal-only service) |
| `apps/web`    | `web`     | SvelteKit (Phase 4)        | Dashboard SPA with real charts |
| `migrations`  | `migrate` | golang-migrate SQL         | Versioned DB schema |
| `deploy`      | —         | nginx vhost reference      | Ops reference |

## Local development

```bash
cp .env.example .env          # then fill in real values
docker compose up --build     # db + migrate + scraper + api
# API:    http://localhost:5020
# Scraper is internal-only (http://scraper:5000 from the API)
```

Apply a new migration: add `NNNN_name.up.sql` / `.down.sql` under `migrations/`, then
`docker compose up migrate`.

## Roadmap (tracked in the task list)

0. **Monorepo unification** — single repo, one compose. *(done)*
1. **Multi-user core** — users + auth, per-user encrypted Binance keys, user-scoped data.
2. **Trading hardening** — stop-loss + risk caps, WebSocket fills/price for fast reaction.
3. **Portfolio integration** — API calls the scraper; per-user wallet + caching.
4. **SvelteKit frontend** — auth UI, dashboards, and proper charts.
5. **Deploy** — ship to coin.bobagi.space, reset DB, decommission old containers/repo.

## Security posture

- Per-user Binance secrets are encrypted at rest (AES-256-GCM) and never logged.
- Use **trade-only** Binance API keys (withdrawals disabled).
- New users start on **Binance Testnet**; live trading requires explicit opt-in.
- Automated trading carries real financial risk; risk controls (stop-loss, caps) are built in.
