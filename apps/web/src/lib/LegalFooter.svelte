<script lang="ts">
  import { t } from './i18n'
  import { navigate } from './stores'
  import Collapsible from './Collapsible.svelte'
</script>

<footer class="legal">
  <p class="short">{$t('legal.short')}</p>
  <Collapsible variant="help" title={$t('legal.title')}>
    <p>{$t('legal.p1')}</p>
    <p>{$t('legal.p2')}</p>
    <p>{$t('legal.p3')}</p>
    <p>{$t('legal.p4')}</p>
  </Collapsible>
  <nav class="links">
    <button type="button" class="link-btn" on:click={() => navigate('terms')}>{$t('legal.viewTerms')}</button>
    <span aria-hidden="true">·</span>
    <button type="button" class="link-btn" on:click={() => navigate('privacy')}>{$t('legal.viewPrivacy')}</button>
  </nav>
</footer>

<style>
  .legal { max-width: var(--page-max); margin: var(--space-5) auto 0; padding: var(--space-5); border-top: 1px solid var(--border); color: var(--muted); font-size: var(--text-xs); line-height: 1.6; }
  .short { color: var(--muted); }
  .links { display: flex; align-items: center; gap: var(--space-2); margin-top: var(--space-3); color: var(--muted); }
  .links .link-btn { background: transparent; border: none; padding: 0; min-height: 24px; color: var(--brand); font-weight: 700; cursor: pointer; font-size: var(--text-xs); }
  .links .link-btn:hover { text-decoration: underline; }
</style>
