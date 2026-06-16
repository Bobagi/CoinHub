<script lang="ts">
  import { t } from './i18n'

  // Reusable table pager: a page-size dropdown (10→50, default 10) plus prev/next so long, ever-growing
  // tables (history, positions, …) don't render every row at once. `page` and `pageSize` are bindable so
  // the parent slices its own rows; this component owns only the controls and the bounds math.
  export let total = 0
  export let page = 1
  export let pageSize = 10

  const sizeOptions = [10, 20, 30, 40, 50]

  $: pageCount = Math.max(1, Math.ceil(total / pageSize))
  // Keep the page in range when the data shrinks or the size grows.
  $: if (page > pageCount) page = pageCount
  $: if (page < 1) page = 1
  $: startIndex = total === 0 ? 0 : (page - 1) * pageSize + 1
  $: endIndex = Math.min(page * pageSize, total)

  function changeSize(event: Event) {
    pageSize = Number((event.target as HTMLSelectElement).value)
    page = 1
  }
</script>

{#if total > sizeOptions[0]}
  <div class="pager">
    <label class="pager-size">
      <span>{$t('pager.show')}</span>
      <select value={pageSize} on:change={changeSize}>
        {#each sizeOptions as option}<option value={option}>{option}</option>{/each}
      </select>
    </label>
    <span class="pager-info">{startIndex}–{endIndex} {$t('pager.of')} {total}</span>
    <div class="pager-nav">
      <button type="button" class="ghost pbtn" disabled={page <= 1} on:click={() => (page -= 1)} aria-label={$t('pager.prev')}>‹</button>
      <span class="pager-page">{page} / {pageCount}</span>
      <button type="button" class="ghost pbtn" disabled={page >= pageCount} on:click={() => (page += 1)} aria-label={$t('pager.next')}>›</button>
    </div>
  </div>
{/if}

<style>
  .pager {
    display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap;
    gap: var(--space-3); margin-top: var(--space-3); font-size: var(--text-sm); color: var(--muted);
  }
  .pager-size { display: inline-flex; align-items: center; gap: var(--space-2); }
  .pager-size select { height: 2rem; padding: 0 var(--space-2); width: auto; }
  .pager-info { font-variant-numeric: tabular-nums; }
  .pager-nav { display: inline-flex; align-items: center; gap: var(--space-2); }
  .pbtn { height: 2rem; padding: 0 var(--space-3); font-size: var(--text-md); }
  .pager-page { font-variant-numeric: tabular-nums; min-width: 3.5em; text-align: center; }
</style>
