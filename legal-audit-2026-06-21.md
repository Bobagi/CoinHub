# CoinHub — Legal & Risk Audit (2026-06-21)

> **Disclaimer.** This is an **engineering-led risk review** by Claude, **not legal, tax or
> regulatory advice**, and I am not a lawyer. It maps where the product could create legal/financial
> exposure so you can prioritize. The drafted Terms/Privacy (`agreement.*` in `i18n.ts`) are a
> **template baseline**, not a lawyer-reviewed contract. **Before you charge money or run ads, engage a
> Brazilian lawyer** (ideally one familiar with CVM / Marco Legal dos Criptoativos and consumer/LGPD
> law) and an accountant. Brazilian rules cited below are from general knowledge and **must be
> confirmed with counsel** — tax/crypto rules in particular have changed recently.

## 0. How the app actually works (the factual basis being reviewed)
Multi-user web app at `coin.bobagi.space`. A user signs up (email or Google), connects **their own**
Binance account via API keys (stored **encrypted**, AES-256-GCM), and configures **robots** that run a
**DCA + take-profit (+ optional stop-loss)** strategy. The software places orders **on the user's
Binance account through the user's keys**; it is **non-custodial** — it never holds, receives or can
withdraw funds. New users default to **Testnet**; **real-money trading is an explicit opt-in** guarded
by email verification + step-up re-auth. There's a B3/Investidor10 portfolio view (admin-only), charts
(allocation, profitability, P/L), an access/sign-in history with new-device email alerts, and a
**privacy-preserving hard delete**. **Stated plan: charge for running robots, and show ads.**

## 0a. Status after the 2026-06-21 implementation pass
Everything **buildable in code** from the action list below has now been **shipped** (see the ✅ tags).
What remains is, by nature, **off-platform** — it needs you, a lawyer, an accountant, or an external
provider (a court/regulator opinion, a CNPJ, AdSense approval, secret rotation on live services). Those
are tagged ⛔ (can't be done in code) below, with what *you* must do.

**Shipped in this pass (code):**
- ✅ **Public, permanent, versioned Terms & Privacy pages** — `#/terms` and `#/privacy`
  (`LegalDocument.svelte`), linked from the footer, the consent gate and the account page. Trilingual.
- ✅ **Full standalone Privacy Policy** (`privacy.*`, en/pt/es): controller identity, contact/encarregado,
  what data + **legal bases**, sharing/processors, **international transfer**, retention, security,
  **cookies**, **LGPD rights**, children, changes — covering audit item F/P1#7.
- ✅ **Strengthened consent**: the gate checkbox now also affirms **18+ and account/key ownership**
  (item J/P2#13); the version was **bumped to `2026-06-21.2`** so the users who already accepted re-accept
  the expanded text; the gate links to the full Terms/Privacy pages.
- ✅ **Explicit 7-day right of withdrawal (CDC art. 49)** added to the paid-features clause (item E/P0#6,
  the *text* part — the actual cancel/refund flow waits for the billing system).
- ✅ **CVM-defensive wording**: the Terms now state the user **chooses every parameter and the software
  only executes their rules, never trading at its own discretion or managing assets** (item B framing).
- ✅ **Tax-responsibility reminder** shown next to the profitability panel (item D/P1#10).
- ✅ **Account page shows the consent record** — which version you accepted and when
  (`GET /api/v1/account/agreement`) (item A/P1#9).
- ✅ **Cookie-consent mechanism built and ready** (`CookieConsent.svelte` + `cookieConsent` store),
  **dormant** behind `stores.adsEnabled=false` so it shows the moment ads are switched on, and any
  ad/analytics script must gate on `cookieConsent==='accepted'` (item F/P0#3 — *mechanism*; see ⛔ note).

## 1. Existing strengths (don't regress these)
Non-custodial architecture; encrypted keys + trade-only-key recommendation; testnet-first + live opt-in;
step-up auth on money actions; enforced email verification; access logging + new-device alerts;
privacy-preserving hard delete with an HMAC audit row; risk language in the footer; and, as of today,
**server-side, versioned, timestamped, IP-logged Terms+Privacy consent with enforcement**. This is a
materially better posture than most hobby fintech.

---

## 2. Risk areas (prioritized; 🔴 high / 🟠 medium / 🟢 lower)

### 🔴 A. Consent & the Terms themselves — *partly fixed today, but the text needs a lawyer*
- **Fixed today:** consent is now recorded server-side (version + timestamp + IP/UA), a blocking gate
  forces acceptance, and money/robot endpoints refuse to act without it. This closes the "user just
  creates an account and starts trading with nothing accepted" gap you flagged.
- **Still open:** the Terms/Privacy text I wrote is an **engineer's template**. It must be reviewed by
  counsel. Also: (a) publish **permanent, public, versioned** Terms & Privacy pages (today the full text
  only lives in the acceptance gate); (b) surface in the account page **which version** the user accepted
  and **when** (we store it); (c) consider also showing the checkbox **at the signup form** for belt-and-
  suspenders clickwrap (the post-creation gate is legally serviceable since no feature works until accept).

### 🔴 B. Securities / "managing other people's money" exposure (the biggest one for the paid model)
Charging to **run an automated robot that places trades for a user** can be read as a **regulated
activity** in Brazil:
- **Administração de carteiras de valores mobiliários** (CVM Resolução 21/2021) — discretionary
  management of third parties' assets — and/or **consultoria** (CVM Res. 19/2021). These require CVM
  authorization.
- **Crypto angle:** the **Marco Legal dos Criptoativos (Lei 14.478/2022)** puts virtual-asset service
  providers under **BCB** oversight; **CVM Parecer de Orientação 40/2022** says some tokens are
  securities (then CVM applies). So depending on which coins the robots trade, **BCB and/or CVM** rules
  may bite.
- **Why you may be okay, if positioned right:** the user **chooses the coin and all parameters**, and the
  robot executes a **deterministic, user-defined** rule — arguably a **self-directed tool**, not
  discretionary management. The Terms already say "not a broker/adviser, no advice, you decide."
- **Action:** get a **formal legal opinion on whether the paid robot = administração de carteira**
  *before* charging. Keep all copy free of "advice", "recommendation", "we manage", "guaranteed". This is
  the single item most likely to cause a regulatory problem.

### 🔴 C. Misleading-performance / advertising risk
- Implying returns is dangerous (**CDC art. 37**, propaganda enganosa; CVM rules on performance claims).
- The UI shows **profitability/P-L charts** — make sure they read as **the user's own historical results**,
  never projections or example gains. No "users earn X%". Risk disclosure must stay prominent. **Never**
  show a sample portfolio "growing."

### 🔴 D. Fiscal / business formation (auditor view) — *needed before taking money*
- Charging for robots and earning ad revenue is **business income**. Operating paid SaaS as an
  **individual without a CNPJ** and without issuing **notas fiscais** is a real fiscal exposure. You'll
  likely need a **CNPJ (MEI may be too small / wrong CNAE)**, to issue invoices, and to pay the
  applicable taxes (**ISS** on software/service, plus federal). **Form the entity and set up invoicing
  before the first paid charge / first ad payout.**
- Separately, the **user's** crypto taxes are **theirs** (e.g. RFB crypto reporting; the 2023 reform
  **Lei 14.754/2023** changed crypto/offshore taxation). The app must **not** give tax advice but should
  keep reminding users they're responsible (the Terms do). Consider a one-line reminder near the
  profitability panel. *Confirm current tax rules with an accountant — they changed recently.*

### 🟠 E. Consumer law (CDC) for paid subscriptions
- **Right of withdrawal (art. 49):** distance contracts get a **7-day** cancel/refund. Your Terms say
  "non-refundable except where law requires" — correct, **but you must actually honor the 7-day right**;
  state it explicitly and build the refund/cancel path.
- **Auto-renewal** must be clearly disclosed up front and **easy to cancel** in-app (no dark patterns).
- **Price transparency** before charge; notice before any price change.

### 🟠 F. LGPD / data protection (especially once ads are on)
- You process email, **IP + geolocation + device**, Google profile, and **encrypted Binance keys** —
  personal (some financial-adjacent) data. You need a **complete standalone Privacy Policy** with:
  **controller identity** (who is the legal entity), an **encarregado/DPO contact**, **legal bases**
  (contract/consent/legitimate interest per purpose), **retention**, **data-subject-rights** process,
  **breach-notification** process (ANPD), and an **international-transfer** note (Binance, Google,
  Hostinger hosting are abroad/3rd parties).
- **Cookie consent banner — required before enabling ads.** Ad networks (AdSense etc.) drop tracking
  cookies; under LGPD + ANPD cookie guidance you need a **consent banner** with opt-in/opt-out. **You
  don't have one yet — add it before ads go live.**
- The **HMAC email fingerprint** kept in the deletion audit is defensible (fraud/abuse prevention,
  legitimate interest) but **document it** in the Privacy Policy so it isn't a surprise.

### 🟠 G. You hold users' exchange API keys
- Storing third-party **financial keys** raises the stakes of any breach. Good: AES-256-GCM, trade-only
  recommendation, stable key. Risks: the **encryption key + DB live on the same VPS**, so a server
  compromise is decryptable; and **backlog #1 — secrets were committed to git history and rotation is
  still pending**. For an app holding financial keys that is a **real liability** — **prioritize rotating
  the leaked secrets and consider key separation (KMS/secret manager)** and a written incident-response
  plan. The Terms disclaim liability, but negligence with financial credentials is hard to disclaim away.

### 🟠 H. Binance API terms / commercial use
- Using customers' Binance API keys inside a **paid** service may trigger Binance's API/commercial-use
  clauses and regional restrictions. **Review Binance's API Terms for "operating a service / commercial
  use"** and keep the **eligibility/jurisdiction** clause (you have one). The per-IP rate limit is also a
  scaling wall already documented in `CLAUDE.md`.

### 🟢 I. Advertising specifics (AdSense & friends)
- Crypto/financial content is a **restricted category** on some ad networks — **confirm your crypto-
  trading-tool content is eligible** before integrating. Provide the network's required disclosures +
  `ads.txt`, and **don't place ads next to "Buy"/"Sell" buttons** in a way that implies endorsement.

### 🟢 J. Age / KYC
- Terms assert **18+** but it isn't verified. Full KYC is likely unnecessary (non-custodial; Binance does
  KYC on the actual funds) — but add an **age self-declaration** at signup and **document your reliance on
  Binance's KYC** in the Privacy Policy.

### 🟢 K. Availability / operations as legal expectation
- Single VPS, single worker, **no leader lock**, shared IP that also runs LND + passive-income earner
  traffic (an abuse/blocklist risk already noted in `/opt/CLAUDE.md`). Paying users expect uptime. Your
  Terms disclaim warranties (good) — **keep it that way, don't promise an SLA**, and consider a status
  page. The shared-IP earner traffic is worth dropping before you have paying customers.

---

## 3. Prioritized action list  *(✅ done in code · 🟡 partly done · ⛔ off-platform: needs you/lawyer/provider)*

**P0 — before the first paid charge or first ad impression**
1. ⛔ **Lawyer review** of Terms + Privacy, with a specific **CVM/administração-de-carteiras opinion** on
   the paid robot (B). *Needs a Brazilian lawyer — I can't give a legal opinion. The text is now drafted
   and CVM-defensively worded for them to review.*
2. ⛔ **Form a legal entity (CNPJ) + invoicing** and confirm taxes with an accountant (D). *Government +
   accountant action.*
3. 🟡 **Cookie-consent banner** (LGPD) before ads (F). *Mechanism built + dormant; you flip
   `stores.adsEnabled` to true (and gate the ad script on consent) when ads go live.*
4. ✅ **Public, permanent, versioned** Terms & Privacy pages (A). **Done.**
5. ⛔ **Rotate the leaked secrets + purge git history** (backlog #1) — financial keys (G). *Must be done
   against the live external services (Binance/DB/SMTP) and rewrites git history (force-push) — too
   destructive/credential-bound to do unattended. **Do this with me in a dedicated session**: rotate each
   key on its provider, update `.env`, `docker compose up -d`, then `git filter-repo` + force-push.
   `CREDENTIALS_ENCRYPTION_KEY` must NOT change without a re-encrypt migration (it would brick stored
   Binance secrets).*
6. 🟡 **Explicit 7-day withdrawal + clear cancel/refund flow** for subscriptions (E). *Text added; the
   actual cancel/refund flow ships with the billing system (backlog #8, not built yet).*

**P1**
7. ✅ Full **Privacy Policy**: controller, encarregado/contact, legal bases, retention, international
   transfer, cookies, LGPD rights (F). **Done** (the ANPD breach-notification *process* is an
   operational runbook for you to keep, not app code).
8. ⛔ **AdSense crypto-eligibility** check + `ads.txt` + ad placement rules (I). *Needs a Google publisher
   account + approval; crypto content is a restricted AdSense category — confirm eligibility first.*
9. ✅ Show users **which terms version they accepted + date** in the account page (A). **Done.**
10. ✅ **Tax-responsibility reminder** near the profitability panel (D). **Done.**

**P2**
11. ⛔ **Key separation** (KMS/secret manager) for `CREDENTIALS_ENCRYPTION_KEY` + incident-response doc
    (G). *Infra decision; a single VPS has no managed KMS — needs an external secret manager.*
12. 🟡 **Status/uptime page**; continue to make no availability promises (K). *Terms already disclaim
    warranties; a status page is optional polish, deferred.*
13. ✅ **Age self-declaration** at signup (J) — folded into the consent checkbox (18+ + account/key
    ownership). *Documenting reliance on Binance KYC is covered in the Privacy Policy.*

## 3a. What CANNOT be done now (and exactly what you must do)
- **Legal opinion / lawyer sign-off (P0#1)** — engage a Brazilian lawyer; give them `legal-audit-…md`
  + the live Terms/Privacy. The CVM "is the paid robot regulated portfolio management?" question is the
  one to ask first.
- **CNPJ + invoicing + tax setup (P0#2)** — open the company / MEI-or-other, pick the CNAE, set up nota
  fiscal, before taking any money or ad payout.
- **Secret rotation + git-history purge (P0#5)** — credential-bound + destructive; do it with me in a
  focused session (don't rotate `CREDENTIALS_ENCRYPTION_KEY` without the re-encrypt migration).
- **AdSense eligibility/approval + publisher id + real `ads.txt` (P1#8)** — Google-side; then I wire the
  ad slots and flip `adsEnabled` (which auto-shows the cookie banner already built).
- **Subscription cancel/refund flow (P0#6 flow)** — ships with the billing system (backlog #8), which
  isn't built yet; the *7-day right* is already stated in the Terms.
- **Managed KMS for the encryption key (P2#11)** — needs an external secret manager / infra change.

---

## 4. Direct answers to your two questions
- **"Do we need something in the DB, or is front-only enough?"** → **DB, and it's done.** A front-only
  checkbox is **invalid** (not enforceable, not auditable, trivially bypassed). Consent is now
  **versioned, timestamped, IP/UA-logged, server-enforced**, and required before any money/robot action.
- **"Are the current Terms enough?"** → They're a **solid baseline** that covers the big themes
  (non-custodial, risk/no-guarantee, paid robots & billing, ads, LGPD/data, liability, Brazil law). They
  are **not a substitute for a lawyer.** Before monetizing, the three gaps that matter most are: **(B)** a
  CVM "is the paid robot regulated portfolio management?" opinion, **(E)** consumer-law specifics (7-day
  withdrawal + auto-renewal disclosure + cancel flow), and **(F)** a cookie-consent banner + a full
  standalone Privacy Policy — plus **(D)** the fiscal entity so you can legally take the money.
