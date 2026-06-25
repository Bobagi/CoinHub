package repository

import (
	"context"
	"database/sql"
	"time"
)

// WorkerHeartbeatRepository persists and reads the AutomationWorker's liveness heartbeat (a single
// row). The worker UPDATEs it on every monitor tick; status/health readers load it to decide whether
// automation is alive and recent.
type WorkerHeartbeatRepository interface {
	RecordHeartbeat(updateContext context.Context, instanceIdentifier string) error
	LoadLastTick(loadContext context.Context) (time.Time, string, error)
}

type PostgresWorkerHeartbeatRepository struct {
	Database *sql.DB
}

func NewPostgresWorkerHeartbeatRepository(database *sql.DB) *PostgresWorkerHeartbeatRepository {
	return &PostgresWorkerHeartbeatRepository{Database: database}
}

// RecordHeartbeat stamps the heartbeat row with the current time and the writing instance id.
func (repository *PostgresWorkerHeartbeatRepository) RecordHeartbeat(updateContext context.Context, instanceIdentifier string) error {
	_, executionError := repository.Database.ExecContext(
		updateContext,
		`UPDATE worker_heartbeat SET last_tick_at = NOW(), instance_id = $1 WHERE id = 1`,
		instanceIdentifier,
	)
	return executionError
}

// LoadLastTick returns when the worker last recorded a heartbeat and which instance wrote it.
func (repository *PostgresWorkerHeartbeatRepository) LoadLastTick(loadContext context.Context) (time.Time, string, error) {
	var lastTickAt time.Time
	var instanceIdentifier string
	scanError := repository.Database.QueryRowContext(
		loadContext,
		`SELECT last_tick_at, instance_id FROM worker_heartbeat WHERE id = 1`,
	).Scan(&lastTickAt, &instanceIdentifier)
	if scanError != nil {
		return time.Time{}, "", scanError
	}
	return lastTickAt, instanceIdentifier, nil
}
