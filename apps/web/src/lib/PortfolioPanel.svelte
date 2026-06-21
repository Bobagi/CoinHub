<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from './api'
  import { t } from './i18n'
  import { notifyError } from './stores'
  import Pagination from './Pagination.svelte'
  import Collapsible from './Collapsible.svelte'

  type AssetTable = { table_name: string; header: string[]; rows: string[][]; error?: string }

  let walletUrl = ''
  let savedUrl = ''
  let saving = false
  let sourceMessage = ''
  let assets: AssetTable[] = []
  let dividends: { asset: string; date_com: string }[] = []
  let busyAssets = false
  let busyDividends = false
  let assetsError = ''
  let dividendsError = ''

  // Per-table pagination (default 10 rows/page). One page+size per asset table, plus the dividends table.
  let assetPages: number[] = []
  let assetSizes: number[] = []
  let dividendsPage = 1
  let dividendsPageSize = 10

  onMount(async () => {
    try {
      savedUrl = (await api.getPortfolioSource()).wallet_url
      walletUrl = savedUrl
    } catch {
      /* not set yet */
    }
  })

  async function saveSource() {
    saving = true
    sourceMessage = ''
    try {
      await api.savePortfolioSource(walletUrl)
      savedUrl = walletUrl.trim()
      sourceMessage = $t('portfolio.saved')
    } catch (e) {
      notifyError(e)
    } finally {
      saving = false
    }
  }

  async function loadAssets() {
    busyAssets = true
    assetsError = ''
    try {
      assets = (await api.getPortfolioAssets()).tables || []
      assetPages = assets.map(() => 1)
      assetSizes = assets.map(() => 10)
    } catch (e) {
      notifyError(e)
    } finally {
      busyAssets = false
    }
  }

  async function loadDividends() {
    busyDividends = true
    dividendsError = ''
    try {
      dividends = (await api.getPortfolioDividends()).results || []
    } catch (e) {
      notifyError(e)
    } finally {
      busyDividends = false
    }
  }
</script>

<section class="card">
  <div class="card-header">
    <span class="card-title">{$t('portfolio.title')}</span>
    <span class="card-subtitle">{$t('portfolio.subtitle')}</span>
  </div>
  <Collapsible variant="help" title={$t('help.summary')}><p>{$t('portfolio.help')}</p></Collapsible>

  <input class="mt-4" bind:value={walletUrl} placeholder={$t('portfolio.placeholder')} />
  <div class="actions">
    <button class="btn-primary" on:click={saveSource} disabled={saving}>{saving ? $t('common.saving') : $t('portfolio.saveUrl')}</button>
    <button class="ghost" on:click={loadAssets} disabled={busyAssets || !savedUrl}>{busyAssets ? $t('portfolio.loading') : $t('portfolio.loadAssets')}</button>
    <button class="ghost" on:click={loadDividends} disabled={busyDividends || !savedUrl}>{busyDividends ? $t('portfolio.loading') : $t('portfolio.dividends')}</button>
  </div>
  {#if sourceMessage}<p class="muted">{sourceMessage}</p>{/if}
  {#if assetsError}<p class="error">{assetsError}</p>{/if}
  {#if dividendsError}<p class="error">{dividendsError}</p>{/if}

  {#each assets as table, tableIndex}
    <h3>{table.table_name}</h3>
    {#if table.error}
      <p class="error">{table.error}</p>
    {:else}
      <div class="ptable">
        {#if table.header && table.header.length}
          <div class="prow phead">{#each table.header as cell}<div>{cell}</div>{/each}</div>
        {/if}
        {#each table.rows.slice(((assetPages[tableIndex] || 1) - 1) * (assetSizes[tableIndex] || 10), (assetPages[tableIndex] || 1) * (assetSizes[tableIndex] || 10)) as row}
          <div class="prow">{#each row as cell}<div>{cell}</div>{/each}</div>
        {/each}
      </div>
      <Pagination total={table.rows.length} bind:page={assetPages[tableIndex]} bind:pageSize={assetSizes[tableIndex]} />
    {/if}
  {/each}

  {#if dividends.length}
    <h3>{$t('portfolio.upcoming')}</h3>
    <div class="ptable">
      <div class="prow phead"><div>{$t('portfolio.asset')}</div><div>{$t('portfolio.date')}</div></div>
      {#each dividends.slice((dividendsPage - 1) * dividendsPageSize, dividendsPage * dividendsPageSize) as dividend}
        <div class="prow"><div>{dividend.asset}</div><div>{dividend.date_com}</div></div>
      {/each}
    </div>
    <Pagination total={dividends.length} bind:page={dividendsPage} bind:pageSize={dividendsPageSize} />
  {/if}
</section>

<style>
  .actions { display: flex; gap: var(--space-2); margin-top: var(--space-3); flex-wrap: wrap; }
  h3 { margin: var(--space-4) 0 var(--space-2); }
  .ptable { display: flex; flex-direction: column; overflow-x: auto; }
  .prow { display: flex; gap: var(--space-2); padding: var(--space-2) var(--space-1); border-bottom: 1px solid var(--border); }
  .prow > div { flex: 1; min-width: 90px; font-size: var(--text-sm); white-space: nowrap; }
  .phead { color: var(--muted); font-weight: 700; }
</style>
