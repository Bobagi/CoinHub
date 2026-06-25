<script lang="ts">
  import { t } from './i18n'
  import { systemStatus } from './stores'

  // Shown only when the trading automation can't run right now, so users understand why a bot didn't
  // buy/sell (Binance rate-limit, IP ban, or a stalled worker). The header light turns red in tandem.
  $: notOperational = !!$systemStatus && $systemStatus.operational === false
  $: reasons = $systemStatus?.reasons ?? []
</script>

{#if notOperational}
  <div class="status-banner-outer" role="status" aria-live="polite">
    <div class="status-banner">
      <span class="sb-title">⚠ {$t('status.bannerTitle')}</span>
      <ul class="sb-list">
        {#each reasons as reason}
          <li>{$t('status.' + reason.code)}</li>
        {/each}
      </ul>
    </div>
  </div>
{/if}

<style>
  .status-banner-outer { max-width: var(--page-max); margin: 0 auto; padding: var(--space-4) var(--space-5) 0; }
  .status-banner {
    display: flex; flex-direction: column; gap: var(--space-2);
    background: rgba(255, 90, 95, 0.1); border: 1px solid rgba(255, 90, 95, 0.38);
    color: var(--red); border-radius: var(--radius-md); padding: var(--space-3) var(--space-4);
  }
  .sb-title { font-weight: 700; }
  .sb-list { margin: 0; padding-left: var(--space-5); color: var(--text); }
  .sb-list li { line-height: 1.55; }
</style>
