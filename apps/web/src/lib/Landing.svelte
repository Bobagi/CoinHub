<script lang="ts">
  import { t } from './i18n'
  import { navigate, authMode } from './stores'
  import LanguageDropdown from './LanguageDropdown.svelte'
  import LegalFooter from './LegalFooter.svelte'

  function goSignup() {
    authMode.set('signup')
    navigate('login')
  }
  function goLogin() {
    authMode.set('login')
    navigate('login')
  }

  // Sequence is real (each buy flows buy → take-profit → optional stop), so the numbered steps in the
  // i18n titles encode order, not decoration.
  const steps = [
    { t: 'landing.how.dca.title', b: 'landing.how.dca.body' },
    { t: 'landing.how.tp.title', b: 'landing.how.tp.body' },
    { t: 'landing.how.sl.title', b: 'landing.how.sl.body' }
  ]
  const feats = [
    { t: 'landing.feat.robots.title', b: 'landing.feat.robots.body' },
    { t: 'landing.feat.envs.title', b: 'landing.feat.envs.body' },
    { t: 'landing.feat.charts.title', b: 'landing.feat.charts.body' },
    { t: 'landing.feat.security.title', b: 'landing.feat.security.body' }
  ]
  const securePoints = [
    'landing.security.point1',
    'landing.security.point2',
    'landing.security.point3',
    'landing.security.point4'
  ]
</script>

<div class="lp">
  <header class="nav">
    <div class="brand">Coin<span>Hub</span></div>
    <div class="nav-actions">
      <LanguageDropdown compact />
      <button type="button" class="signin" on:click={goLogin}>{$t('landing.signIn')}</button>
      <button type="button" class="btn-primary" on:click={goSignup}>{$t('landing.getStarted')}</button>
    </div>
  </header>

  <main>
    <!-- Hero: the thesis — your keys, your rules. Left = the claim, right = the robot, live. -->
    <section class="hero">
      <div class="hero-copy">
        <span class="status"><span class="status-dot" aria-hidden="true"></span>{$t('landing.hero.eyebrow')}</span>
        <h1>{$t('landing.hero.title')} <span class="accent">{$t('landing.hero.titleAccent')}</span></h1>
        <p class="sub">{$t('landing.hero.subtitle')}</p>
        <div class="cta">
          <button type="button" class="btn-primary lg" on:click={goSignup}>{$t('landing.hero.ctaPrimary')}</button>
          <button type="button" class="signin lg" on:click={goLogin}>{$t('landing.hero.ctaSecondary')}</button>
        </div>
        <ul class="trust">
          <li>{$t('landing.hero.trust1')}</li>
          <li>{$t('landing.hero.trust2')}</li>
          <li>{$t('landing.hero.trust3')}</li>
        </ul>
      </div>

      <!-- Signature: the product's actual daily cycle (DCA → take-profit → sold), rendered as the
           robot's own log. It runs on the user's keys and never withdraws — the non-custodial thesis,
           shown rather than asserted. Decorative duplicate of the copy, so hidden from assistive tech. -->
      <aside class="console" aria-hidden="true">
        <div class="console-top">
          <span class="bot">
            <svg class="bot-glyph" viewBox="0 0 24 24" width="18" height="18">
              <line x1="12" y1="3" x2="12" y2="8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
              <circle cx="12" cy="2.7" r="1.4" fill="currentColor" />
              <rect x="4" y="8" width="16" height="11" rx="3.5" fill="none" stroke="currentColor" stroke-width="1.6" />
              <circle cx="9.2" cy="13.4" r="1.5" fill="currentColor" />
              <circle cx="14.8" cy="13.4" r="1.5" fill="currentColor" />
            </svg>
            {$t('landing.hero.demo.robot')}
          </span>
          <span class="env">Testnet</span>
          <span class="live"><span class="live-dot"></span>{$t('landing.hero.demo.live')}</span>
        </div>
        <div class="tape">
          <div class="line buy">
            <span class="time">09:00</span>
            <span class="evt">{$t('landing.hero.demo.buy')}</span>
            <span class="amt">+$20.00</span>
          </div>
          <div class="line wait">
            <span class="time">09:00</span>
            <span class="evt">{$t('landing.hero.demo.tp')}</span>
            <span class="tag">{$t('landing.hero.demo.placed')}</span>
          </div>
          <div class="line sell">
            <span class="time">14:32</span>
            <span class="evt">{$t('landing.hero.demo.sold')}</span>
            <span class="amt up">+$0.61</span>
          </div>
        </div>
        <p class="console-foot">{$t('landing.hero.demo.caption')}</p>
      </aside>
    </section>

    <!-- How a robot works (the product explainer). -->
    <section class="block">
      <div class="head">
        <h2>{$t('landing.how.title')}</h2>
        <p class="head-sub">{$t('landing.how.subtitle')}</p>
      </div>
      <div class="grid steps">
        {#each steps as s}
          <article class="card step">
            <h3>{$t(s.t)}</h3>
            <p>{$t(s.b)}</p>
          </article>
        {/each}
      </div>
    </section>

    <!-- Features. -->
    <section class="block">
      <div class="head">
        <h2>{$t('landing.features.title')}</h2>
        <p class="head-sub">{$t('landing.features.subtitle')}</p>
      </div>
      <div class="grid feats">
        {#each feats as f}
          <article class="card feat">
            <h3>{$t(f.t)}</h3>
            <p>{$t(f.b)}</p>
          </article>
        {/each}
      </div>
    </section>

    <!-- Security / non-custodial trust. -->
    <section class="block">
      <article class="card secure">
        <h2>{$t('landing.security.title')}</h2>
        <p class="secure-lead">{$t('landing.security.p1')}</p>
        <ul class="checks">
          {#each securePoints as p}
            <li><span class="tick" aria-hidden="true">✓</span><span>{$t(p)}</span></li>
          {/each}
        </ul>
      </article>
    </section>

    <!-- Honest risk note (no profit promise; CVM/CDC-safe wording). -->
    <section class="block">
      <article class="card risk">
        <h3>{$t('landing.risk.title')}</h3>
        <p>{$t('landing.risk.body')}</p>
      </article>
    </section>

    <!-- Final call to action. -->
    <section class="block">
      <article class="card final">
        <h2>{$t('landing.cta.title')}</h2>
        <p class="head-sub">{$t('landing.cta.subtitle')}</p>
        <button type="button" class="btn-primary lg" on:click={goSignup}>{$t('landing.cta.button')}</button>
      </article>
    </section>
  </main>

  <LegalFooter />
</div>

<style>
  .lp { min-height: 100vh; }

  /* Top bar */
  .nav {
    display: flex; flex-wrap: wrap; gap: var(--space-3);
    align-items: center; justify-content: space-between;
    max-width: var(--page-max); margin: 0 auto;
    padding: var(--space-4) var(--space-5);
  }
  .brand { font-size: var(--text-xl); font-weight: 800; }
  .brand span { color: var(--brand); }
  .nav-actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }
  .signin { background: transparent; border: 1px solid var(--border-strong); color: var(--text); font-weight: 700; }
  .signin:hover:not(:disabled) { border-color: var(--brand); filter: none; }

  /* Hero — asymmetric split: the claim (left) beside the robot, live (right) */
  .hero {
    max-width: var(--page-max); margin: 0 auto;
    padding: var(--space-7) var(--space-5);
    display: grid; grid-template-columns: 1.05fr 0.95fr;
    gap: var(--space-7); align-items: center;
  }
  .hero-copy {
    display: flex; flex-direction: column; align-items: flex-start; gap: var(--space-4);
    animation: hero-rise 0.6s ease both;
  }
  .status {
    display: inline-flex; align-items: center; gap: var(--space-2);
    border: 1px solid var(--border-strong); border-radius: var(--radius-pill);
    padding: var(--space-1) var(--space-3);
    font-size: var(--text-xs); color: var(--brand-soft);
    text-transform: uppercase; letter-spacing: 0.08em;
  }
  .status-dot {
    width: 7px; height: 7px; border-radius: 50%; background: var(--brand);
    animation: pulse-gold 2.4s ease-out infinite;
  }
  .hero h1 {
    font-size: clamp(2.1rem, 4.6vw, 3.4rem); line-height: 1.08; font-weight: 800;
    margin: 0; letter-spacing: -0.02em;
  }
  .accent {
    background: linear-gradient(100deg, var(--brand), var(--brand-strong));
    -webkit-background-clip: text; background-clip: text; color: transparent;
  }
  .sub { color: var(--muted); font-size: var(--text-md); line-height: 1.6; max-width: 46ch; margin: 0; }
  .cta { display: flex; flex-wrap: wrap; gap: var(--space-3); margin-top: var(--space-2); }
  .lg { height: 3rem; padding: 0 var(--space-5); font-size: var(--text-md); }
  .trust {
    list-style: none; padding: 0; margin: var(--space-3) 0 0;
    display: flex; flex-wrap: wrap; gap: var(--space-2);
  }
  .trust li {
    display: inline-flex; align-items: center; gap: var(--space-2);
    border: 1px solid var(--border); border-radius: var(--radius-pill);
    padding: var(--space-1) var(--space-3);
    font-size: var(--text-xs); color: var(--muted);
  }
  .trust li::before { content: ''; width: 6px; height: 6px; border-radius: 50%; background: var(--brand); }

  /* Signature: the live robot console */
  .console {
    position: relative;
    font-family: ui-monospace, 'SF Mono', 'JetBrains Mono', Menlo, Consolas, monospace;
    background:
      radial-gradient(120% 90% at 100% 0%, rgba(250, 176, 5, 0.10), transparent 60%),
      linear-gradient(180deg, var(--surface-2), var(--surface));
    border: 1px solid var(--border-strong); border-radius: var(--radius-lg);
    box-shadow: var(--shadow-pop);
    padding: var(--space-4);
    display: flex; flex-direction: column; gap: var(--space-3);
    animation: hero-rise 0.7s ease 0.1s both;
  }
  .console-top {
    display: flex; align-items: center; gap: var(--space-2);
    padding-bottom: var(--space-3); border-bottom: 1px solid var(--border);
  }
  .bot { display: inline-flex; align-items: center; gap: var(--space-2); font-weight: 700; color: var(--text); font-size: var(--text-sm); }
  .bot-glyph { color: var(--brand); flex: none; }
  .env {
    font-size: var(--text-xs); color: var(--brand-soft);
    border: 1px solid var(--border-strong); border-radius: var(--radius-pill);
    padding: 2px var(--space-2); letter-spacing: 0.04em;
  }
  .live {
    margin-left: auto; display: inline-flex; align-items: center; gap: var(--space-2);
    font-size: var(--text-xs); color: var(--green); text-transform: uppercase; letter-spacing: 0.06em;
  }
  .live-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--green); animation: pulse-green 2s ease-out infinite; }

  .tape { display: flex; flex-direction: column; gap: var(--space-2); }
  .line {
    display: grid; grid-template-columns: auto 1fr auto; align-items: center; gap: var(--space-3);
    padding: var(--space-2); border-radius: var(--radius-sm);
    background: rgba(255, 249, 219, 0.02); border-left: 2px solid var(--border-strong);
    font-size: var(--text-sm);
    animation: hero-rise 0.5s ease both;
  }
  .line.buy { border-left-color: var(--brand); }
  .line.sell { border-left-color: var(--green); }
  .line:nth-child(1) { animation-delay: 0.30s; }
  .line:nth-child(2) { animation-delay: 0.48s; }
  .line:nth-child(3) { animation-delay: 0.66s; }
  .time { color: var(--muted); font-variant-numeric: tabular-nums; }
  .evt { color: var(--text); }
  .amt { font-weight: 700; color: var(--brand); font-variant-numeric: tabular-nums; }
  .amt.up { color: var(--green); }
  .tag {
    font-size: var(--text-xs); color: var(--muted);
    border: 1px solid var(--border); border-radius: var(--radius-pill); padding: 1px var(--space-2);
  }
  .console-foot { margin: 0; font-family: var(--font-sans); font-size: var(--text-xs); color: var(--muted); line-height: 1.5; }

  @keyframes hero-rise { from { opacity: 0; transform: translateY(10px); } to { opacity: 1; transform: none; } }
  @keyframes pulse-gold {
    0% { box-shadow: 0 0 0 0 rgba(255, 212, 59, 0.45); }
    70% { box-shadow: 0 0 0 6px rgba(255, 212, 59, 0); }
    100% { box-shadow: 0 0 0 0 rgba(255, 212, 59, 0); }
  }
  @keyframes pulse-green {
    0% { box-shadow: 0 0 0 0 rgba(43, 214, 106, 0.5); }
    70% { box-shadow: 0 0 0 6px rgba(43, 214, 106, 0); }
    100% { box-shadow: 0 0 0 0 rgba(43, 214, 106, 0); }
  }

  /* Generic content block */
  .block { max-width: var(--page-max); margin: 0 auto; padding: var(--space-6) var(--space-5); }
  .head { text-align: center; max-width: 60ch; margin: 0 auto var(--space-6); display: flex; flex-direction: column; gap: var(--space-2); }
  .head h2 { font-size: clamp(1.4rem, 3vw, 2rem); font-weight: 800; }
  .head-sub { color: var(--muted); line-height: 1.6; }

  .grid { display: grid; gap: var(--space-4); }
  /* 3 steps stay a single even row (3-up) down to the mobile breakpoint, so the tablet width never
     orphans the 3rd card into an empty second row. Features (even count of 4) reflow freely. */
  .steps { grid-template-columns: repeat(3, 1fr); }
  .feats { grid-template-columns: repeat(auto-fit, minmax(230px, 1fr)); }

  .card h3 { font-size: var(--text-md); font-weight: 800; margin: 0 0 var(--space-2); }
  .card p { color: var(--muted); line-height: 1.6; margin: 0; }
  .step { border-top: 2px solid var(--brand); }

  /* Security card */
  .secure { max-width: 760px; margin: 0 auto; }
  .secure h2 { font-size: clamp(1.4rem, 3vw, 1.9rem); font-weight: 800; }
  .secure-lead { color: var(--muted); line-height: 1.6; margin: var(--space-3) 0 var(--space-4); }
  .checks { list-style: none; padding: 0; margin: 0; display: grid; gap: var(--space-3); }
  .checks li { display: flex; align-items: flex-start; gap: var(--space-3); line-height: 1.55; }
  .tick {
    flex: none; width: 1.4rem; height: 1.4rem; border-radius: 50%;
    display: grid; place-items: center;
    background: rgba(250, 176, 5, 0.14); color: var(--brand);
    font-size: var(--text-sm); font-weight: 800; margin-top: 2px;
  }

  /* Risk note */
  .risk { max-width: 760px; margin: 0 auto; background: var(--surface-2); }
  .risk h3 { color: var(--brand-soft); }

  /* Final CTA */
  .final {
    max-width: 760px; margin: 0 auto; text-align: center;
    display: flex; flex-direction: column; align-items: center; gap: var(--space-3);
    background: radial-gradient(600px 200px at 50% 0%, rgba(250, 176, 5, 0.12), transparent), var(--surface);
    border-color: var(--border-strong);
  }
  .final h2 { font-size: clamp(1.4rem, 3vw, 1.9rem); font-weight: 800; }
  .final .lg { margin-top: var(--space-2); }

  /* Stack the split: claim first, then the robot console as proof. */
  @media (max-width: 860px) {
    .hero { grid-template-columns: 1fr; gap: var(--space-6); }
    .hero-copy { align-items: center; text-align: center; }
    .sub { max-width: 56ch; }
    .cta, .trust { justify-content: center; }
    .console { width: 100%; max-width: 460px; margin: 0 auto; }
  }

  @media (max-width: 600px) {
    .nav { justify-content: center; }
    .hero { padding: var(--space-6) var(--space-5) var(--space-5); }
    .steps { grid-template-columns: 1fr; }
  }

  @media (prefers-reduced-motion: reduce) {
    .hero-copy, .console, .line { animation: none; }
    .status-dot, .live-dot { animation: none; }
  }
</style>
