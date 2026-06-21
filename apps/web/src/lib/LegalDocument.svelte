<script lang="ts">
  // Public, permanently-reachable legal page (#/terms, #/privacy). Renders the full Terms of Use or
  // Privacy Policy from the i18n layer, so it is available before login, after login, and from the
  // consent gate — not only inside the acceptance modal. Same source text as the gate (terms reuse the
  // `agreement.*` keys); the Privacy Policy has its own fuller `privacy.*` set.
  import { navigate } from './stores'
  import { t } from './i18n'
  import LanguageDropdown from './LanguageDropdown.svelte'

  export let doc: 'terms' | 'privacy' = 'terms'

  const termsSections = [
    'service', 'eligibility', 'nonCustodial', 'risk', 'userResponsibilities',
    'paid', 'ads', 'privacy', 'liability', 'termination', 'changes', 'law'
  ]
  const privacySections = [
    'controller', 'data', 'bases', 'use', 'sharing', 'international',
    'retention', 'security', 'cookies', 'rights', 'children', 'contact'
  ]

  $: prefix = doc === 'privacy' ? 'privacy' : 'agreement'
  $: sections = doc === 'privacy' ? privacySections : termsSections
</script>

<div class="wrap">
  <div class="bar">
    <button class="brand" type="button" on:click={() => navigate('dashboard')}>Coin<span>Hub</span></button>
    <LanguageDropdown compact />
  </div>

  <article class="card doc">
    <button type="button" class="link-btn back" on:click={() => navigate('dashboard')}>← {$t('legal.back')}</button>
    <h1>{$t(`${prefix}.title`)}</h1>
    <p class="muted effective">{$t(`${prefix}.effective`)}</p>
    <p class="intro">{$t(`${prefix}.intro`)}</p>

    {#each sections as key}
      <section class="doc-section">
        <h2>{$t(`${prefix}.${key}Title`)}</h2>
        <p>{$t(`${prefix}.${key}Body`)}</p>
      </section>
    {/each}

    <div class="cross-links">
      {#if doc === 'terms'}
        <button type="button" class="link-btn" on:click={() => navigate('privacy')}>{$t('legal.viewPrivacy')} →</button>
      {:else}
        <button type="button" class="link-btn" on:click={() => navigate('terms')}>{$t('legal.viewTerms')} →</button>
      {/if}
    </div>
  </article>
</div>

<style>
  .wrap { max-width: 760px; margin: 0 auto; padding: var(--space-5); }
  .bar { display: flex; justify-content: space-between; align-items: center; margin-bottom: var(--space-4); }
  .brand { gap: 0; background: transparent; border: none; padding: 0; min-height: 24px; color: var(--text); font-weight: 800; font-size: var(--text-lg); }
  .brand span { color: var(--brand); }
  .doc { line-height: 1.6; }
  .back { background: transparent; border: none; color: var(--brand); font-weight: 700; padding: 0; min-height: 24px; cursor: pointer; font-size: var(--text-sm); }
  .doc h1 { font-size: var(--text-xl); margin-top: var(--space-3); }
  .effective { margin-top: var(--space-1); font-size: var(--text-xs); }
  .intro { margin-top: var(--space-3); color: var(--muted); font-size: var(--text-sm); }
  .doc-section { margin-top: var(--space-5); }
  .doc-section h2 { font-size: var(--text-base); font-weight: 800; color: var(--brand-soft); }
  .doc-section p { margin-top: var(--space-2); color: var(--muted); font-size: var(--text-sm); }
  .cross-links { margin-top: var(--space-6); padding-top: var(--space-4); border-top: 1px solid var(--border); }
  .cross-links .link-btn { background: transparent; border: none; color: var(--brand); font-weight: 700; padding: 0; min-height: 24px; cursor: pointer; font-size: var(--text-sm); }
</style>
