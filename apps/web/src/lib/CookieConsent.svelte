<script lang="ts">
  // Cookie / non-essential-script consent banner (LGPD). The strictly-necessary session cookie needs
  // no consent; everything else (analytics now, advertising later) is opt-in. The banner is shown
  // whenever a non-essential script needs consent (`consentRequired`) and the user hasn't decided yet.
  // The choice persists in localStorage; analytics/ads scripts load only when `cookieConsent === 'accepted'`
  // (see lib/analytics.ts). Accept and Reject are given equal prominence (no dark patterns).
  import { consentRequired, cookieConsent, setCookieConsent, navigate } from './stores'
  import { t } from './i18n'

  $: visible = consentRequired && $cookieConsent === null
</script>

{#if visible}
  <div class="cookie-banner" role="region" aria-label={$t('cookie.title')}>
    <p class="msg">
      {$t('cookie.message')}
      <button type="button" class="inline-link" on:click={() => navigate('privacy')}>{$t('cookie.learnMore')}</button>
    </p>
    <div class="actions">
      <button type="button" class="ghost btn-sm" on:click={() => setCookieConsent('rejected')}>{$t('cookie.reject')}</button>
      <button type="button" class="btn-primary btn-sm" on:click={() => setCookieConsent('accepted')}>{$t('cookie.accept')}</button>
    </div>
  </div>
{/if}

<style>
  .cookie-banner {
    position: fixed; left: var(--space-4); right: var(--space-4); bottom: var(--space-4); z-index: 60;
    max-width: 720px; margin-inline: auto;
    display: flex; align-items: center; gap: var(--space-4); flex-wrap: wrap;
    background: var(--surface-2); border: 1px solid var(--border-strong);
    border-radius: var(--radius-md); padding: var(--space-4); box-shadow: var(--shadow-pop);
  }
  .msg { flex: 1 1 280px; color: var(--muted); font-size: var(--text-sm); line-height: 1.55; }
  .inline-link { background: transparent; border: none; padding: 0; color: var(--brand); font-weight: 700; cursor: pointer; font-size: var(--text-sm); }
  .inline-link:hover { text-decoration: underline; }
  .actions { display: flex; gap: var(--space-2); margin-left: auto; }
</style>
