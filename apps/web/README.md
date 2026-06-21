# apps/web — Coin Hub frontend (Svelte + Vite SPA)

The dashboard single-page app served at **https://coin.bobagi.space**. **Svelte 4 + Vite +
TypeScript** (not SvelteKit) — a hash-routed SPA that talks to the Go API (`apps/api`) over JSON
(cookie session, same-origin). It is **built to static files** in `dist/`, which **nginx serves
directly** — so `pnpm build` *is* the deploy (no Node server, no container).

## Build & run

Node 18 + pnpm 9 (via nvm):

```bash
export PATH="$HOME/.nvm/versions/node/v18.20.5/bin:$PATH"
cd apps/web
pnpm install
pnpm build            # vite build → dist/ + copies country flags into dist/flags
pnpm check            # svelte-check (type + a11y); pnpm dev for the Vite dev server
```

From the repo root, `./deploy.sh web` runs the build (the production deploy for the SPA).

## What's built (today)
- **Auth screens** (`Login.svelte`): email sign-up/sign-in, Google sign-in, forgot/reset password,
  email-verification landing.
- **Consent gate** (`AgreementGate.svelte`): a blocking, full-screen Terms-of-Use + Privacy-Policy
  acceptance shown until the user accepts the version in force; plus the public, permanent
  `#/terms` and `#/privacy` pages (`LegalDocument.svelte`).
- **Dashboard** (`Dashboard.svelte`): 3 tabs — **Trade** (default, with the buy panel + a **Robots**
  list/editor), **Connection** (Binance credentials + environment switch), **B3** (Investidor10
  portfolio, admin-only). A collapsible **"Positions & performance"** card with an **AllocationPanel**
  (wallet donut + price-history chart) and a **ProfitabilityPanel** (cost vs realized, P/L, tax note),
  plus an **Operations** card (Positions / History sub-tabs, paginated).
- **Account** (`AccountSettings.svelte`): edit name, set/change password, language, the consent record
  ("you accepted version X on <date>"), **access/sign-in history** with country flags + new-device
  badges, and a privacy-preserving account deletion.
- **Cross-cutting:** trilingual i18n (`i18n.ts`, en/pt/es auto-detected), the gold design system
  (`app.css` tokens), a sticky `TopNav` (with the Google avatar), global `AppModal` + toast
  notifications (`Toasts.svelte`), a reusable `Collapsible`, `Pagination`, country/language flags
  (local SVGs), and a **dormant cookie-consent banner** (`CookieConsent.svelte`, shows once ads are
  enabled via `stores.adsEnabled`).

## Conventions
- **Editable source lives in `src/lib/`.** The repo-root `.gitignore` ignores `lib/`, so
  `apps/web/.gitignore` re-includes it (`!src/lib/`) — don't remove that or the UI stops being committed.
- **All UI strings go through `src/lib/i18n.ts`** (every key in all three dicts), never inline.
- **Spacing/sizing use the `app.css` design tokens** (`var(--space-*)`, type scale), not magic px.
- New error catches call **`notifyError(e)`** (toast), not inline error text — except standalone auth
  pages, which stay inline. New collapsibles use **`<Collapsible>`**, not a hand-rolled `<details>`.
