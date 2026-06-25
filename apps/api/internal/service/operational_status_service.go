package service

import (
	"context"
	"time"

	"coin-hub/internal/repository"
)

// workerStaleThreshold is how long without a heartbeat before the automation worker is considered
// stalled. The monitor loop ticks every 30s, so ~3 minutes (≈6 missed ticks) avoids false alarms from
// a single slow tick while still catching a genuinely stuck or dead worker quickly.
const workerStaleThreshold = 3 * time.Minute

// Operational-status reason codes. These are the machine codes the SPA localizes (status.* i18n keys)
// and shows on the header indicator / banner, so the user understands WHY automation is paused.
const (
	StatusReasonBinanceBusy   = "binance_busy"   // Binance is rate-limiting this IP (429); auto-resumes
	StatusReasonBinanceBanned = "binance_banned" // Binance auto-banned this IP (418); auto-resumes
	StatusReasonWorkerStalled = "worker_stalled" // the automation worker hasn't checked in recently
)

// StatusReason explains one thing currently preventing the bots from operating normally.
type StatusReason struct {
	Code         string `json:"code"`
	RetrySeconds int    `json:"retry_seconds,omitempty"` // for transient (auto-resuming) reasons
}

// OperationalStatus is the aggregate health the UI shows: whether automation can operate right now and,
// if not, the reasons.
type OperationalStatus struct {
	Operational bool           `json:"operational"`
	Reasons     []StatusReason `json:"reasons"`
}

// OperationalStatusService aggregates the live signals that decide whether the trading automation can
// run: the worker's heartbeat freshness and the shared Binance rate-limit gate.
type OperationalStatusService struct {
	heartbeatRepository repository.WorkerHeartbeatRepository
}

func NewOperationalStatusService(heartbeatRepository repository.WorkerHeartbeatRepository) *OperationalStatusService {
	return &OperationalStatusService{heartbeatRepository: heartbeatRepository}
}

// Current computes the operational status now. It never errors out to the caller: an unreadable
// heartbeat is treated as "not stalled" (fail open) so a transient DB read can't paint a false outage —
// the dedicated /health/worker probe and the alert email cover a truly dead worker.
func (service *OperationalStatusService) Current(statusContext context.Context) OperationalStatus {
	reasons := make([]StatusReason, 0, 2)

	gate := BinanceRateGateStatus()
	if gate.InCooldown {
		code := StatusReasonBinanceBusy
		if gate.Banned {
			code = StatusReasonBinanceBanned
		}
		reasons = append(reasons, StatusReason{Code: code, RetrySeconds: gate.SecondsRemaining})
	}

	if service.WorkerStalled(statusContext) {
		reasons = append(reasons, StatusReason{Code: StatusReasonWorkerStalled})
	}

	return OperationalStatus{Operational: len(reasons) == 0, Reasons: reasons}
}

// WorkerStalled reports whether the automation worker's last heartbeat is older than the stale
// threshold. A read error is treated as not stalled (fail open).
func (service *OperationalStatusService) WorkerStalled(statusContext context.Context) bool {
	lastTickAt, _, loadError := service.heartbeatRepository.LoadLastTick(statusContext)
	if loadError != nil {
		return false
	}
	return time.Since(lastTickAt) > workerStaleThreshold
}
