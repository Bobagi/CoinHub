<script lang="ts">
  import { t } from './i18n'
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
</footer>

<style>
  .legal { max-width: var(--page-max); margin: var(--space-5) auto 0; padding: var(--space-5); border-top: 1px solid var(--border); color: var(--muted); font-size: var(--text-xs); line-height: 1.6; }
  .short { color: var(--muted); }
</style>
