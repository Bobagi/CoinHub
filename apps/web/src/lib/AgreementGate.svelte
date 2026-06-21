<script lang="ts">
  // Blocking consent gate. Shown over the whole app whenever the signed-in user has NOT accepted the
  // version of the Terms of Use + Privacy Policy currently in force (currentUser.terms_accepted=false).
  // It applies uniformly to email sign-ups, Google sign-ups and existing users on a terms update, so
  // nobody reaches the dashboard without an on-record, server-side acceptance. Until they accept they
  // can only accept or sign out.
  import { api } from './api'
  import { currentUser, binanceStatus, navigate, notifyError } from './stores'
  import { t } from './i18n'
  import LanguageDropdown from './LanguageDropdown.svelte'

  // Open the full, standalone Terms/Privacy pages from inside the gate. Navigating away then back
  // re-shows the gate (terms_accepted is still false), so nothing is lost.
  function openTerms() { navigate('terms') }
  function openPrivacy() { navigate('privacy') }

  let agreed = false
  let busy = false

  // The legal document is rendered as an ordered list of titled sections; each maps to an i18n key
  // pair (title/body) so the whole text is translated per locale.
  const sections = [
    'service', 'eligibility', 'nonCustodial', 'risk', 'userResponsibilities',
    'paid', 'ads', 'privacy', 'liability', 'termination', 'changes', 'law'
  ]

  async function accept() {
    if (!agreed || busy) return
    busy = true
    try {
      const user = await api.acceptAgreement()
      currentUser.set(user)
    } catch (e) {
      notifyError(e)
    } finally {
      busy = false
    }
  }

  async function declineAndSignOut() {
    busy = true
    try {
      await api.logout()
    } catch {
      /* ignore */
    }
    binanceStatus.set(null)
    currentUser.set(null)
    navigate('dashboard')
  }
</script>

<div class="wrap">
  <div class="card gate">
    <div class="top">
      <div class="brand">Coin<span>Hub</span></div>
      <LanguageDropdown compact />
    </div>

    <h1 class="title">{$t('agreement.title')}</h1>
    <p class="muted version">{$t('agreement.version', { version: $currentUser?.terms_version || '' })}</p>
    <p class="intro">{$t('agreement.intro')}</p>
    <p class="full-links">
      <button type="button" class="link-btn" on:click={openTerms}>{$t('legal.viewTerms')}</button>
      <span aria-hidden="true">·</span>
      <button type="button" class="link-btn" on:click={openPrivacy}>{$t('legal.viewPrivacy')}</button>
    </p>

    <div class="doc" role="region" aria-label={$t('agreement.title')}>
      {#each sections as key}
        <section class="doc-section">
          <h2>{$t(`agreement.${key}Title`)}</h2>
          <p>{$t(`agreement.${key}Body`)}</p>
        </section>
      {/each}
    </div>

    <label class="accept-row">
      <input type="checkbox" bind:checked={agreed} />
      <span>{$t('agreement.checkbox')}</span>
    </label>

    <button class="btn-primary btn-block mt-4" disabled={!agreed || busy} on:click={accept}>
      {busy ? $t('agreement.saving') : $t('agreement.accept')}
    </button>
    <button type="button" class="link-btn decline" disabled={busy} on:click={declineAndSignOut}>
      {$t('agreement.decline')}
    </button>
  </div>
</div>

<style>
  .wrap { display: grid; place-items: center; min-height: 100vh; padding: var(--space-5); }
  .gate { width: 100%; max-width: 640px; }
  .top { display: flex; justify-content: space-between; align-items: center; }
  .brand { font-size: var(--text-xl); font-weight: 800; }
  .brand span { color: var(--brand); }
  .title { font-size: var(--text-xl); margin-top: var(--space-4); }
  .version { margin-top: var(--space-1); font-size: var(--text-xs); }
  .intro { margin-top: var(--space-3); color: var(--muted); font-size: var(--text-sm); line-height: 1.6; }
  .full-links { display: flex; align-items: center; gap: var(--space-2); margin-top: var(--space-2); }
  .full-links .link-btn { background: transparent; border: none; padding: 0; min-height: 24px; color: var(--brand); font-weight: 700; cursor: pointer; font-size: var(--text-xs); }
  .full-links .link-btn:hover { text-decoration: underline; }
  .doc {
    margin-top: var(--space-4);
    max-height: 46vh;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    padding: var(--space-4) var(--space-4);
  }
  .doc-section + .doc-section { margin-top: var(--space-4); }
  .doc-section h2 { font-size: var(--text-sm); font-weight: 800; color: var(--brand-soft); }
  .doc-section p { margin-top: var(--space-2); color: var(--muted); font-size: var(--text-sm); line-height: 1.6; }
  .accept-row { display: flex; align-items: flex-start; gap: var(--space-3); margin-top: var(--space-4); font-size: var(--text-sm); line-height: 1.5; cursor: pointer; }
  .accept-row input { margin-top: 3px; flex: none; }
  .decline { display: block; margin: var(--space-3) auto 0; background: transparent; border: none; color: var(--muted); font-size: var(--text-sm); cursor: pointer; min-height: 24px; }
  .decline:hover:not(:disabled) { text-decoration: underline; color: var(--text); }
</style>
