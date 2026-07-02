<script lang="ts">
  import { t, intlLocale } from './i18n'
  import { formatMoney } from './money'

  // The user's free spot balance of the pair's quote currency, shown above amount fields so "how
  // much can I spend?" is answered before typing a value. Used by the Buy card and the robot editor.
  export let quote = ''
  export let free = 0
</script>

<div class="balance-callout">
  <span>{$t('buy.availableLabel', { quote })}</span>
  <strong class="bc-value">{formatMoney(free, quote, $intlLocale)}</strong>
</div>

<style>
  .balance-callout { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); background: var(--surface-2); border: 1px solid var(--border); border-left: 3px solid var(--green); border-radius: var(--radius-md); padding: var(--space-2) var(--space-3); font-size: var(--text-sm); margin-top: var(--space-3); }
  .bc-value { color: var(--green); font-weight: 800; white-space: nowrap; }
</style>
