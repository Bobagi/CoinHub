<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { api, type TradingSettings, type CredentialStatus, type Operation, type Execution, type Robot } from './api'
  import { binanceStatus, currentUser, pushToast, notifyError } from './stores'
  import { t, intlLocale, formatDateTime, formatDate, translateError } from './i18n'
  import AllocationPanel from './AllocationPanel.svelte'
  import ProfitabilityPanel from './ProfitabilityPanel.svelte'
  import PortfolioPanel from './PortfolioPanel.svelte'
  import LegalFooter from './LegalFooter.svelte'
  import SymbolAutocomplete from './SymbolAutocomplete.svelte'
  import LockOverlay from './LockOverlay.svelte'
  import Pagination from './Pagination.svelte'

  let activeTab: 'connection' | 'trade' | 'b3' = 'trade'
  let opsView: 'positions' | 'history' = 'positions'
  let allocView: 'allocation' | 'profit' = 'allocation'
  const environments = ['TESTNET', 'PRODUCTION']

  // Common quote assets (the coin you pay WITH), longest first so e.g. FDUSD wins over USD.
  const knownQuoteAssets = ['FDUSD', 'USDT', 'USDC', 'BUSD', 'TUSD', 'DAI', 'BRL', 'EUR', 'GBP', 'AUD', 'TRY', 'BTC', 'ETH', 'BNB']
  function quoteAssetOf(symbol: string): string {
    const upper = (symbol || '').toUpperCase()
    for (const quote of knownQuoteAssets) {
      if (upper.length > quote.length && upper.endsWith(quote)) return quote
    }
    return ''
  }

  let settings: TradingSettings | null = null
  let credentials: CredentialStatus | null = null
  let operations: Operation[] = []
  let executions: Execution[] = []
  let symbols: string[] = []
  let loadingError = ''

  // credEnv is the environment currently selected in the connection tab (which the key form targets).
  let credEnv = 'TESTNET'
  let credKey = ''
  let credSecret = ''
  let credMsg = ''
  let credErr = ''
  let credBusy = false
  // Removing stored keys: two-step inline confirm.
  let confirmingDelete = false
  let credDeleteBusy = false

  let envBusy = ''
  let envErr = ''

  let settingsMsg = ''
  let settingsErr = ''
  let settingsBusy = false
  let botToggleBusy = false

  let dailyHourLocal = 4
  const hours = Array.from({ length: 24 }, (_, index) => index)
  const localTimeZone = typeof Intl !== 'undefined' ? Intl.DateTimeFormat().resolvedOptions().timeZone : 'UTC'
  const tzOffset = timezoneOffsetLabel()

  let tradeSymbol = 'BTCUSDT'
  let tradeAmount = 15
  let tradeTarget = 1.5
  let tradePrice: number | null = null
  let tradeFilters: { min_notional: number; tick_size: number; step_size: number } | null = null
  let tradeErr = ''
  let tradeBusy = false

  let sellBusyId: number | null = null
  let placeSellBusyId: number | null = null

  // Robots: each is one automated bot for a single coin. The settings panel is hidden until a robot
  // is selected from the list.
  let robots: Robot[] = []
  let robotLimit = 1 // 0 = unlimited (admins)
  let maxOrderQuoteAmount = 0 // global per-order spending ceiling (from the API); 0 = no cap
  let selectedRobotId: number | null = null
  let creatingRobot = false
  let newRobotSymbol = 'BTCUSDT'
  let robotDraft: Robot | null = null
  let robotDailyHourLocal = 4
  let robotBusy = false
  let robotMsg = ''
  let robotErr = ''

  const fmt = (value: number | null) =>
    value === null || value === undefined ? '—' : value.toLocaleString(undefined, { maximumFractionDigits: 8 })

  function formatHour(hour: number) {
    return String(hour).padStart(2, '0') + ':00'
  }
  function utcHourToLocal(utcHour: number) {
    const date = new Date()
    date.setUTCHours(utcHour, 0, 0, 0)
    return date.getHours()
  }
  function localHourToUtc(localHour: number) {
    const date = new Date()
    date.setHours(localHour, 0, 0, 0)
    return date.getUTCHours()
  }
  function timezoneOffsetLabel() {
    const totalMinutes = -new Date().getTimezoneOffset()
    const sign = totalMinutes >= 0 ? '+' : '-'
    const absMinutes = Math.abs(totalMinutes)
    const wholeHours = Math.floor(absMinutes / 60)
    const minutes = absMinutes % 60
    return `GMT${sign}${wholeHours}${minutes ? ':' + String(minutes).padStart(2, '0') : ''}`
  }
  function nextDailyRun(utcHour: number) {
    const now = new Date()
    const next = new Date()
    next.setUTCHours(utcHour, 0, 0, 0)
    if (next <= now) next.setUTCDate(next.getUTCDate() + 1)
    return next
  }

  $: botEnabled = !!settings && settings.daily_purchase_enabled
  $: botActive = botEnabled && !!settings && settings.capital_threshold > 0
  $: connected = !!credentials?.has_active_credential
  $: isAdmin = !!$currentUser?.is_admin
  // The Trade tab needs the ACTIVE environment to have keys before anything can run. (You can switch
  // to an environment without keys; it is then "active but not connected" and the tab stays locked.)
  $: tradeLocked = !credentials?.has_active_credential
  $: selectedRobot = robots.find((robot) => robot.id === selectedRobotId) || null
  $: canCreateRobot = robotLimit === 0 || robots.length < robotLimit
  $: robotProductionNeedsLive = credentials?.active_environment === 'PRODUCTION' && !!settings && !settings.live_trading_enabled

  // Profitability (for the active environment). Costs/proceeds come from the operations table (the
  // source of truth); the site-vs-robots split comes from executions (initiated_by). Buys = money in;
  // completed sales (SELL) = money back; SELL_ORDER_PLACED records are just placed orders, not sales.
  $: investedTotal = operations.reduce((sum, op) => sum + op.quantity * op.purchase_price_per_unit, 0)
  $: openCostTotal = operations
    .filter((op) => op.status === 'OPEN')
    .reduce((sum, op) => sum + op.quantity * op.purchase_price_per_unit, 0)
  $: soldOperations = operations.filter((op) => op.status === 'SOLD' && op.sell_price_per_unit != null)
  $: realizedProceeds = soldOperations.reduce((sum, op) => sum + op.quantity * (op.sell_price_per_unit as number), 0)
  $: realizedCost = soldOperations.reduce((sum, op) => sum + op.quantity * op.purchase_price_per_unit, 0)
  $: realizedResult = realizedProceeds - realizedCost
  $: spentBySite = executions.filter((e) => e.success && e.operation_type === 'BUY' && e.initiated_by === 'USER').reduce((s, e) => s + e.total_value, 0)
  $: spentByRobots = executions.filter((e) => e.success && (e.operation_type === 'DAILY_BUY' || e.operation_type === 'BUY') && e.initiated_by === 'BOT').reduce((s, e) => s + e.total_value, 0)
  $: earnedBySite = executions.filter((e) => e.success && e.operation_type === 'SELL' && e.initiated_by === 'USER').reduce((s, e) => s + e.total_value, 0)
  $: earnedByRobots = executions.filter((e) => e.success && e.operation_type === 'SELL' && e.initiated_by === 'BOT').reduce((s, e) => s + e.total_value, 0)
  $: acquiredBySymbol = aggregateQuantity(operations)
  $: hasProfitData = operations.length > 0
  function aggregateQuantity(list: Operation[]): { symbol: string; quantity: number }[] {
    const totals: Record<string, number> = {}
    for (const op of list) totals[op.symbol] = (totals[op.symbol] || 0) + op.quantity
    return Object.entries(totals)
      .map(([symbol, quantity]) => ({ symbol, quantity }))
      .sort((a, b) => b.quantity - a.quantity)
  }
  $: needsLiveWarning = botActive && credentials?.active_environment === 'PRODUCTION' && !!settings && !settings.live_trading_enabled
  $: nextRun = nextDailyRun(localHourToUtc(dailyHourLocal))
  $: nextRunLabel = nextRun.toLocaleString($intlLocale, { weekday: 'short', hour: '2-digit', minute: '2-digit' })
  $: hoursUntilNext = Math.max(1, Math.round((nextRun.getTime() - Date.now()) / 3600000))

  const isConfigured = (environment: string) => !!credentials?.configured_environments?.includes(environment)
  // The active environment (the user's choice), regardless of whether it has keys yet.
  const isActive = (environment: string) => credentials?.active_environment === environment

  function publishBinanceStatus(status: CredentialStatus) {
    binanceStatus.set({ has_active_credential: status.has_active_credential, active_environment: status.active_environment })
  }

  async function loadAll() {
    try {
      const [loadedSettings, loadedCredentials, loadedOperations] = await Promise.all([
        api.getSettings(),
        api.getCredentials(),
        api.getOperations()
      ])
      settings = loadedSettings
      credentials = loadedCredentials
      operations = loadedOperations
      publishBinanceStatus(loadedCredentials)
      if (loadedCredentials.has_active_credential) credEnv = loadedCredentials.active_environment
      dailyHourLocal = utcHourToLocal(loadedSettings.daily_purchase_hour_utc)
      tradeSymbol = loadedSettings.trading_pair_symbol || 'BTCUSDT'
      tradeTarget = loadedSettings.target_profit_percent || 1.5
      if (loadedSettings.capital_threshold > 0) tradeAmount = loadedSettings.capital_threshold
    } catch (e) {
      notifyError(e)
    }
  }

  async function loadExecutions() {
    try {
      executions = await api.getExecutions()
    } catch {
      executions = []
    }
  }

  async function loadSymbols() {
    try {
      symbols = (await api.getSymbols()).symbols || []
    } catch {
      symbols = []
    }
  }

  // Live prices for open positions → the per-row "if you sold now" P/L arrow + tooltip in Positions.
  let currentPrices: Record<string, number> = {}
  let pricesTimer: ReturnType<typeof setInterval> | undefined
  let pricedSymbolsKey = ''
  $: openSymbolsKey = [...new Set(operations.filter((o) => o.status === 'OPEN').map((o) => o.symbol))].sort().join(',')
  // Refetch prices whenever the set of open coins changes (a buy/sell adds or removes one).
  $: if (openSymbolsKey !== pricedSymbolsKey) {
    pricedSymbolsKey = openSymbolsKey
    loadPositionPrices()
  }

  async function loadPositionPrices() {
    const symbols = [...new Set(operations.filter((o) => o.status === 'OPEN').map((o) => o.symbol))]
    if (symbols.length === 0) {
      currentPrices = {}
      return
    }
    const entries = await Promise.all(
      symbols.map(async (symbol) => {
        try {
          return [symbol, (await api.getPrice(symbol)).price] as const
        } catch {
          return [symbol, currentPrices[symbol] || 0] as const
        }
      })
    )
    currentPrices = Object.fromEntries(entries)
  }

  // Unrealized P/L of an open position at the current price.
  function pnlPct(operation: Operation): number {
    const current = currentPrices[operation.symbol] || 0
    if (!current || !operation.purchase_price_per_unit) return 0
    return ((current - operation.purchase_price_per_unit) / operation.purchase_price_per_unit) * 100
  }
  function pnlValue(operation: Operation): number {
    const current = currentPrices[operation.symbol] || 0
    if (!current) return 0
    return (current - operation.purchase_price_per_unit) * operation.quantity
  }
  function pnlTooltip(operation: Operation): string {
    const current = currentPrices[operation.symbol] || 0
    return [
      $t('ops.pnlBuy', { price: fmt(operation.purchase_price_per_unit) }),
      $t('ops.pnlNow', { price: fmt(current) }),
      $t('ops.pnlProfit', {
        value: (pnlValue(operation) >= 0 ? '+' : '') + fmt(pnlValue(operation)),
        pct: (pnlPct(operation) >= 0 ? '+' : '') + pnlPct(operation).toFixed(2) + '%'
      })
    ].join('\n')
  }

  async function saveCredentials() {
    credBusy = true
    credMsg = ''
    credErr = ''
    try {
      await api.saveCredentials(credEnv, credKey, credSecret)
      credKey = ''
      credSecret = ''
      credMsg = $t('binance.validatedSaved')
      // Saving keys activates that environment, so reload everything for the now-active environment.
      await loadAll()
      await loadExecutions()
      loadSymbols()
      loadRobots()
      selectedRobotId = null
      creatingRobot = false
    } catch (e) {
      notifyError(e)
    } finally {
      credBusy = false
    }
  }

  // Permanently delete the stored keys for the selected environment.
  async function deleteCredentials() {
    credDeleteBusy = true
    credMsg = ''
    credErr = ''
    try {
      await api.deleteCredentials(credEnv)
      confirmingDelete = false
      credMsg = $t('binance.deleted')
      await loadAll()
      await loadExecutions()
      loadRobots()
      selectedRobotId = null
      creatingRobot = false
    } catch (e) {
      notifyError(e)
    } finally {
      credDeleteBusy = false
    }
  }

  // Selecting an environment makes it the active one — even if it has no keys yet (it then shows as
  // "active but not connected"). The key form below always targets the selected environment.
  async function selectEnvironment(environment: string) {
    credEnv = environment
    envErr = ''
    confirmingDelete = false
    if (isActive(environment)) return
    envBusy = environment
    try {
      await api.activateEnvironment(environment)
      await loadAll()
      await loadExecutions()
      loadSymbols()
      loadRobots()
      selectedRobotId = null
      creatingRobot = false
      credEnv = environment
    } catch (e) {
      notifyError(e)
    } finally {
      envBusy = ''
    }
  }

  async function saveSettings() {
    if (!settings) return
    settingsBusy = true
    settingsMsg = ''
    settingsErr = ''
    try {
      settings.daily_purchase_hour_utc = localHourToUtc(dailyHourLocal)
      settings = await api.saveSettings(settings)
      settingsMsg = $t('settings.saved')
    } catch (e) {
      notifyError(e)
    } finally {
      settingsBusy = false
    }
  }

  async function toggleBot() {
    if (!settings) return
    botToggleBusy = true
    settingsErr = ''
    settingsMsg = ''
    const previousValue = settings.daily_purchase_enabled
    try {
      settings.daily_purchase_enabled = !previousValue
      settings.daily_purchase_hour_utc = localHourToUtc(dailyHourLocal)
      settings = await api.saveSettings(settings)
    } catch (e) {
      if (settings) settings.daily_purchase_enabled = previousValue
      notifyError(e)
    } finally {
      botToggleBusy = false
    }
  }

  async function checkPrice() {
    if (!tradeSymbol) return
    tradeErr = ''
    try {
      tradePrice = (await api.getPrice(tradeSymbol)).price
    } catch (e) {
      notifyError(e)
      tradePrice = null
    }
    try {
      tradeFilters = await api.getSymbolFilters(tradeSymbol)
    } catch {
      tradeFilters = null
    }
  }

  $: belowMinimum = !!tradeFilters && tradeFilters.min_notional > 0 && tradeAmount < tradeFilters.min_notional
  // Positions view: hide CANCELED (take-profit removed externally) always, and hide SOLD (closed)
  // unless the user opts in — the default is "still open" only. Toggling resets to the first page.
  let showSoldPositions = false
  $: visiblePositions = operations.filter(
    (operation) => operation.status !== 'CANCELED' && (showSoldPositions || operation.status === 'OPEN'),
  )
  $: hasSoldPositions = operations.some((operation) => operation.status === 'SOLD')
  function toggleShowSold() {
    showSoldPositions = !showSoldPositions
    positionsPage = 1
  }

  // Paginate the (potentially ever-growing) positions and history tables; default 10 rows per page.
  let positionsPage = 1
  let positionsPageSize = 10
  let historyPage = 1
  let historyPageSize = 10
  $: pagedPositions = visiblePositions.slice((positionsPage - 1) * positionsPageSize, positionsPage * positionsPageSize)
  $: pagedExecutions = executions.slice((historyPage - 1) * historyPageSize, historyPage * historyPageSize)

  async function buy() {
    tradeBusy = true
    tradeErr = ''
    try {
      const operation = await api.buy(tradeSymbol, tradeAmount, tradeTarget)
      pushToast(
        $t('buy.bought', {
          qty: fmt(operation.quantity),
          symbol: operation.symbol,
          price: fmt(operation.purchase_price_per_unit)
        }),
        'success'
      )
      operations = await api.getOperations()
      loadExecutions()
    } catch (e) {
      pushToast(translateError($t, e), 'error')
    } finally {
      tradeBusy = false
    }
  }

  async function sellNow(operationId: number) {
    if (!confirm($t('ops.sellConfirm'))) return
    sellBusyId = operationId
    try {
      await api.sellOperation(operationId)
      pushToast($t('ops.sold'), 'success')
      operations = await api.getOperations()
      loadExecutions()
    } catch (e) {
      pushToast(translateError($t, e), 'error')
    } finally {
      sellBusyId = null
    }
  }

  // Retry placing the take-profit sell order for a position whose original sell failed.
  async function placeSell(operationId: number) {
    placeSellBusyId = operationId
    try {
      await api.placeSellOrder(operationId)
      pushToast($t('ops.sellPlaced'), 'success')
      operations = await api.getOperations()
      loadExecutions()
    } catch (e) {
      pushToast(translateError($t, e), 'error')
    } finally {
      placeSellBusyId = null
    }
  }

  let robotMessageTimer: ReturnType<typeof setTimeout> | undefined
  // Success messages auto-dismiss so they don't linger (e.g. after going back to the list).
  function setRobotMessage(message: string) {
    robotMsg = message
    if (robotMessageTimer) clearTimeout(robotMessageTimer)
    if (message) robotMessageTimer = setTimeout(() => (robotMsg = ''), 4000)
  }

  function clearRobotMessages() {
    robotMsg = ''
    robotErr = ''
    if (robotMessageTimer) clearTimeout(robotMessageTimer)
  }

  function backToRobotList() {
    selectedRobotId = null
    robotDraft = null
    creatingRobot = false
    clearRobotMessages()
  }

  async function loadRobots() {
    try {
      const response = await api.getRobots()
      robots = response.robots
      robotLimit = response.limit
      maxOrderQuoteAmount = response.max_order_quote_amount ?? 0
    } catch {
      robots = []
    }
  }

  function selectRobot(robot: Robot) {
    robotDraft = { ...robot }
    robotDailyHourLocal = utcHourToLocal(robot.daily_purchase_hour_utc)
    selectedRobotId = robot.id
    creatingRobot = false
    clearRobotMessages()
  }

  async function createRobot() {
    robotBusy = true
    robotErr = ''
    robotMsg = ''
    try {
      const created = await api.createRobot({
        symbol: newRobotSymbol,
        name: newRobotSymbol,
        capital_threshold: 0,
        max_invested: 0,
        target_profit_percent: 1.5,
        stop_loss_percent: null,
        daily_purchase_hour_utc: localHourToUtc(4),
        daily_purchase_enabled: false,
        sell_order_validity_days: 0,
        is_enabled: true
      })
      await loadRobots()
      selectRobot(robots.find((robot) => robot.id === created.id) || created)
      setRobotMessage($t('robots.created'))
    } catch (e) {
      notifyError(e)
    } finally {
      robotBusy = false
    }
  }

  async function saveRobot() {
    if (!robotDraft) return
    robotBusy = true
    robotErr = ''
    robotMsg = ''
    try {
      robotDraft.daily_purchase_hour_utc = localHourToUtc(robotDailyHourLocal)
      if (!(robotDraft.stop_loss_percent && robotDraft.stop_loss_percent > 0)) robotDraft.stop_loss_percent = null
      const updated = await api.updateRobot(robotDraft)
      await loadRobots()
      robotDraft = { ...updated }
      setRobotMessage($t('robots.saved'))
    } catch (e) {
      notifyError(e)
    } finally {
      robotBusy = false
    }
  }

  async function deleteRobot(robotId: number) {
    if (!confirm($t('robots.deleteConfirm'))) return
    robotBusy = true
    robotErr = ''
    robotMsg = ''
    try {
      await api.deleteRobot(robotId)
      selectedRobotId = null
      robotDraft = null
      await loadRobots()
      setRobotMessage($t('robots.deleted'))
    } catch (e) {
      notifyError(e)
    } finally {
      robotBusy = false
    }
  }

  onMount(async () => {
    await loadAll()
    checkPrice()
    loadSymbols()
    loadExecutions()
    loadRobots()
    // Keep the Positions P/L arrows fresh while that sub-tab is open (backend caches prices for 5s).
    pricesTimer = setInterval(() => {
      if (opsView === 'positions') loadPositionPrices()
    }, 30000)
  })

  onDestroy(() => {
    if (pricesTimer) clearInterval(pricesTimer)
  })
</script>

<main class="page stack-lg">
  {#if loadingError}<div class="card error">{loadingError}</div>{/if}

  <details class="card start" open>
    <summary>
      <span class="start-caret">▸</span>
      <span class="start-title">{$t('start.title')}</span>
    </summary>
    <p class="muted mt-3">{$t('start.intro')}</p>
    <ol>
      <li>{$t('start.s1')}</li>
      <li>{$t('start.s2')}</li>
      <li>{$t('start.s3')}</li>
      <li>{$t('start.s4')}</li>
    </ol>
  </details>

  <div class="tabs" role="tablist">
    <button class="tab" role="tab" aria-selected={activeTab === 'trade'} class:active={activeTab === 'trade'} on:click={() => (activeTab = 'trade')}>
      {#if tradeLocked}<span class="tab-lock" title={$t('lock.tradeMsg')}>🔒</span>{/if}{$t('tab.trade')}
    </button>
    <button class="tab" role="tab" aria-selected={activeTab === 'connection'} class:active={activeTab === 'connection'} on:click={() => (activeTab = 'connection')}>{$t('tab.connection')}</button>
    <button class="tab" role="tab" aria-selected={activeTab === 'b3'} class:active={activeTab === 'b3'} on:click={() => (activeTab = 'b3')}>
      {#if !isAdmin}<span class="tab-lock" title={$t('lock.b3Msg')}>🔒</span>{/if}{$t('tab.b3')}
    </button>
  </div>

  {#if activeTab === 'connection'}
    <section class="card conn">
      <div class="card-header">
        <span class="card-title">{$t('binance.title')}</span>
        <span class="card-subtitle">{$t('binance.subtitle')}</span>
      </div>
      <details class="help"><summary>{$t('help.summary')}</summary><p>{$t('binance.help')}</p></details>

      <div class="field">
        <span class="field-label">{$t('binance.activeEnv')}</span>
        <div class="env-switch">
          {#each environments as environment}
            <button
              type="button"
              class="env-btn"
              class:active={credEnv === environment}
              disabled={envBusy === environment}
              on:click={() => selectEnvironment(environment)}
            >
              <span>{environment === 'TESTNET' ? $t('binance.testnet') : $t('binance.production')}</span>
              {#if isActive(environment) && isConfigured(environment)}
                <span class="tag on">✓ {$t('binance.active')}</span>
              {:else if isActive(environment)}
                <span class="tag warn">✓ {$t('binance.active')} · ⚠ {$t('binance.noKey')}</span>
              {:else if !isConfigured(environment)}
                <span class="tag">· {$t('binance.notConfigured')}</span>
              {/if}
            </button>
          {/each}
        </div>
        <span class="muted mt-2">{$t('binance.envHint')}</span>
        <span class="muted mt-2">{$t('binance.envIsolation')}</span>
      </div>
      {#if envErr}<p class="error mt-2">{envErr}</p>{/if}

      {#if isActive(credEnv) && credentials?.has_active_credential}
        <div class="pill mt-4">{$t('binance.activePrefix')}: {credEnv} • {credentials.masked_api_key}</div>
      {:else if isActive(credEnv)}
        <p class="warn-box mt-4">⚠ {$t('binance.activeNoKey')}</p>
      {:else}
        <p class="muted mt-4">{$t('binance.connectHint')}</p>
      {/if}

      <div class="field">
        <label for="cred-key">{$t('binance.apiKey')} — {credEnv === 'TESTNET' ? $t('binance.testnet') : $t('binance.production')}</label>
        <input id="cred-key" bind:value={credKey} placeholder={$t('binance.apiKey')} />
      </div>
      <div class="field">
        <label for="cred-secret">{$t('binance.apiSecret')}</label>
        <input id="cred-secret" type="password" bind:value={credSecret} placeholder={$t('binance.apiSecret')} />
      </div>
      <button class="btn-block mt-5" disabled={credBusy} on:click={saveCredentials}>
        {credBusy ? $t('binance.saving') : $t('binance.save')}
      </button>
      {#if credMsg}<p class="success mt-3">{credMsg}</p>{/if}
      {#if credErr}<p class="error mt-3">{credErr}</p>{/if}

      {#if isConfigured(credEnv)}
        <div class="danger-zone mt-5">
          {#if !confirmingDelete}
            <button class="ghost danger btn-sm" disabled={credDeleteBusy} on:click={() => (confirmingDelete = true)}>
              {$t('binance.deleteKeys')}
            </button>
            <span class="muted mt-2">{$t('binance.deleteKeysHint')}</span>
          {:else}
            <p class="mt-2">{$t('binance.deleteConfirm')}</p>
            <span class="muted">{$t('binance.deleteKeysHint')}</span>
            <div class="danger-actions mt-3">
              <button class="danger btn-sm" disabled={credDeleteBusy} on:click={deleteCredentials}>
                {credDeleteBusy ? $t('binance.deleting') : $t('binance.confirmDelete')}
              </button>
              <button class="ghost btn-sm" disabled={credDeleteBusy} on:click={() => (confirmingDelete = false)}>
                {$t('binance.cancelDelete')}
              </button>
            </div>
          {/if}
        </div>
      {/if}
    </section>
  {:else if activeTab === 'trade'}
    <div class="locked-wrap">
      {#if tradeLocked}
        <LockOverlay message={$t('lock.tradeMsg')} ctaLabel={$t('lock.goConnect')} onCta={() => (activeTab = 'connection')} />
      {/if}
      <div class="locked-content" class:dimmed={tradeLocked}>
    <div class="grid">
      <section class="card">
        <div class="card-header">
          <span class="card-title">{$t('buy.title')}</span>
          <span class="card-subtitle">{$t('buy.subtitle')}</span>
        </div>
        <details class="help"><summary>{$t('help.summary')}</summary><p>{$t('buy.help')}</p></details>
        <div class="field">
          <label for="trade-symbol">{$t('buy.pair')}</label>
          <SymbolAutocomplete id="trade-symbol" bind:value={tradeSymbol} options={symbols} placeholder="BTCUSDT" on:select={checkPrice} on:commit={checkPrice} />
        </div>
        {#if tradePrice !== null}<div class="muted mt-2">{$t('buy.currentPrice', { price: fmt(tradePrice) })}</div>{/if}
        <div class="field">
          <label for="trade-amount">{$t('buy.amount')}</label>
          <input id="trade-amount" type="number" bind:value={tradeAmount} min="0" step="0.01" />
          {#if tradeFilters && tradeFilters.min_notional > 0}
            <span class="muted">{$t('buy.minOrder', { min: fmt(tradeFilters.min_notional) })}</span>
          {/if}
          {#if quoteAssetOf(tradeSymbol)}
            <span class="muted">{$t('buy.spotHint', { quote: quoteAssetOf(tradeSymbol) })}</span>
          {/if}
          {#if belowMinimum && tradeFilters}<span class="error">{$t('buy.belowMin', { min: fmt(tradeFilters.min_notional) })}</span>{/if}
        </div>
        <div class="field">
          <label for="trade-target">{$t('buy.target')}</label>
          <input id="trade-target" type="number" bind:value={tradeTarget} min="0" step="0.01" />
        </div>
        <button class="btn-block mt-5" disabled={tradeBusy || belowMinimum || !(tradeAmount > 0)} on:click={buy}>
          {tradeBusy ? $t('buy.placing') : $t('buy.button')}
        </button>
        {#if tradeErr}<p class="error mt-3">{tradeErr}</p>{/if}
      </section>

      <section class="card">
        <div class="card-header ops-header">
          <div class="stack-title">
            <span class="card-title">{$t('robots.title')}</span>
            <span class="card-subtitle">{$t('robots.subtitle')}</span>
          </div>
        </div>
        <details class="help"><summary>{$t('help.summary')}</summary><p>{$t('robots.help')}</p></details>
        <p class="muted">{isAdmin ? $t('robots.planAdmin') : $t('robots.planStandard', { n: robotLimit })}</p>
        {#if selectedRobot && robotDraft}
          <button class="btn-sm ghost robot-nav-btn" on:click={backToRobotList}>{$t('robots.back')}</button>
        {:else if !creatingRobot}
          <button class="btn-sm robot-nav-btn" disabled={!canCreateRobot} on:click={() => { creatingRobot = true; robotErr = ''; robotMsg = '' }}>{$t('robots.new')}</button>
        {/if}
        {#if robotMsg}<p class="success mt-2">✓ {robotMsg}</p>{/if}
        {#if robotErr}<p class="error mt-2">{robotErr}</p>{/if}

        {#if selectedRobot && robotDraft}
          <div class="bot-status" class:on={robotDraft.is_enabled && robotDraft.daily_purchase_enabled && robotDraft.capital_threshold > 0}>
            <div class="bot-head">
              <span class="badge {robotDraft.is_enabled ? 'green' : 'amber'}">{robotDraft.is_enabled ? $t('robots.on') : $t('robots.off')}</span>
              <strong>{robotDraft.symbol}</strong>
              <span class="spacer"></span>
              <label class="switch-inline"><input type="checkbox" bind:checked={robotDraft.is_enabled} /> {$t('robots.master')}</label>
            </div>
            {#if robotDraft.is_enabled && robotDraft.daily_purchase_enabled && robotDraft.capital_threshold > 0}
              <p class="muted">{$t('bot.summary', { time: formatHour(robotDailyHourLocal), capital: fmt(robotDraft.capital_threshold), symbol: robotDraft.symbol, target: robotDraft.target_profit_percent })}</p>
              {#if !connected}<p class="warn">{$t('bot.needsConnection')}</p>{/if}
              {#if robotProductionNeedsLive}<p class="warn">{$t('bot.needsLive')}</p>{/if}
            {:else}
              <p class="muted">{$t('robots.idleHint')}</p>
            {/if}
          </div>

          <div class="field">
            <label for="robot-name">{$t('robots.name')}</label>
            <input id="robot-name" bind:value={robotDraft.name} placeholder={$t('robots.namePlaceholder')} />
          </div>
          <div class="grid-2 mt-4">
            <div class="field" style="margin-top:0">
              <label for="robot-coin">{$t('robots.coin')}</label>
              <input id="robot-coin" value={robotDraft.symbol} disabled />
              {#if quoteAssetOf(robotDraft.symbol)}<span class="muted">{$t('buy.spotHint', { quote: quoteAssetOf(robotDraft.symbol) })}</span>{/if}
            </div>
            <label class="checkbox-row" style="margin-top:0">
              <input type="checkbox" bind:checked={robotDraft.daily_purchase_enabled} />
              {$t('robots.dailyEnabled')}
            </label>
          </div>
          <div class="grid-2 mt-4">
            <div class="field" style="margin-top:0">
              <label for="robot-capital">{$t('settings.capital')}</label>
              <input id="robot-capital" type="number" bind:value={robotDraft.capital_threshold} min="0" step="0.01" />
            </div>
            <div class="field" style="margin-top:0">
              <label for="robot-max-invested">{$t('settings.maxInvested')}</label>
              <input id="robot-max-invested" type="number" bind:value={robotDraft.max_invested} min="0" step="0.01" placeholder={$t('settings.maxInvestedNone')} />
            </div>
          </div>
          <p class="muted">{$t('settings.maxInvestedHelp')}{#if maxOrderQuoteAmount > 0} {$t('settings.maxOrderHelp', { max: fmt(maxOrderQuoteAmount) })}{/if}</p>
          <div class="grid-2 mt-4">
            <div class="field" style="margin-top:0">
              <label for="robot-target">{$t('settings.target')}</label>
              <input id="robot-target" type="number" bind:value={robotDraft.target_profit_percent} min="0" step="0.01" />
            </div>
            <div class="field" style="margin-top:0">
              <label for="robot-stop">{$t('settings.stopLoss')}</label>
              <input id="robot-stop" type="number" bind:value={robotDraft.stop_loss_percent} min="0" step="0.01" placeholder={$t('settings.stopLossNone')} />
            </div>
          </div>
          <div class="grid-2 mt-4">
            <div class="field" style="margin-top:0">
              <label for="robot-daily-time">{$t('settings.dailyTime')}</label>
              <select id="robot-daily-time" bind:value={robotDailyHourLocal}>
                {#each hours as hour}<option value={hour}>{formatHour(hour)}</option>{/each}
              </select>
            </div>
            <div class="field" style="margin-top:0">
              <label for="robot-validity">{$t('settings.validity')}</label>
              <input id="robot-validity" type="number" bind:value={robotDraft.sell_order_validity_days} min="0" max="365" step="1" />
            </div>
          </div>
          <p class="muted tz-note">{$t('settings.timezoneNote', { tz: localTimeZone, offset: tzOffset })}</p>
          <p class="muted">{$t('settings.validityHelp')}</p>
          <div class="robot-editor-actions mt-5">
            <button class="danger btn-sm" disabled={robotBusy} on:click={() => deleteRobot(robotDraft.id)}>{$t('robots.delete')}</button>
            <button class="btn-sm" disabled={robotBusy} on:click={saveRobot}>{robotBusy ? $t('settings.saving') : $t('settings.save')}</button>
          </div>
        {:else if creatingRobot}
          <div class="field">
            <label for="new-robot-symbol">{$t('robots.coin')}</label>
            <SymbolAutocomplete id="new-robot-symbol" bind:value={newRobotSymbol} options={symbols} placeholder="BTCUSDT" />
          </div>
          <div class="robot-editor-actions mt-4">
            <button class="btn-sm ghost" disabled={robotBusy} on:click={() => (creatingRobot = false)}>{$t('common.cancel')}</button>
            <button class="btn-sm" disabled={robotBusy || !newRobotSymbol} on:click={createRobot}>{robotBusy ? $t('robots.creating') : $t('robots.create')}</button>
          </div>
        {:else}
          {#if robots.length === 0}
            <p class="muted mt-3">{$t('robots.none')}</p>
          {:else}
            <div class="robot-list mt-3">
              {#each robots as robot (robot.id)}
                <button class="robot-row" on:click={() => selectRobot(robot)}>
                  <span class="badge {robot.is_enabled ? 'green' : 'amber'}">{robot.is_enabled ? $t('robots.on') : $t('robots.off')}</span>
                  <strong class="robot-name">{robot.name}</strong>
                  <span class="muted robot-sym">{robot.symbol}</span>
                  <span class="spacer"></span>
                  {#if robot.daily_purchase_enabled && robot.capital_threshold > 0}
                    <span class="muted robot-dca">DCA {fmt(robot.capital_threshold)} · {formatHour(utcHourToLocal(robot.daily_purchase_hour_utc))}</span>
                  {/if}
                  <span class="robot-open">{$t('robots.open')} →</span>
                </button>
              {/each}
            </div>
          {/if}
          {#if !canCreateRobot}<p class="warn mt-3">{$t('robots.limitReached')}</p>{/if}
        {/if}

        {#if settings && credentials?.active_environment === 'PRODUCTION'}
          <div class="live-panel" class:armed={settings.live_trading_enabled}>
            <div class="live-panel-head">
              <div class="stack-title">
                <span class="live-panel-title">{$t('settings.liveTitle')}</span>
                <span class="badge {settings.live_trading_enabled ? 'green' : 'amber'}">{settings.live_trading_enabled ? $t('settings.liveOn') : $t('settings.liveOff')}</span>
              </div>
              <label class="switch" title={$t('settings.enableLive')}>
                <input type="checkbox" bind:checked={settings.live_trading_enabled} on:change={saveSettings} disabled={settingsBusy} />
                <span class="slider"></span>
              </label>
            </div>
            <p class="muted live-panel-help">{$t('settings.liveHelp')}</p>
          </div>
          {#if settingsErr}<p class="error mt-2">{settingsErr}</p>{/if}
        {/if}
      </section>
    </div>

    <details class="card alloc-card" open>
      <summary class="alloc-summary">
        <span class="start-caret">▸</span>
        <span class="card-title">{$t('alloc.section')}</span>
      </summary>

      <div class="subtabs mt-4">
        <button class="subtab" class:active={allocView === 'allocation'} on:click={() => (allocView = 'allocation')}>{$t('alloc.tabAllocation')}</button>
        <button class="subtab" class:active={allocView === 'profit'} on:click={() => (allocView = 'profit')}>{$t('alloc.tabProfit')}</button>
      </div>

      {#if allocView === 'allocation'}
        <details class="help"><summary>{$t('help.summary')}</summary><p>{$t('alloc.help')}</p></details>
        <AllocationPanel {operations} />
      {:else}
        <details class="help"><summary>{$t('help.summary')}</summary><p>{$t('prof.help')}</p></details>
        {#if !hasProfitData}
          <p class="muted mt-3">{$t('prof.none')}</p>
        {:else}
          <div class="prof-grid mt-3">
            <div class="prof-card">
              <span class="prof-label">{$t('prof.spent')}</span>
              <span class="prof-value">{fmt(investedTotal)}</span>
              <span class="prof-split">{$t('prof.you')}: {fmt(spentBySite)} · {$t('prof.robots')}: {fmt(spentByRobots)}</span>
            </div>
            <div class="prof-card">
              <span class="prof-label">{$t('prof.received')}</span>
              <span class="prof-value">{fmt(realizedProceeds)}</span>
              <span class="prof-split">{$t('prof.you')}: {fmt(earnedBySite)} · {$t('prof.robots')}: {fmt(earnedByRobots)}</span>
            </div>
            <div class="prof-card">
              <span class="prof-label">{$t('prof.realized')}</span>
              <span class="prof-value {realizedResult > 0 ? 'pos' : realizedResult < 0 ? 'neg' : ''}">{realizedResult >= 0 ? '+' : ''}{fmt(realizedResult)}</span>
              <span class="prof-split">{$t('prof.openCost')}: {fmt(openCostTotal)}</span>
            </div>
          </div>
          <div class="prof-coins mt-4">
            <span class="prof-label">{$t('prof.acquired')}</span>
            <div class="coin-chips">
              {#each acquiredBySymbol as item (item.symbol)}
                <span class="coin-chip">{fmt(item.quantity)} {item.symbol}</span>
              {/each}
            </div>
          </div>
          <div class="prof-chart-block mt-5">
            <ProfitabilityPanel {operations} />
          </div>
        {/if}
      {/if}
    </details>

    <section class="card">
      <div class="card-header">
        <span class="card-title">{$t('ops.title')}</span>
      </div>
      <div class="subtabs mt-4">
        <button class="subtab" class:active={opsView === 'positions'} on:click={() => (opsView = 'positions')}>{$t('ops.tabPositions')}</button>
        <button class="subtab" class:active={opsView === 'history'} on:click={() => (opsView = 'history')}>{$t('ops.tabHistory')}</button>
      </div>

      {#if opsView === 'positions'}
        <details class="help">
          <summary>{$t('ops.statusHelp')}</summary>
          <p>{$t('ops.openMeaning')}</p>
          <p>{$t('ops.soldMeaning')}</p>
          <p>{$t('ops.sellOrderMeaning')}</p>
        </details>
        {#if hasSoldPositions}
          <label class="show-sold mt-3">
            <input type="checkbox" checked={showSoldPositions} on:change={toggleShowSold} />
            {$t('ops.showSold')}
          </label>
        {/if}
        {#if visiblePositions.length === 0}
          <p class="muted mt-3">{$t('ops.none')}</p>
        {:else}
          <div class="table positions-table mt-3">
            <div class="trow thead">
              <div>{$t('ops.pair')}</div>
              <div>{$t('ops.status')}</div>
              <div>{$t('ops.qty')}</div>
              <div>{$t('ops.buyPrice')}</div>
              <div>{$t('ops.target')}</div>
              <div>{$t('ops.targetPct')}</div>
              <div>{$t('ops.sellOrder')}</div>
              <div>{$t('ops.purchased')}</div>
              <div class="col-actions">{$t('ops.actions')}</div>
            </div>
            {#each pagedPositions as operation (operation.id)}
              <div class="trow">
                <div class="pair-cell" data-label={$t('ops.pair')}>{operation.symbol}</div>
                <div data-label={$t('ops.status')}><span class="badge {operation.status === 'SOLD' ? 'green' : 'amber'}">{operation.status}</span></div>
                <div data-label={$t('ops.qty')}>{fmt(operation.quantity)}</div>
                <div data-label={$t('ops.buyPrice')}>{fmt(operation.purchase_price_per_unit)}</div>
                <div data-label={$t('ops.target')}>{fmt(operation.sell_target_price_per_unit)}</div>
                <div class="gold" data-label={$t('ops.targetPct')}>+{fmt(operation.target_profit_percent)}%</div>
                <div class="sell-cell" data-label={$t('ops.sellOrder')}>
                  {#if operation.sell_order_id}
                    <span class="badge green" title={$t('ops.gtcHelp')}>✓</span>
                    {#if operation.sell_order_expires_at}
                      <span class="muted gtc">{$t('ops.expiresAt', { date: $formatDate(operation.sell_order_expires_at, { day: '2-digit', month: '2-digit' }) })}</span>
                    {:else}
                      <span class="muted gtc" title={$t('ops.gtcHelp')}>{$t('ops.gtc')}</span>
                    {/if}
                  {:else if operation.status === 'OPEN'}
                    <span class="badge red" title={$t('ops.noSellOrder')}>⚠</span>
                  {:else}
                    <span class="muted">—</span>
                  {/if}
                </div>
                <div class="muted" data-label={$t('ops.purchased')}>{$formatDateTime(operation.purchased_at)}</div>
                <div class="col-actions ops-actions" data-label={$t('ops.actions')}>
                  {#if operation.status === 'OPEN'}
                    {#if !operation.sell_order_id}
                      <button class="btn-sm" disabled={placeSellBusyId === operation.id} on:click={() => placeSell(operation.id)}>
                        {placeSellBusyId === operation.id ? $t('ops.retrying') : $t('ops.retrySell')}
                      </button>
                    {/if}
                    <div class="sell-row">
                      {#if currentPrices[operation.symbol]}
                        <span class="pnl-arrow {pnlPct(operation) >= 0 ? 'pos' : 'neg'}" title={pnlTooltip(operation)} aria-label={pnlTooltip(operation)}>
                          <span class="pnl-caret">{pnlPct(operation) >= 0 ? '▲' : '▼'}</span>{pnlPct(operation) >= 0 ? '+' : ''}{pnlPct(operation).toFixed(2)}%
                        </span>
                      {/if}
                      <button class="danger btn-sm" disabled={sellBusyId === operation.id} on:click={() => sellNow(operation.id)}>
                        {sellBusyId === operation.id ? $t('ops.selling') : $t('ops.sellNow')}
                      </button>
                    </div>
                  {:else}
                    <span class="muted">—</span>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
          <Pagination total={visiblePositions.length} bind:page={positionsPage} bind:pageSize={positionsPageSize} />
        {/if}
      {:else}
        {#if executions.length === 0}
          <p class="muted mt-3">{$t('hist.none')}</p>
        {:else}
          <div class="table htable mt-3">
            <div class="hrow thead">
              <div>{$t('hist.when')}</div>
              <div>{$t('hist.action')}</div>
              <div>{$t('hist.by')}</div>
              <div>{$t('ops.pair')}</div>
              <div>{$t('hist.price')}</div>
              <div>{$t('hist.qty')}</div>
              <div>{$t('hist.total')}</div>
              <div class="col-actions">{$t('hist.result')}</div>
            </div>
            {#each pagedExecutions as execution (execution.id)}
              <div class="hrow">
                <div class="muted" data-label={$t('hist.when')}>{$formatDateTime(execution.executed_at)}</div>
                <div data-label={$t('hist.action')}><span class="badge {execution.operation_type === 'SELL' ? 'green' : execution.operation_type === 'SELL_ORDER_PLACED' ? 'blue' : execution.operation_type.startsWith('SELL_') ? 'red' : 'amber'}">{$t('hist.act.' + execution.operation_type)}</span></div>
                <div data-label={$t('hist.by')}><span class="by-badge {execution.initiated_by === 'BOT' ? 'bot' : 'user'}">{execution.initiated_by === 'BOT' ? $t('hist.bot') : $t('hist.you')}</span></div>
                <div data-label={$t('ops.pair')}>{execution.symbol}</div>
                <div data-label={$t('hist.price')}>{fmt(execution.unit_price)}</div>
                <div data-label={$t('hist.qty')}>{fmt(execution.quantity)}</div>
                <div data-label={$t('hist.total')}>{fmt(execution.total_value)}</div>
                <div class="col-actions" data-label={$t('hist.result')}>
                  {#if execution.success}
                    <span class="badge green">✓</span>
                  {:else}
                    <span class="badge red" title={execution.error_message || ''}>✗</span>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
          <Pagination total={executions.length} bind:page={historyPage} bind:pageSize={historyPageSize} />
        {/if}
      {/if}
    </section>
      </div>
    </div>
  {:else if activeTab === 'b3'}
    <div class="locked-wrap">
      {#if !isAdmin}
        <LockOverlay message={$t('lock.b3Msg')} />
      {/if}
      <div class="locked-content" class:dimmed={!isAdmin}>
        <PortfolioPanel />
      </div>
    </div>
  {/if}

  <LegalFooter />
</main>

<style>
  .danger-zone { display: flex; flex-direction: column; gap: var(--space-1); padding-top: var(--space-4); border-top: 1px solid var(--border); }
  .danger-actions { display: flex; gap: var(--space-2); flex-wrap: wrap; }
  .start summary { cursor: pointer; list-style: none; display: flex; align-items: center; gap: var(--space-2); }
  .start summary::-webkit-details-marker { display: none; }
  .start-caret { color: var(--brand); display: inline-block; transition: transform 0.15s ease; }
  .start[open] .start-caret { transform: rotate(90deg); }
  .start-title { font-size: var(--text-md); font-weight: 800; }
  .start ol { margin: var(--space-3) 0 0; padding-left: var(--space-5); line-height: 1.8; color: var(--text); }

  .tabs { display: flex; gap: var(--space-2); border-bottom: 1px solid var(--border); flex-wrap: wrap; }
  .tab { background: transparent; border: none; border-bottom: 2px solid transparent; border-radius: 0; color: var(--muted); font-weight: 700; height: auto; padding: var(--space-3) var(--space-4); }
  .tab:hover:not(:disabled) { filter: none; color: var(--text); }
  .tab.active { color: var(--brand); border-bottom-color: var(--brand); }
  .tab-lock { margin-right: var(--space-1); font-size: 0.8em; }
  .locked-wrap { position: relative; }
  /* Restore the page's vertical rhythm: the wrapped sections need the same column gap stack-lg gives. */
  .locked-content { display: flex; flex-direction: column; gap: var(--space-5); }
  .locked-content.dimmed { pointer-events: none; opacity: 0.5; filter: grayscale(0.4); user-select: none; }

  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: var(--space-4); align-items: start; }
  .conn { max-width: 560px; margin-inline: auto; }
  .checkbox-row { display: flex; align-items: center; gap: var(--space-2); margin: var(--space-4) 0 0; font-weight: 600; }
  .tz-note { margin-top: var(--space-2); }

  .env-switch { display: flex; gap: var(--space-2); flex-wrap: wrap; }
  .env-btn { background: var(--surface-2); border: 1px solid var(--border-strong); color: var(--text); font-weight: 700; }
  .env-btn.active { background: var(--brand); border-color: var(--brand); color: var(--on-brand); }
  .env-btn .tag { font-weight: 600; opacity: 0.75; }
  .env-btn .tag.on { opacity: 1; }
  .env-btn .tag.warn { opacity: 1; }
  .warn-box { border: 1px solid var(--amber); border-left: 3px solid var(--amber); border-radius: var(--radius-md); background: var(--surface-2); padding: var(--space-3) var(--space-4); color: var(--amber); line-height: 1.5; }

  .bot-status { border: 1px solid var(--border); border-left: 3px solid var(--amber); border-radius: var(--radius-md); background: var(--surface-2); padding: var(--space-3) var(--space-4); margin-bottom: var(--space-4); }
  .bot-status.on { border-left-color: var(--green); }
  .bot-head { display: flex; align-items: center; gap: var(--space-2); margin-bottom: var(--space-2); }
  .bot-status p { margin-top: var(--space-1); line-height: 1.5; }
  .bot-status .warn { color: var(--amber); font-size: var(--text-sm); }

  .stack-title { display: flex; flex-direction: column; }
  .switch-inline { display: inline-flex; align-items: center; gap: var(--space-1); font-size: var(--text-xs); font-weight: 600; white-space: nowrap; }
  .robot-nav-btn { display: inline-flex; margin-top: var(--space-3); margin-bottom: var(--space-4); }
  .robot-editor-actions { display: flex; justify-content: space-between; gap: var(--space-2); }
  .robot-list { display: flex; flex-direction: column; gap: var(--space-2); }
  .robot-row { display: flex; align-items: center; gap: var(--space-2); width: 100%; text-align: left; background: var(--surface-2); border: 1px solid var(--border); border-radius: var(--radius-md); padding: var(--space-2) var(--space-3); color: var(--text); font: inherit; height: auto; }
  .robot-row:hover:not(:disabled) { border-color: var(--brand); filter: none; }
  .robot-name { font-weight: 700; }
  .robot-sym, .robot-dca { font-size: var(--text-xs); }
  .robot-open { color: var(--brand); font-weight: 700; font-size: var(--text-xs); white-space: nowrap; }
  .live-panel { border-top: 1px solid var(--border); padding-top: var(--space-4); margin-top: var(--space-4); }
  .live-panel-head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }
  .live-panel-title { font-weight: 700; }
  .live-panel-help { margin-top: var(--space-2); font-size: var(--text-xs); }
  .switch { position: relative; display: inline-block; width: 46px; height: 26px; flex: none; }
  .switch input { position: absolute; opacity: 0; width: 0; height: 0; }
  .switch .slider { position: absolute; inset: 0; cursor: pointer; background: var(--border-strong); border-radius: 999px; transition: background .2s; }
  .switch .slider::before { content: ''; position: absolute; height: 20px; width: 20px; left: 3px; top: 3px; background: #fff; border-radius: 50%; transition: transform .2s; }
  .switch input:checked + .slider { background: var(--brand-strong); }
  .switch input:checked + .slider::before { transform: translateX(20px); }
  .switch input:disabled + .slider { opacity: .6; cursor: default; }
  .switch input:focus-visible + .slider { outline: 2px solid var(--brand); outline-offset: 2px; }

  .alloc-card > summary { cursor: pointer; list-style: none; display: flex; align-items: center; gap: var(--space-2); }
  .alloc-card > summary::-webkit-details-marker { display: none; }
  .alloc-card[open] > summary .start-caret { transform: rotate(90deg); }
  .prof-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: var(--space-3); }
  .prof-card { display: flex; flex-direction: column; gap: 2px; background: var(--surface-2); border: 1px solid var(--border); border-radius: var(--radius-md); padding: var(--space-3); }
  .prof-label { color: var(--muted); font-size: var(--text-xs); font-weight: 700; text-transform: uppercase; letter-spacing: 0.02em; }
  .prof-value { font-size: var(--text-lg); font-weight: 800; }
  .prof-value.pos { color: var(--green); }
  .prof-value.neg { color: var(--red); }
  .prof-split { color: var(--muted); font-size: var(--text-xs); }
  .prof-coins { display: flex; flex-direction: column; gap: var(--space-2); }
  .coin-chips { display: flex; flex-wrap: wrap; gap: var(--space-2); }
  .coin-chip { background: var(--surface-2); border: 1px solid var(--border); border-radius: var(--radius-pill); padding: 2px var(--space-3); font-size: var(--text-sm); font-weight: 700; }

  .ops-header { flex-direction: row; align-items: center; justify-content: space-between; gap: var(--space-3); flex-wrap: wrap; }
  .subtabs { display: flex; gap: var(--space-1); }
  .subtab { background: var(--surface-2); border: 1px solid var(--border); color: var(--muted); height: 2rem; padding: 0 var(--space-3); font-size: var(--text-xs); font-weight: 700; border-radius: var(--radius-sm); }
  .subtab.active { background: var(--brand); color: var(--on-brand); border-color: var(--brand); }
  .show-sold { display: inline-flex; align-items: center; gap: var(--space-2); font-size: var(--text-sm); color: var(--muted); cursor: pointer; }
  .show-sold input { width: 1rem; height: 1rem; accent-color: var(--brand); cursor: pointer; }

  .table { display: flex; flex-direction: column; overflow-x: auto; }
  .trow { display: grid; grid-template-columns: 1fr 1fr 1fr 1.2fr 1.2fr 0.9fr 1.3fr 1.4fr 200px; gap: var(--space-2); padding: var(--space-3) var(--space-1); border-bottom: 1px solid var(--border); align-items: center; font-size: var(--text-sm); min-width: 1020px; }
  .trow .gold { color: var(--brand); font-weight: 700; }
  .hrow { display: grid; grid-template-columns: 150px 0.9fr 0.8fr 0.9fr 1fr 0.9fr 1fr 70px; gap: var(--space-2); padding: var(--space-3) var(--space-1); border-bottom: 1px solid var(--border); align-items: center; font-size: var(--text-sm); min-width: 760px; }
  .thead { color: var(--muted); font-weight: 700; font-size: var(--text-xs); }
  .col-actions { text-align: right; }
  .ops-actions { display: flex; flex-direction: column; gap: var(--space-1); align-items: flex-end; }
  .sell-row { display: flex; align-items: center; gap: var(--space-2); }
  .pnl-arrow { display: inline-flex; align-items: center; gap: 2px; font-size: var(--text-xs); font-weight: 700; white-space: nowrap; cursor: help; }
  .pnl-arrow.pos { color: var(--green); }
  .pnl-arrow.neg { color: var(--red); }
  .pnl-caret { font-size: 0.7em; }
  .sell-cell { display: flex; align-items: center; gap: var(--space-2); }
  .sell-cell .gtc { font-size: var(--text-xs); }
  .by-badge { padding: 2px var(--space-2); border-radius: var(--radius-pill); font-weight: 700; font-size: var(--text-xs); white-space: nowrap; }
  .by-badge.bot { background: rgba(151, 117, 250, 0.18); color: #b197fc; }
  .by-badge.user { background: rgba(77, 171, 247, 0.18); color: #74c0fc; }

  /* Positions table reflows to stacked label/value cards on phones — a 9-column grid is unusable in a
     ~390px side-scroll (it hides the P/L and the Sell action off-screen). Desktop is unchanged. */
  @media (max-width: 600px) {
    .positions-table .thead { display: none; }
    .positions-table .trow {
      grid-template-columns: 1fr; min-width: 0; gap: var(--space-1);
      padding: var(--space-3); border: 1px solid var(--border); border-radius: var(--radius-md);
      margin-bottom: var(--space-2);
    }
    .positions-table .trow > div:not(.ops-actions) {
      display: flex; justify-content: space-between; align-items: center; gap: var(--space-3);
    }
    .positions-table .trow > div:not(.ops-actions)::before {
      content: attr(data-label); color: var(--muted); font-size: var(--text-xs); font-weight: 700; flex: none;
    }
    .positions-table .pair-cell { font-weight: 800; font-size: var(--text-md); }
    .positions-table .ops-actions { align-items: stretch; text-align: left; margin-top: var(--space-1); }
    .positions-table .ops-actions::before {
      content: attr(data-label); color: var(--muted); font-size: var(--text-xs); font-weight: 700;
    }
    .positions-table .sell-row { justify-content: flex-end; }
    /* History reflows the same way (read-only, no actions column). */
    .htable .thead { display: none; }
    .htable .hrow {
      grid-template-columns: 1fr; min-width: 0; gap: var(--space-1);
      padding: var(--space-3); border: 1px solid var(--border); border-radius: var(--radius-md);
      margin-bottom: var(--space-2);
    }
    .htable .hrow > div { display: flex; justify-content: space-between; align-items: center; gap: var(--space-3); }
    .htable .hrow > div::before { content: attr(data-label); color: var(--muted); font-size: var(--text-xs); font-weight: 700; flex: none; }
    .htable .col-actions { text-align: left; }
  }
</style>
