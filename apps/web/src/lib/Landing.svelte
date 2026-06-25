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
    <!-- Hero: the thesis — non-custodial, your keys, your rules. -->
    <section class="hero">
      <span class="eyebrow">{$t('landing.hero.eyebrow')}</span>
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

  /* Hero */
  .hero {
    max-width: 820px; margin: 0 auto;
    padding: var(--space-7) var(--space-5);
    text-align: center;
    display: flex; flex-direction: column; align-items: center; gap: var(--space-4);
  }
  .eyebrow {
    display: inline-block;
    border: 1px solid var(--border-strong); border-radius: var(--radius-pill);
    padding: var(--space-1) var(--space-3);
    font-size: var(--text-xs); color: var(--brand-soft);
    text-transform: uppercase; letter-spacing: 0.08em;
  }
  .hero h1 { font-size: clamp(2rem, 5vw, 3.25rem); line-height: 1.1; font-weight: 800; margin: 0; }
  .accent {
    background: linear-gradient(90deg, var(--brand), var(--brand-strong));
    -webkit-background-clip: text; background-clip: text; color: transparent;
  }
  .sub { color: var(--muted); font-size: var(--text-md); line-height: 1.6; max-width: 60ch; margin: 0; }
  .cta { display: flex; flex-wrap: wrap; gap: var(--space-3); justify-content: center; margin-top: var(--space-2); }
  .lg { height: 3rem; padding: 0 var(--space-5); font-size: var(--text-md); }
  .trust {
    list-style: none; padding: 0; margin: var(--space-3) 0 0;
    display: flex; flex-wrap: wrap; gap: var(--space-2); justify-content: center;
  }
  .trust li {
    display: inline-flex; align-items: center; gap: var(--space-2);
    border: 1px solid var(--border); border-radius: var(--radius-pill);
    padding: var(--space-1) var(--space-3);
    font-size: var(--text-xs); color: var(--muted);
  }
  .trust li::before { content: ''; width: 6px; height: 6px; border-radius: 50%; background: var(--brand); }

  /* Generic content block */
  .block { max-width: var(--page-max); margin: 0 auto; padding: var(--space-6) var(--space-5); }
  .head { text-align: center; max-width: 60ch; margin: 0 auto var(--space-6); display: flex; flex-direction: column; gap: var(--space-2); }
  .head h2 { font-size: clamp(1.4rem, 3vw, 2rem); font-weight: 800; }
  .head-sub { color: var(--muted); line-height: 1.6; }

  .grid { display: grid; gap: var(--space-4); }
  .steps { grid-template-columns: repeat(auto-fit, minmax(230px, 1fr)); }
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

  @media (max-width: 600px) {
    .nav { justify-content: center; }
    .hero { padding-top: var(--space-6); }
  }
</style>
