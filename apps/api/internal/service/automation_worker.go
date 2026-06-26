package service

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"coin-hub/internal/domain"
	"coin-hub/internal/repository"
)

type activeUserLister interface {
	ListActiveUserIdentifiers(loadContext context.Context) ([]int64, error)
	// ListActiveUserIdentifiersForShard returns only the users in this worker's shard (id % count ==
	// index), so several worker instances can process disjoint slices of users in parallel.
	ListActiveUserIdentifiersForShard(loadContext context.Context, shardCount int, shardIndex int) ([]int64, error)
}

type dailyPurchaseGuard interface {
	HasSuccessfulExecutionOfTypeSince(loadContext context.Context, userIdentifier int64, environment string, operationType string, tradingPairSymbol string, since time.Time) (bool, error)
}

// workerStalledAlerter is notified when the worker's heartbeat goes stale (e.g. a stuck loop while the
// process is still up), so an operator can be paged. Implemented by OpsAlertService.
type workerStalledAlerter interface {
	AlertWorkerStalled(alertContext context.Context, staleFor time.Duration)
}

// leadershipAcquirer is the worker's view of the leader lock: try to become the single active worker.
type leadershipAcquirer interface {
	TryAcquire(acquireContext context.Context) (bool, error)
	Release(releaseContext context.Context)
}

// AutomationWorker runs per-user background trading automation: it reconciles filled take-profit
// orders, enforces stop-loss, and runs the daily DCA purchase. It iterates every active user that
// has connected Binance credentials.
//
// It runs only while it holds leadership (a Postgres advisory lock), so it is a guaranteed singleton
// even if the API is scaled to several replicas. On every monitor tick it records a heartbeat, and a
// watchdog alerts operators if that heartbeat goes stale.
type AutomationWorker struct {
	userLister          activeUserLister
	credentialService   *UserCredentialService
	robotRepository     repository.TradingRobotRepository
	operationRepository repository.UserTradingOperationRepository
	executionRepository repository.UserTradingOperationExecutionRepository
	purchaseGuard       dailyPurchaseGuard
	tradingService      *UserTradingService
	heartbeatRepository repository.WorkerHeartbeatRepository
	leaderLock          leadershipAcquirer
	alerter             workerStalledAlerter
	instanceIdentifier  string
	shardCount          int
	shardIndex          int
	monitorInterval     time.Duration
	// WebSocket accelerators (best-effort; the REST poller above remains the correctness backstop):
	// pushed market prices into the shared cache, and pushed order events that trigger an early reconcile.
	marketStreams   *MarketStreamManager
	userDataStreams *UserDataStreamManager
}

func NewAutomationWorker(
	userLister activeUserLister,
	credentialService *UserCredentialService,
	robotRepository repository.TradingRobotRepository,
	operationRepository repository.UserTradingOperationRepository,
	executionRepository repository.UserTradingOperationExecutionRepository,
	purchaseGuard dailyPurchaseGuard,
	tradingService *UserTradingService,
	heartbeatRepository repository.WorkerHeartbeatRepository,
	leaderLock leadershipAcquirer,
	alerter workerStalledAlerter,
	shardCount int,
	shardIndex int,
	monitorInterval time.Duration,
) *AutomationWorker {
	if monitorInterval <= 0 {
		monitorInterval = 30 * time.Second
	}
	if shardCount < 1 {
		shardCount = 1
	}
	if shardIndex < 0 || shardIndex >= shardCount {
		shardIndex = 0
	}
	hostname, _ := os.Hostname()
	return &AutomationWorker{
		userLister:          userLister,
		credentialService:   credentialService,
		robotRepository:     robotRepository,
		operationRepository: operationRepository,
		executionRepository: executionRepository,
		purchaseGuard:       purchaseGuard,
		tradingService:      tradingService,
		heartbeatRepository: heartbeatRepository,
		leaderLock:          leaderLock,
		alerter:             alerter,
		instanceIdentifier:  hostname,
		shardCount:          shardCount,
		shardIndex:          shardIndex,
		monitorInterval:     monitorInterval,
	}
}

// Start launches the leadership manager. The trading loops only run on the replica that wins the
// advisory lock; the rest stay passive (HTTP keeps serving) and periodically retry to take over.
func (worker *AutomationWorker) Start(applicationContext context.Context) {
	// WebSocket accelerators. They live for the process lifetime but only do work once the worker leads
	// (Watch/EnsureUsers are only called from the leader's monitor loop), so non-leaders open no sockets.
	worker.marketStreams = NewMarketStreamManager(applicationContext)
	worker.userDataStreams = NewUserDataStreamManager(applicationContext, worker.credentialService, worker.monitorUser)
	go worker.runLeadership(applicationContext)
	if worker.shardCount > 1 {
		log.Printf("Automation worker starting (monitor interval %s, shard %d/%d); awaiting leadership", worker.monitorInterval, worker.shardIndex, worker.shardCount)
	} else {
		log.Printf("Automation worker starting (monitor interval %s); awaiting leadership", worker.monitorInterval)
	}
}

// runLeadership keeps trying to become the single active worker. Once it leads, it starts the trading
// loops + heartbeat + watchdog and holds leadership for the process lifetime (the advisory lock is held
// on a dedicated connection that releases on shutdown or crash, letting another replica take over).
func (worker *AutomationWorker) runLeadership(applicationContext context.Context) {
	const leadershipRetryInterval = 15 * time.Second
	for {
		if applicationContext.Err() != nil {
			return
		}
		acquired, acquireError := worker.leaderLock.TryAcquire(applicationContext)
		if acquireError != nil {
			// Fail-open: TryAcquire returns acquired=true on a lock-layer fault so a broken lock never
			// silently stops ALL trading. With a single replica this is the right call.
			log.Printf("automation: leader lock errored (%v) — running worker anyway (single-instance assumption)", acquireError)
		}
		if !acquired {
			select {
			case <-applicationContext.Done():
				return
			case <-time.After(leadershipRetryInterval):
				continue
			}
		}

		log.Println("automation: acquired leadership — starting trading loops")
		worker.recordHeartbeatSafely(applicationContext) // fresh heartbeat immediately on takeover
		go worker.runMonitorLoop(applicationContext)
		go worker.runDailyPurchaseLoop(applicationContext)
		go worker.runWatchdogLoop(applicationContext)

		<-applicationContext.Done()
		worker.leaderLock.Release(context.Background())
		log.Println("automation: released leadership on shutdown")
		return
	}
}

// recordHeartbeatSafely stamps the liveness heartbeat; failures are logged, never fatal.
func (worker *AutomationWorker) recordHeartbeatSafely(parentContext context.Context) {
	writeContext, cancel := context.WithTimeout(parentContext, 5*time.Second)
	defer cancel()
	if heartbeatError := worker.heartbeatRepository.RecordHeartbeat(writeContext, worker.instanceIdentifier); heartbeatError != nil {
		log.Printf("automation: could not record heartbeat: %v", heartbeatError)
	}
}

// runWatchdogLoop emails operators if the heartbeat goes stale (a stuck loop while the process is still
// alive — a dead process is caught by the external /health/worker probe instead). Debounced: it alerts
// once per stall episode and re-arms only after the worker recovers.
func (worker *AutomationWorker) runWatchdogLoop(applicationContext context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	alreadyAlerted := false
	for {
		select {
		case <-applicationContext.Done():
			return
		case <-ticker.C:
			stale, staleFor := worker.heartbeatStale(applicationContext)
			switch {
			case stale && !alreadyAlerted:
				log.Printf("automation: worker heartbeat stale for %s — alerting operators", staleFor.Round(time.Second))
				if worker.alerter != nil {
					worker.alerter.AlertWorkerStalled(applicationContext, staleFor)
				}
				alreadyAlerted = true
			case !stale:
				alreadyAlerted = false
			}
		}
	}
}

func (worker *AutomationWorker) heartbeatStale(parentContext context.Context) (bool, time.Duration) {
	readContext, cancel := context.WithTimeout(parentContext, 5*time.Second)
	defer cancel()
	lastTickAt, _, loadError := worker.heartbeatRepository.LoadLastTick(readContext)
	if loadError != nil {
		return false, 0
	}
	since := time.Since(lastTickAt)
	return since > workerStaleThreshold, since
}

func (worker *AutomationWorker) runMonitorLoop(applicationContext context.Context) {
	ticker := time.NewTicker(worker.monitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-applicationContext.Done():
			log.Println("Automation monitor loop stopped")
			return
		case <-ticker.C:
			worker.monitorAllUsers(applicationContext)
		}
	}
}

func (worker *AutomationWorker) monitorAllUsers(applicationContext context.Context) {
	// Record liveness at the START of the tick too: even if listing users or a user step is slow, a
	// completed tick proves the loop is running. A final stamp below marks a fully-processed tick.
	worker.recordHeartbeatSafely(applicationContext)

	userIdentifiers, listError := worker.userLister.ListActiveUserIdentifiersForShard(applicationContext, worker.shardCount, worker.shardIndex)
	if listError != nil {
		log.Printf("automation: could not list active users: %v", listError)
		return
	}
	// Keep a user-data WebSocket per active user (push order fills → early reconcile). Best-effort; the
	// loop below still reconciles everyone regardless.
	if worker.userDataStreams != nil {
		worker.userDataStreams.EnsureUsers(userIdentifiers)
	}
	for _, userIdentifier := range userIdentifiers {
		// Isolate each user: a panic while processing one user (e.g. an unexpected nil from Binance) is
		// recovered so it never tears down the whole automation goroutine and stalls every other user.
		runUserStepSafely(userIdentifier, "monitor", func() {
			worker.monitorUser(applicationContext, userIdentifier)
		})
	}

	worker.recordHeartbeatSafely(applicationContext)
}

// runUserStepSafely runs one user's automation step, recovering from any panic so a single user can
// never crash the shared worker loop (an unrecovered panic in a goroutine terminates the process).
func runUserStepSafely(userIdentifier int64, step string, run func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("automation: recovered from panic in %s for user %d: %v", step, userIdentifier, recovered)
		}
	}()
	run()
}

func (worker *AutomationWorker) monitorUser(applicationContext context.Context, userIdentifier int64) {
	environmentConfiguration, configurationError := worker.credentialService.LoadActiveEnvironmentConfiguration(applicationContext, userIdentifier)
	if configurationError != nil || environmentConfiguration == nil {
		return
	}

	openOperations, listError := worker.operationRepository.ListOpenOperationsForUser(applicationContext, userIdentifier, environmentConfiguration.EnvironmentName)
	if listError != nil {
		log.Printf("automation: open operations for user %d failed: %v", userIdentifier, listError)
		return
	}
	if len(openOperations) == 0 {
		return
	}

	// Tell the market stream which coins to push live prices for (feeds the shared cache that resolvePrice
	// reads). Best-effort; resolvePrice falls back to REST on a cache miss.
	if worker.marketStreams != nil {
		watchedSymbols := make([]string, 0, len(openOperations))
		for _, operation := range openOperations {
			watchedSymbols = append(watchedSymbols, operation.TradingPairSymbol)
		}
		worker.marketStreams.Watch(environmentConfiguration.RESTBaseURL, watchedSymbols)
	}

	// Stop-loss is configured per robot (one per coin). Map each coin to its robot's stop-loss so an
	// open position is judged against the robot that trades that coin (or no stop-loss if none).
	robots, _ := worker.robotRepository.ListRobotsForUser(applicationContext, userIdentifier, environmentConfiguration.EnvironmentName)
	stopLossBySymbol := make(map[string]*float64)
	for _, robot := range robots {
		if robot.IsEnabled {
			stopLossBySymbol[robot.TradingPairSymbol] = robot.StopLossPercent
		}
	}

	tradingService := NewBinanceTradingService(*environmentConfiguration)
	priceService := NewBinancePriceService(*environmentConfiguration)
	priceBySymbol := make(map[string]float64)

	resolvePrice := func(tradingPairSymbol string) (float64, bool) {
		if cachedPrice, present := priceBySymbol[tradingPairSymbol]; present {
			return cachedPrice, true
		}
		currentPrice, priceError := priceService.GetCurrentPrice(applicationContext, tradingPairSymbol)
		if priceError != nil {
			return 0, false
		}
		priceBySymbol[tradingPairSymbol] = currentPrice
		return currentPrice, true
	}

	for _, openOperation := range openOperations {
		worker.processOpenOperation(applicationContext, userIdentifier, openOperation, stopLossBySymbol[openOperation.TradingPairSymbol], tradingService, resolvePrice)
	}
}

func (worker *AutomationWorker) processOpenOperation(applicationContext context.Context, userIdentifier int64, operation domain.TradingOperation, stopLossPercent *float64, tradingService *BinanceTradingService, resolvePrice func(string) (float64, bool)) {
	// 1) Reconcile the resting take-profit limit sell against Binance.
	if operation.SellOrderIdentifier != nil {
		orderStatus, statusError := tradingService.GetOrderStatus(applicationContext, operation.TradingPairSymbol, *operation.SellOrderIdentifier)
		if statusError == nil && orderStatus != nil {
			switch orderStatus.Status {
			case "FILLED":
				worker.markOperationSold(applicationContext, userIdentifier, operation, fillPriceFromStatus(*orderStatus, operation.PurchasePricePerUnit), "take-profit filled")
				return
			case "CANCELED", "EXPIRED", "REJECTED":
				// Removed outside the app (e.g. the user cancelled it in the Binance app).
				worker.markOperationCanceledExternally(applicationContext, userIdentifier, operation)
				return
			}
			// Still resting: enforce the app-side validity window (Binance spot LIMIT has no native expiry).
			if operation.SellOrderExpiresAt != nil && time.Now().After(*operation.SellOrderExpiresAt) {
				worker.expireSellOrder(applicationContext, userIdentifier, operation, tradingService)
				return
			}
		}
	}

	// 2) Stop-loss: if this coin's robot has one configured and the price fell below it, sell now.
	if stopLossPercent == nil || *stopLossPercent <= 0 {
		return
	}
	currentPrice, pricePresent := resolvePrice(operation.TradingPairSymbol)
	if !pricePresent {
		return
	}
	stopLossThreshold := operation.PurchasePricePerUnit * (1 - (*stopLossPercent / 100))
	if currentPrice > stopLossThreshold {
		return
	}

	// Free the balance held by the resting limit sell before selling at market.
	if operation.SellOrderIdentifier != nil {
		if cancelError := tradingService.CancelOrder(applicationContext, operation.TradingPairSymbol, *operation.SellOrderIdentifier); cancelError != nil {
			// The cancel may have failed because the order just filled — reconcile that case.
			if orderStatus, statusError := tradingService.GetOrderStatus(applicationContext, operation.TradingPairSymbol, *operation.SellOrderIdentifier); statusError == nil && orderStatus != nil && orderStatus.Status == "FILLED" {
				worker.markOperationSold(applicationContext, userIdentifier, operation, fillPriceFromStatus(*orderStatus, operation.PurchasePricePerUnit), "take-profit filled")
			} else {
				log.Printf("automation: stop-loss cancel failed for operation %d (user %d): %v", operation.Identifier, userIdentifier, cancelError)
			}
			return
		}
	}

	sellResponse, sellError := tradingService.PlaceMarketSellByQuantity(applicationContext, operation.TradingPairSymbol, operation.QuantityPurchased)
	if sellError != nil {
		worker.logSellExecution(applicationContext, userIdentifier, operation.BinanceEnvironment, domain.ExecutionInitiatorBot, operation.TradingPairSymbol, currentPrice, operation.QuantityPurchased, false, sellError, nil)
		log.Printf("automation: stop-loss market sell failed for operation %d (user %d): %v", operation.Identifier, userIdentifier, sellError)
		return
	}
	worker.markOperationSold(applicationContext, userIdentifier, operation, fillPriceFromOrder(*sellResponse, currentPrice), "stop-loss")
}

func (worker *AutomationWorker) markOperationSold(applicationContext context.Context, userIdentifier int64, operation domain.TradingOperation, fillPrice float64, reason string) {
	if updateError := worker.operationRepository.UpdateOperationAsSoldForUser(applicationContext, userIdentifier, operation.Identifier, fillPrice); updateError != nil {
		log.Printf("automation: could not mark operation %d sold (user %d): %v", operation.Identifier, userIdentifier, updateError)
		return
	}
	worker.logSellExecution(applicationContext, userIdentifier, operation.BinanceEnvironment, domain.ExecutionInitiatorBot, operation.TradingPairSymbol, fillPrice, operation.QuantityPurchased, true, nil, operation.SellOrderIdentifier)
	log.Printf("automation: closed operation %d (user %d) via %s at %.8f", operation.Identifier, userIdentifier, reason, fillPrice)
}

// markOperationCanceledExternally handles a take-profit that was cancelled outside the app: it closes
// the operation as CANCELED (drops it from the active positions view) and records a history event.
func (worker *AutomationWorker) markOperationCanceledExternally(applicationContext context.Context, userIdentifier int64, operation domain.TradingOperation) {
	if updateError := worker.operationRepository.MarkOperationCanceledForUser(applicationContext, userIdentifier, operation.Identifier); updateError != nil {
		log.Printf("automation: could not mark operation %d canceled (user %d): %v", operation.Identifier, userIdentifier, updateError)
		return
	}
	worker.logTakeProfitEvent(applicationContext, userIdentifier, operation, domain.TradingOperationTypeSellCancel, domain.ExecutionInitiatorUser)
	log.Printf("automation: operation %d (user %d) take-profit cancelled externally; position released", operation.Identifier, userIdentifier)
}

// expireSellOrder cancels a take-profit that reached its validity window, leaving the position OPEN
// but unprotected (⚠) so the user can re-place it or sell. Records a history event.
func (worker *AutomationWorker) expireSellOrder(applicationContext context.Context, userIdentifier int64, operation domain.TradingOperation, tradingService *BinanceTradingService) {
	if operation.SellOrderIdentifier != nil {
		if cancelError := tradingService.CancelOrder(applicationContext, operation.TradingPairSymbol, *operation.SellOrderIdentifier); cancelError != nil {
			// If it actually filled meanwhile, reconcile to sold instead of expiring it.
			if orderStatus, statusError := tradingService.GetOrderStatus(applicationContext, operation.TradingPairSymbol, *operation.SellOrderIdentifier); statusError == nil && orderStatus != nil && orderStatus.Status == "FILLED" {
				worker.markOperationSold(applicationContext, userIdentifier, operation, fillPriceFromStatus(*orderStatus, operation.PurchasePricePerUnit), "take-profit filled")
				return
			}
			log.Printf("automation: could not cancel expired sell order for operation %d (user %d): %v", operation.Identifier, userIdentifier, cancelError)
			return
		}
	}
	if clearError := worker.operationRepository.ClearSellOrderForUser(applicationContext, userIdentifier, operation.Identifier); clearError != nil {
		log.Printf("automation: could not clear expired sell order for operation %d (user %d): %v", operation.Identifier, userIdentifier, clearError)
		return
	}
	worker.logTakeProfitEvent(applicationContext, userIdentifier, operation, domain.TradingOperationTypeSellExpire, domain.ExecutionInitiatorBot)
	log.Printf("automation: take-profit for operation %d (user %d) reached its validity and was cancelled", operation.Identifier, userIdentifier)
}

// logTakeProfitEvent records a non-trade history event (cancel/expire) for a take-profit order.
func (worker *AutomationWorker) logTakeProfitEvent(applicationContext context.Context, userIdentifier int64, operation domain.TradingOperation, operationType string, initiatedBy string) {
	_, _ = worker.executionRepository.LogExecutionForUser(applicationContext, userIdentifier, domain.TradingOperationExecution{
		TradingPairSymbol:  operation.TradingPairSymbol,
		OperationType:      operationType,
		BinanceEnvironment: operation.BinanceEnvironment,
		InitiatedBy:        initiatedBy,
		Quantity:           operation.QuantityPurchased,
		ExecutedAt:         time.Now(),
		Success:            true,
		OrderIdentifier:    operation.SellOrderIdentifier,
	})
}

func (worker *AutomationWorker) logSellExecution(applicationContext context.Context, userIdentifier int64, environment string, initiatedBy string, tradingPairSymbol string, unitPrice float64, quantity float64, success bool, cause error, orderIdentifier *string) {
	var errorMessage *string
	if cause != nil {
		message := cause.Error()
		errorMessage = &message
	}
	_, _ = worker.executionRepository.LogExecutionForUser(applicationContext, userIdentifier, domain.TradingOperationExecution{
		TradingPairSymbol:  tradingPairSymbol,
		OperationType:      domain.TradingOperationTypeSell,
		BinanceEnvironment: environment,
		InitiatedBy:        initiatedBy,
		UnitPrice:          unitPrice,
		Quantity:           quantity,
		TotalValue:         unitPrice * quantity,
		ExecutedAt:         time.Now(),
		Success:            success,
		ErrorMessage:       errorMessage,
		OrderIdentifier:    orderIdentifier,
	})
}

// dailyPurchaseCheckInterval is how often the daily-buy loop wakes to see whether a robot's scheduled
// hour has arrived. It is the PRECISION of the daily buy: the buy fires on the first tick inside the
// target hour, so a 5-minute interval meant buys landed up to ~5 min late (and scattered, since the
// timer re-aligned on every restart). 30s caps that lateness at ≤30s. The buy is idempotent per day per
// symbol (HasSuccessfulExecutionOfTypeSince), so waking often is safe — it buys once and then no-ops.
const dailyPurchaseCheckInterval = 30 * time.Second

func (worker *AutomationWorker) runDailyPurchaseLoop(applicationContext context.Context) {
	ticker := time.NewTicker(dailyPurchaseCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-applicationContext.Done():
			log.Println("Automation daily purchase loop stopped")
			return
		case <-ticker.C:
			worker.processDailyPurchases(applicationContext)
		}
	}
}

func (worker *AutomationWorker) processDailyPurchases(applicationContext context.Context) {
	userIdentifiers, listError := worker.userLister.ListActiveUserIdentifiersForShard(applicationContext, worker.shardCount, worker.shardIndex)
	if listError != nil {
		return
	}

	nowUTC := time.Now().UTC()
	startOfDayUTC := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)

	for _, userIdentifier := range userIdentifiers {
		runUserStepSafely(userIdentifier, "daily purchase", func() {
			worker.processDailyPurchasesForUser(applicationContext, userIdentifier, nowUTC, startOfDayUTC)
		})
	}
}

func (worker *AutomationWorker) processDailyPurchasesForUser(applicationContext context.Context, userIdentifier int64, nowUTC time.Time, startOfDayUTC time.Time) {
	environmentConfiguration, _ := worker.credentialService.LoadActiveEnvironmentConfiguration(applicationContext, userIdentifier)
	if environmentConfiguration == nil {
		return
	}
	environmentName := environmentConfiguration.EnvironmentName

	// Each robot runs its own daily DCA buy for its coin, independently and idempotently per day.
	robots, _ := worker.robotRepository.ListRobotsForUser(applicationContext, userIdentifier, environmentName)
	for _, robot := range robots {
		// A robot's single on/off is IsEnabled — an enabled robot IS its daily DCA buy (that is the
		// robot's whole purpose), so there is no separate "daily purchase" gate. CapitalThreshold<=0
		// means "configured but no amount set yet", so it just doesn't buy until an amount is given.
		if !robot.IsEnabled || robot.CapitalThreshold <= 0 {
			continue
		}
		if nowUTC.Hour() != robot.DailyPurchaseHourUTC {
			continue
		}
		alreadyPurchased, _ := worker.purchaseGuard.HasSuccessfulExecutionOfTypeSince(applicationContext, userIdentifier, environmentName, domain.TradingOperationTypeDailyBuy, robot.TradingPairSymbol, startOfDayUTC)
		if alreadyPurchased {
			continue
		}

		// Max-invested ceiling: skip the buy while open positions for this coin already hold (or would,
		// after this buy) more than the cap — the robot waits for a take-profit/stop-loss to free capital
		// before buying again. 0 = no cap. Fail-closed (skip) on a read error so we never over-invest.
		if robot.MaxInvested > 0 {
			openAllocation, allocationError := worker.operationRepository.CalculateOpenAllocationForUserSymbol(applicationContext, userIdentifier, environmentName, robot.TradingPairSymbol)
			if allocationError != nil {
				log.Printf("automation: skipping daily buy for user %d robot %d (%s): could not read open allocation: %v", userIdentifier, robot.Identifier, robot.TradingPairSymbol, allocationError)
				continue
			}
			if openAllocation+robot.CapitalThreshold > robot.MaxInvested {
				log.Printf("automation: skipping daily buy for user %d robot %d (%s): open allocation %.2f + buy %.2f would exceed max invested %.2f", userIdentifier, robot.Identifier, robot.TradingPairSymbol, openAllocation, robot.CapitalThreshold, robot.MaxInvested)
				continue
			}
		}

		log.Printf("automation: running daily purchase for user %d robot %d (%s)", userIdentifier, robot.Identifier, robot.TradingPairSymbol)
		if _, purchaseError := worker.tradingService.ExecuteDailyPurchase(applicationContext, userIdentifier, environmentName, robot.TradingPairSymbol, robot.CapitalThreshold, robot.TargetProfitPercent, robot.SellOrderValidityDays); purchaseError != nil {
			log.Printf("automation: daily purchase failed for user %d robot %d: %v", userIdentifier, robot.Identifier, purchaseError)
		}
	}
}

func fillPriceFromStatus(orderStatus BinanceOrderStatus, fallbackPrice float64) float64 {
	executedQuantity, quantityError := strconv.ParseFloat(orderStatus.ExecutedQty, 64)
	cumulativeQuote, quoteError := strconv.ParseFloat(orderStatus.CumulativeQuote, 64)
	if quantityError == nil && quoteError == nil && executedQuantity > 0 && cumulativeQuote > 0 {
		return cumulativeQuote / executedQuantity
	}
	if parsedPrice, priceError := strconv.ParseFloat(orderStatus.Price, 64); priceError == nil && parsedPrice > 0 {
		return parsedPrice
	}
	return fallbackPrice
}

func fillPriceFromOrder(orderResponse binanceOrderResponse, fallbackPrice float64) float64 {
	executedQuantity, quantityError := strconv.ParseFloat(orderResponse.ExecutedQty, 64)
	cumulativeQuote, quoteError := strconv.ParseFloat(orderResponse.CumulativeQuote, 64)
	if quantityError == nil && quoteError == nil && executedQuantity > 0 && cumulativeQuote > 0 {
		return cumulativeQuote / executedQuantity
	}
	return fallbackPrice
}
