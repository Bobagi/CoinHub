<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { api, type TradingSettings, type CredentialStatus, type Operation, type Execution, type Robot, type SymbolInfo, type SpotBalance } from './api'
  import { binanceStatus, currentUser, pushToast, notifyError, displayCurrency, setDisplayCurrency } from './stores'
  import { t, intlLocale, formatDateTime, formatDate, translateError } from './i18n'
  import { splitSymbol, formatMoney, formatConvertedTotal, convertedTotalValue, subtractAmounts, quoteAssetFor, hasUnconvertedParts, type AmountsByCurrency } from './money'
  import AllocationPanel from './AllocationPanel.svelte'
  import ProfitabilityPanel from './ProfitabilityPanel.svelte'
  import PortfolioPanel from './PortfolioPanel.svelte'
  import LegalFooter from './LegalFooter.svelte'
  import SymbolAutocomplete from './SymbolAutocomplete.svelte'
  import LockOverlay from './LockOverlay.svelte'
  import BalanceCallout from './BalanceCallout.svelte'
  import Pagination from './Pagination.svelte'
  import Collapsible from './Collapsible.svelte'

  let activeTab: 'connection' | 'trade' | 'b3' = 'trade'
  let opsView: 'positions' | 'history' = 'positions'
  let allocView: 'allocation' | 'profit' = 'allocation'
  const environments = ['TESTNET', 'PRODUCTION']

  // symbol → base/quote maps built from the exchange info (/binance/symbols); splitSymbol's suffix
  // guess is only the fallback for pairs the active environment no longer lists.
  let quoteBySymbol: Record<string, string> = {}
  let baseBySymbol: Record<string, string> = {}
  // Reactive arrows (not plain functions) so template expressions using them re-evaluate when the
  // async-loaded maps or the locale change.
  $: quoteOf = (symbol: string) => quoteAssetFor(symbol, quoteBySymbol)
  $: baseOf = (symbol: string) => baseBySymbol[(symbol || '').toUpperCase()] || splitSymbol(symbol).base
  // money(v, symbol) formats v in the PAIR'S quote currency — for per-row amounts, which stay in the
  // currency they actually happened in (only aggregates get converted to the display currency).
  $: money = (value: number, symbol: string) => formatMoney(value, quoteOf(symbol), $intlLocale)

  let settings: TradingSettings | null = null
  let credentials: CredentialStatus | null = null
  let operations: Operation[] = []
  let executions: Execution[] = []
  let symbolInfos: SymbolInfo[] = []
  $: symbols = symbolInfos.map((info) => info.symbol) // just the names, for the autocompletes
  let loadingError = ''

  // Spot-wallet balances (free = spendable) for the active environment, keyed by asset. Drives the
  // "available to buy" hints; connected=false (no keys yet) simply hides them.
  let spotBalances: Record<string, SpotBalance> = {}
  let balancesConnected = false
  $: freeOf = (asset: string) => spotBalances[asset]?.free ?? 0

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

  // Profitability (for the active environment), grouped by QUOTE currency — users can trade BRL-,
  // USDT- and BTC-quoted pairs side by side, and summing those raw numbers would mix units. The
  // per-currency totals are converted to the display currency at render time (formatConvertedTotal).
  // Costs/proceeds come from the operations table (the source of truth); the site-vs-robots split
  // comes from executions (initiated_by). Buys = money in; completed sales (SELL) = money back;
  // SELL_ORDER_PLACED records are just placed orders, not sales.
  function sumByQuote<T>(items: T[], quotes: Record<string, string>, symbolOf: (item: T) => string, valueOf: (item: T) => number): AmountsByCurrency {
    const totals: AmountsByCurrency = {}
    for (const item of items) {
      const quote = quoteAssetFor(symbolOf(item), quotes)
      totals[quote] = (totals[quote] || 0) + valueOf(item)
    }
    return totals
  }
  $: investedByQuote = sumByQuote(operations, quoteBySymbol, (op) => op.symbol, (op) => op.quantity * op.purchase_price_per_unit)
  $: openCostByQuote = sumByQuote(operations.filter((op) => op.status === 'OPEN'), quoteBySymbol, (op) => op.symbol, (op) => op.quantity * op.purchase_price_per_unit)
  $: soldOperations = operations.filter((op) => op.status === 'SOLD' && op.sell_price_per_unit != null)
  $: receivedByQuote = sumByQuote(soldOperations, quoteBySymbol, (op) => op.symbol, (op) => op.quantity * (op.sell_price_per_unit as number))
  $: realizedByQuote = subtractAmounts(receivedByQuote, sumByQuote(soldOperations, quoteBySymbol, (op) => op.symbol, (op) => op.quantity * op.purchase_price_per_unit))
  $: spentBySiteByQuote = sumByQuote(executions.filter((e) => e.success && e.operation_type === 'BUY' && e.initiated_by === 'USER'), quoteBySymbol, (e) => e.symbol, (e) => e.total_value)
  $: spentByRobotsByQuote = sumByQuote(executions.filter((e) => e.success && (e.operation_type === 'DAILY_BUY' || e.operation_type === 'BUY') && e.initiated_by === 'BOT'), quoteBySymbol, (e) => e.symbol, (e) => e.total_value)
  $: earnedBySiteByQuote = sumByQuote(executions.filter((e) => e.success && e.operation_type === 'SELL' && e.initiated_by === 'USER'), quoteBySymbol, (e) => e.symbol, (e) => e.total_value)
  $: earnedByRobotsByQuote = sumByQuote(executions.filter((e) => e.success && e.operation_type === 'SELL' && e.initiated_by === 'BOT'), quoteBySymbol, (e) => e.symbol, (e) => e.total_value)
  $: realizedTotal = convertedTotalValue(realizedByQuote, displayCode, rates)
  // Profit/loss coloring only applies over a fully-converted sum — with an unconvertible leftover the
  // converted subtotal's sign could contradict the full picture, so the card stays neutral.
  $: realizedMixed = hasUnconvertedParts(realizedByQuote, displayCode, rates)
  $: acquiredBySymbol = aggregateQuantity(operations)

  // --- Display currency + conversion rates --------------------------------------------------------
  // Every quote currency the user's data touches (positions, history, robots).
  $: allQuoteCurrencies = [
    ...new Set(
      [...operations.map((op) => op.symbol), ...executions.map((e) => e.symbol), ...robots.map((robot) => robot.symbol)].map(
        (symbol) => quoteAssetFor(symbol, quoteBySymbol)
      )
    )
  ].filter(Boolean)
  // Auto display currency = the quote currency with the most invested; new accounts default by locale.
  // The picker persists an explicit choice in localStorage (displayCurrency store).
  $: dominantQuote = Object.entries(investedByQuote).filter(([code]) => code).sort((a, b) => b[1] - a[1])[0]?.[0] || ''
  $: displayCode = ($displayCurrency || dominantQuote || ($intlLocale.startsWith('pt') ? 'BRL' : 'USDT')).toUpperCase()
  $: displayOptions = [...new Set([displayCode, ...allQuoteCurrencies, 'BRL', 'USDT', 'USDC', 'EUR', 'BTC'])].filter(Boolean)

  // Market rates quote-currency → display-currency (1 when equal). A missing rate (e.g. testnet's tiny
  // symbol list) degrades gracefully: formatConvertedTotal shows those parts unconverted.
  let rates: Record<string, number> = {}
  let lastRatesKey = ''
  $: ratesKey = displayCode + '|' + [...allQuoteCurrencies].sort().join(',')
  $: if (ratesKey !== lastRatesKey) {
    lastRatesKey = ratesKey
    loadRates()
  }
  function onDisplayCurrencyChange(event: Event) {
    const select = event.currentTarget as HTMLSelectElement
    setDisplayCurrency(select.value)
  }

  // Monotonic sequence so an older in-flight fetch (larger/smaller currency set OR older display
  // currency) can never overwrite the result of a newer one.
  let ratesRequestSequence = 0
  async function loadRates() {
    const requestId = ++ratesRequestSequence
    const target = displayCode
    const sources = allQuoteCurrencies.filter((code) => code && code !== target)
    const entries = await Promise.all(
      sources.map(async (code) => {
        try {
          return [code, (await api.getRate(code, target)).rate] as const
        } catch {
          return [code, 0] as const
        }
      })
    )
    if (requestId !== ratesRequestSequence) return
    rates = Object.fromEntries(entries)
  }

  // "Available to buy": the free spot balance of every quote currency the user trades with (0 kept —
  // "you're out of BRL" is exactly the useful signal), shown as ONE value converted into the display
  // currency (the whole toolbar follows the selected currency); the tooltip breaks it down per
  // original currency.
  $: availableParts = allQuoteCurrencies.reduce((parts, asset) => {
    parts[asset] = freeOf(asset)
    return parts
  }, {} as AmountsByCurrency)
  $: availableTooltip = [
    $t('display.availableHelp'),
    ...Object.entries(availableParts).map(([asset, value]) => formatMoney(value, asset, $intlLocale))
  ].join('\n')

  // One builder owns the robot-failure text; the list tooltip and the editor line both consume it.
  // Falls back to the raw operation type when a history label is missing (never the raw i18n key).
  $: actionLabel = (operationType: string) => {
    const key = 'hist.act.' + operationType
    const label = $t(key)
    return label === key ? operationType : label
  }
  $: robotWarnDetail = (failure: NonNullable<Robot['last_failure']>) =>
    `${actionLabel(failure.operation_type)} · ${$formatDateTime(failure.at)}: ${failure.message}`
  $: robotWarnTooltip = (failure: NonNullable<Robot['last_failure']>) => $t('robots.warnTitle') + '\n' + robotWarnDetail(failure)
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
      symbolInfos = (await api.getSymbols()).symbols || []
      const quotes: Record<string, string> = {}
      const bases: Record<string, string> = {}
      for (const info of symbolInfos) {
        quotes[info.symbol] = info.quote
        bases[info.symbol] = info.base
      }
      quoteBySymbol = quotes
      baseBySymbol = bases
    } catch {
      symbolInfos = []
    }
  }

  async function loadBalances() {
    try {
      const response = await api.getBalances()
      balancesConnected = response.connected
      spotBalances = Object.fromEntries((response.balances || []).map((balance) => [balance.asset, balance]))
    } catch {
      // Best-effort hint — keep whatever we showed last rather than flashing it away.
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
      $t('ops.pnlBuy', { price: money(operation.purchase_price_per_unit, operation.symbol) }),
      $t('ops.pnlNow', { price: money(current, operation.symbol) }),
      $t('ops.pnlProfit', {
        value: (pnlValue(operation) >= 0 ? '+' : '') + money(pnlValue(operation), operation.symbol),
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
      loadBalances()
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
      loadBalances()
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
      loadBalances()
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
  // NOTE: Positions and History paginate CLIENT-SIDE on purpose, not because pagination is "dumb".
  // The full `operations` set is required on this same screen by AllocationPanel (donut over all OPEN
  // positions) and ProfitabilityPanel (cost basis + realized over ALL operations); `executions` feeds
  // the profitability site-vs-robots split. So the data is already loaded for the charts and the tables
  // just slice it. Making these true server-side paginated (LIMIT/OFFSET) would require moving those
  // aggregations to server-side summary endpoints first — see TODO/backlog. (The access-log table IS
  // real server-side pagination.)
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
          price: money(operation.purchase_price_per_unit, operation.symbol)
        }),
        'success'
      )
      operations = await api.getOperations()
      loadExecutions()
      loadBalances()
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
      loadBalances()
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
      loadBalances()
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
        daily_purchase_enabled: true,
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
      // The robot's single on/off is is_enabled; keep the (now vestigial) daily flag mirroring it.
      robotDraft.daily_purchase_enabled = robotDraft.is_enabled
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
    loadBalances()
    // Keep the Positions P/L arrows, the available-balance hints and the display-currency rates fresh
    // (backend caches: prices 5s, balances 15s — these polls are cheap).
    pricesTimer = setInterval(() => {
      // No point refreshing a dashboard nobody is looking at — and the balance poll is a real
      // (weight-20) Binance call per user, so skip ticks while the tab is hidden.
      if (typeof document !== 'undefined' && document.hidden) return
      if (opsView === 'positions') loadPositionPrices()
      loadBalances()
      loadRates()
    }, 30000)
  })

  onDestroy(() => {
    if (pricesTimer) clearInterval(pricesTimer)
  })
</script>

<main class="page stack-lg">
  {#if loadingError}<div class="card error">{loadingError}</div>{/if}

  <Collapsible variant="section" open title={$t('start.title')}>
    <p class="muted mt-3">{$t('start.intro')}</p>
    <ol class="steps">
      <li>{$t('start.s1')}</li>
      <li>{$t('start.s2')}</li>
      <li>{$t('start.s3')}</li>
      <li>{$t('start.s4')}</li>
    </ol>
    <p class="muted mt-3">{$t('start.reliability')}</p>
  </Collapsible>

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
      <Collapsible variant="help" title={$t('help.summary')}><p>{$t('binance.help')}</p></Collapsible>

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
      <button class="btn-primary btn-block mt-5" disabled={credBusy} on:click={saveCredentials}>
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
        <Collapsible variant="help" title={$t('help.summary')}><p>{$t('buy.help')}</p></Collapsible>
        <div class="field">
          <label for="trade-symbol">{$t('buy.pair')}</label>
          <SymbolAutocomplete id="trade-symbol" bind:value={tradeSymbol} options={symbols} placeholder="BTCUSDT" on:select={checkPrice} on:commit={checkPrice} />
        </div>
        {#if tradePrice !== null}<div class="muted mt-2">{$t('buy.currentPrice', { price: money(tradePrice, tradeSymbol) })}</div>{/if}
        {#if balancesConnected && quoteOf(tradeSymbol)}
          <BalanceCallout quote={quoteOf(tradeSymbol)} free={freeOf(quoteOf(tradeSymbol))} />
        {/if}
        <div class="field">
          <label for="trade-amount">{quoteOf(tradeSymbol) ? $t('buy.amountIn', { quote: quoteOf(tradeSymbol) }) : $t('buy.amount')}</label>
          <input id="trade-amount" type="number" bind:value={tradeAmount} min="0" step="0.01" />
          {#if tradeFilters && tradeFilters.min_notional > 0}
            <span class="muted">{$t('buy.minOrder', { min: money(tradeFilters.min_notional, tradeSymbol) })}</span>
          {/if}
          {#if quoteOf(tradeSymbol)}
            <span class="muted">{$t('buy.spotHint', { quote: quoteOf(tradeSymbol) })}</span>
          {/if}
          {#if belowMinimum && tradeFilters}<span class="error">{$t('buy.belowMin', { min: money(tradeFilters.min_notional, tradeSymbol) })}</span>{/if}
        </div>
        <div class="field">
          <label for="trade-target">{$t('buy.target')}</label>
          <input id="trade-target" type="number" bind:value={tradeTarget} min="0" step="0.01" />
        </div>
        <button class="btn-primary btn-block mt-5" disabled={tradeBusy || belowMinimum || !(tradeAmount > 0)} on:click={buy}>
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
        <Collapsible variant="help" title={$t('help.summary')}><p>{$t('robots.help')}</p></Collapsible>
        <p class="muted mt-3">{isAdmin ? $t('robots.planAdmin') : $t('robots.planStandard', { n: robotLimit })}</p>
        {#if selectedRobot && robotDraft}
          <button class="btn-sm ghost robot-nav-btn" on:click={backToRobotList}>{$t('robots.back')}</button>
        {:else if !creatingRobot}
          <button class="btn-sm btn-primary robot-nav-btn" disabled={!canCreateRobot} on:click={() => { creatingRobot = true; robotErr = ''; robotMsg = '' }}>{$t('robots.new')}</button>
        {/if}
        {#if robotMsg}<p class="success mt-2">✓ {robotMsg}</p>{/if}
        {#if robotErr}<p class="error mt-2">{robotErr}</p>{/if}

        {#if selectedRobot && robotDraft}
          <div class="bot-status" class:on={robotDraft.is_enabled && robotDraft.capital_threshold > 0}>
            <div class="bot-head">
              <span class="badge {robotDraft.is_enabled ? 'green' : 'amber'}">{robotDraft.is_enabled ? $t('robots.on') : $t('robots.off')}</span>
              <strong>{robotDraft.symbol}</strong>
              <span class="spacer"></span>
              <label class="switch-inline"><input type="checkbox" bind:checked={robotDraft.is_enabled} /> {$t('robots.master')}</label>
            </div>
            {#if selectedRobot?.last_failure}
              <p class="warn">⚠ {$t('robots.warnTitle')} — {robotWarnDetail(selectedRobot.last_failure)}</p>
            {/if}
            {#if robotDraft.is_enabled && robotDraft.capital_threshold > 0}
              <p class="muted">{$t('bot.summary', { time: formatHour(robotDailyHourLocal), capital: money(robotDraft.capital_threshold, robotDraft.symbol), symbol: robotDraft.symbol, target: robotDraft.target_profit_percent })}</p>
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
          <div class="field mt-4">
            <label for="robot-coin">{$t('robots.coin')}</label>
            <input id="robot-coin" value={robotDraft.symbol} disabled />
            {#if quoteOf(robotDraft.symbol)}<span class="muted">{$t('buy.spotHint', { quote: quoteOf(robotDraft.symbol) })}</span>{/if}
          </div>
          {#if balancesConnected && quoteOf(robotDraft.symbol)}
            <BalanceCallout quote={quoteOf(robotDraft.symbol)} free={freeOf(quoteOf(robotDraft.symbol))} />
          {/if}
          <div class="grid-2 mt-4">
            <div class="field" style="margin-top:0">
              <label for="robot-capital">{$t('settings.capital')}{quoteOf(robotDraft.symbol) ? ' (' + quoteOf(robotDraft.symbol) + ')' : ''}</label>
              <input id="robot-capital" type="number" bind:value={robotDraft.capital_threshold} min="0" step="0.01" />
            </div>
            <div class="field" style="margin-top:0">
              <label for="robot-max-invested">{$t('settings.maxInvested')}{quoteOf(robotDraft.symbol) ? ' (' + quoteOf(robotDraft.symbol) + ')' : ''}</label>
              <input id="robot-max-invested" type="number" bind:value={robotDraft.max_invested} min="0" step="0.01" placeholder={$t('settings.maxInvestedNone')} />
            </div>
          </div>
          <p class="muted">{$t('settings.maxInvestedHelp')}{#if maxOrderQuoteAmount > 0} {$t('settings.maxOrderHelp', { max: money(maxOrderQuoteAmount, robotDraft.symbol) })}{/if}</p>
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
            <button class="danger btn-sm" disabled={robotBusy} on:click={() => robotDraft && deleteRobot(robotDraft.id)}>{$t('robots.delete')}</button>
            <button class="btn-sm btn-primary" disabled={robotBusy} on:click={saveRobot}>{robotBusy ? $t('settings.saving') : $t('settings.save')}</button>
          </div>
        {:else if creatingRobot}
          <div class="field">
            <label for="new-robot-symbol">{$t('robots.coin')}</label>
            <SymbolAutocomplete id="new-robot-symbol" bind:value={newRobotSymbol} options={symbols} placeholder="BTCUSDT" />
          </div>
          <div class="robot-editor-actions mt-4">
            <button class="btn-sm ghost" disabled={robotBusy} on:click={() => (creatingRobot = false)}>{$t('common.cancel')}</button>
            <button class="btn-sm btn-primary" disabled={robotBusy || !newRobotSymbol} on:click={createRobot}>{robotBusy ? $t('robots.creating') : $t('robots.create')}</button>
          </div>
        {:else}
          {#if robots.length === 0}
            <p class="muted mt-3">{$t('robots.none')}</p>
          {:else}
            <div class="robot-list mt-3">
              {#each robots as robot (robot.id)}
                <button class="robot-row" on:click={() => selectRobot(robot)}>
                  <span class="badge-wrap">
                    <span class="badge {robot.is_enabled ? 'green' : 'amber'}">{robot.is_enabled ? $t('robots.on') : $t('robots.off')}</span>
                    {#if robot.last_failure}
                      <span class="robot-warn" role="img" aria-label={robotWarnTooltip(robot.last_failure)} title={robotWarnTooltip(robot.last_failure)}>⚠</span>
                    {/if}
                  </span>
                  <strong class="robot-name">{robot.name}</strong>
                  <span class="muted robot-sym">{robot.symbol}</span>
                  <span class="spacer"></span>
                  {#if robot.daily_purchase_enabled && robot.capital_threshold > 0}
                    <span class="muted robot-dca">DCA {money(robot.capital_threshold, robot.symbol)} · {formatHour(utcHourToLocal(robot.daily_purchase_hour_utc))}</span>
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

    <Collapsible variant="section" open title={$t('alloc.section')}>

      <div class="subtabs mt-4">
        <button class="subtab" class:active={allocView === 'allocation'} on:click={() => (allocView = 'allocation')}>{$t('alloc.tabAllocation')}</button>
        <button class="subtab" class:active={allocView === 'profit'} on:click={() => (allocView = 'profit')}>{$t('alloc.tabProfit')}</button>
      </div>

      <div class="perf-toolbar mt-3">
        <label class="display-pick" title={$t('display.convertedNote', { code: displayCode })}>
          <span class="muted">{$t('display.currency')}</span>
          <select value={displayCode} on:change={onDisplayCurrencyChange}>
            {#each displayOptions as option (option)}<option value={option}>{option}</option>{/each}
          </select>
        </label>
        {#if balancesConnected && allQuoteCurrencies.length}
          <div class="avail-line" title={availableTooltip}>
            <span class="muted avail-label">{$t('display.available')}:</span>
            <strong class="avail-value">{formatConvertedTotal(availableParts, displayCode, rates, $intlLocale)}</strong>
          </div>
        {/if}
      </div>

      {#if allocView === 'allocation'}
        <Collapsible variant="help" title={$t('help.summary')}><p>{$t('alloc.help')}</p></Collapsible>
        <AllocationPanel {operations} {rates} {displayCode} {quoteBySymbol} />
      {:else}
        <Collapsible variant="help" title={$t('help.summary')}><p>{$t('prof.help')}</p></Collapsible>
        {#if !hasProfitData}
          <p class="muted mt-3">{$t('prof.none')}</p>
        {:else}
          <div class="prof-grid mt-3">
            <div class="prof-card">
              <span class="prof-label">{$t('prof.spent')}</span>
              <span class="prof-value">{formatConvertedTotal(investedByQuote, displayCode, rates, $intlLocale)}</span>
              <span class="prof-split">{$t('prof.you')}: {formatConvertedTotal(spentBySiteByQuote, displayCode, rates, $intlLocale)} · {$t('prof.robots')}: {formatConvertedTotal(spentByRobotsByQuote, displayCode, rates, $intlLocale)}</span>
            </div>
            <div class="prof-card">
              <span class="prof-label">{$t('prof.received')}</span>
              <span class="prof-value">{formatConvertedTotal(receivedByQuote, displayCode, rates, $intlLocale)}</span>
              <span class="prof-split">{$t('prof.you')}: {formatConvertedTotal(earnedBySiteByQuote, displayCode, rates, $intlLocale)} · {$t('prof.robots')}: {formatConvertedTotal(earnedByRobotsByQuote, displayCode, rates, $intlLocale)}</span>
            </div>
            <div class="prof-card">
              <span class="prof-label">{$t('prof.realized')}</span>
              <span class="prof-value {!realizedMixed && realizedTotal > 0 ? 'pos' : !realizedMixed && realizedTotal < 0 ? 'neg' : ''}">{formatConvertedTotal(realizedByQuote, displayCode, rates, $intlLocale, true)}</span>
              <span class="prof-split">{$t('prof.openCost')}: {formatConvertedTotal(openCostByQuote, displayCode, rates, $intlLocale)}</span>
            </div>
          </div>
          <p class="muted converted-note mt-2">{$t('display.convertedNote', { code: displayCode })}</p>
          <div class="prof-coins mt-4">
            <span class="prof-label">{$t('prof.acquired')}</span>
            <div class="coin-chips">
              {#each acquiredBySymbol as item (item.symbol)}
                <span class="coin-chip">{fmt(item.quantity)} {baseOf(item.symbol)}</span>
              {/each}
            </div>
          </div>
          <div class="prof-chart-block mt-5">
            <ProfitabilityPanel {operations} {rates} {displayCode} {quoteBySymbol} />
          </div>
          <p class="muted mt-4">{$t('prof.taxNote')}</p>
        {/if}
      {/if}
    </Collapsible>

    <section class="card">
      <div class="card-header">
        <span class="card-title">{$t('ops.title')}</span>
      </div>
      <div class="subtabs mt-4">
        <button class="subtab" class:active={opsView === 'positions'} on:click={() => (opsView = 'positions')}>{$t('ops.tabPositions')}</button>
        <button class="subtab" class:active={opsView === 'history'} on:click={() => (opsView = 'history')}>{$t('ops.tabHistory')}</button>
      </div>

      {#if opsView === 'positions'}
        <Collapsible variant="help" title={$t('ops.statusHelp')}>
          <p>{$t('ops.openMeaning')}</p>
          <p>{$t('ops.soldMeaning')}</p>
          <p>{$t('ops.sellOrderMeaning')}</p>
        </Collapsible>
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
                <div data-label={$t('ops.qty')}>{fmt(operation.quantity)} {baseOf(operation.symbol)}</div>
                <div data-label={$t('ops.buyPrice')}>{money(operation.purchase_price_per_unit, operation.symbol)}</div>
                <div data-label={$t('ops.target')}>{operation.sell_target_price_per_unit === null ? '—' : money(operation.sell_target_price_per_unit, operation.symbol)}</div>
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
                      <button class="btn-sm btn-primary" disabled={placeSellBusyId === operation.id} on:click={() => placeSell(operation.id)}>
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
                <div data-label={$t('hist.price')}>{money(execution.unit_price, execution.symbol)}</div>
                <div data-label={$t('hist.qty')}>{fmt(execution.quantity)} {baseOf(execution.symbol)}</div>
                <div data-label={$t('hist.total')}>{money(execution.total_value, execution.symbol)}</div>
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
  .steps { margin: var(--space-3) 0 0; padding-left: var(--space-5); line-height: 1.8; color: var(--text); }

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
  .bot-head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); margin-bottom: var(--space-2); }
  .bot-status p { margin-top: var(--space-1); line-height: 1.5; }
  .bot-status .warn { color: var(--amber); font-size: var(--text-sm); }

  .stack-title { display: flex; flex-direction: column; }
  .switch-inline { display: inline-flex; align-items: center; gap: var(--space-1); font-size: var(--text-xs); font-weight: 600; white-space: nowrap; }
  .robot-nav-btn { display: inline-flex; margin: var(--space-3) 0; }
  .robot-editor-actions { display: flex; justify-content: space-between; gap: var(--space-2); }
  .robot-list { display: flex; flex-direction: column; gap: var(--space-2); }
  .robot-row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); width: 100%; text-align: left; background: var(--surface-2); border: 1px solid var(--border); border-radius: var(--radius-md); padding: var(--space-2) var(--space-3); color: var(--text); font: inherit; height: auto; }
  .robot-row:hover:not(:disabled) { border-color: var(--brand); filter: none; }
  .robot-name { font-weight: 700; }
  .robot-sym, .robot-dca { font-size: var(--text-xs); }
  .robot-open { color: var(--brand); font-weight: 700; font-size: var(--text-xs); white-space: nowrap; }
  /* Mobile: a balanced 3-zone grid — badge left, name centered, pair right (row 1);
     DCA left, "Abrir →" right (row 2) — instead of everything bunched against the left edge. */
  @media (max-width: 600px) {
    .robot-row { display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; column-gap: var(--space-2); row-gap: var(--space-1); }
    .robot-row .spacer { display: none; }
    .robot-row .badge-wrap { grid-row: 1; grid-column: 1; justify-self: start; }
    .robot-row .robot-name { grid-row: 1; grid-column: 2; justify-self: center; text-align: center; }
    .robot-row .robot-sym { grid-row: 1; grid-column: 3; justify-self: end; }
    .robot-row .robot-dca { grid-row: 2; grid-column: 1 / 3; justify-self: start; }
    .robot-row .robot-open { grid-row: 2; grid-column: 3; justify-self: end; }
  }
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
  /* The toolbar stacks: currency picker first, the available-to-buy line right below it — the value
     follows the SELECTED display currency, so the two read as one unit. */
  .perf-toolbar { display: flex; flex-direction: column; align-items: flex-start; gap: var(--space-2); }
  .display-pick { display: inline-flex; align-items: center; gap: var(--space-2); font-size: var(--text-sm); cursor: pointer; }
  .display-pick select { width: auto; height: 2rem; padding: 0 var(--space-2); font-size: var(--text-sm); font-weight: 700; }
  .avail-line { display: inline-flex; align-items: center; gap: var(--space-2); cursor: help; }
  .avail-label { font-size: var(--text-xs); font-weight: 700; text-transform: uppercase; letter-spacing: 0.02em; }
  .avail-value { color: var(--green); font-weight: 800; font-size: var(--text-sm); }
  .converted-note { font-size: var(--text-xs); }

  /* badge + warning travel together (one grid cell in the mobile 3-zone robot row). */
  .badge-wrap { display: inline-flex; align-items: center; gap: var(--space-2); }
  .robot-warn { color: var(--amber); cursor: help; font-size: var(--text-sm); }
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
