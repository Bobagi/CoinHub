package service

import (
	"context"
	"database/sql"
	"log"
)

// LeaderLock guards the single-writer automation worker with a Postgres session-level advisory lock.
// Exactly one API replica can hold the lock at a time, so the trading automation (daily DCA, stop-loss,
// reconcile) runs as a guaranteed singleton even when the stateless HTTP API is scaled to many replicas
// behind a load balancer. Without it, every replica's worker would double-execute daily buys and
// stop-losses — strictly worse than one worker.
//
// The lock is held on a dedicated *sql.Conn for the lock's whole lifetime: a session-level advisory
// lock is released automatically when that connection closes (including if the leader process crashes),
// so a dead leader frees the lock and another replica can take over on its next attempt.
type LeaderLock struct {
	database *sql.DB
	lockKey  int64
	heldConn *sql.Conn
}

// NewLeaderLock builds a LeaderLock for a given advisory-lock key. The key is an arbitrary constant
// shared by all replicas of this service (a different service must use a different key).
func NewLeaderLock(database *sql.DB, lockKey int64) *LeaderLock {
	return &LeaderLock{database: database, lockKey: lockKey}
}

// TryAcquire attempts to become leader without blocking.
//   - (true, nil):  this replica is now the leader and holds the lock.
//   - (false, nil): another replica holds the lock; caller should stay passive and retry later.
//   - (true, err):  the lock mechanism itself errored. We FAIL OPEN to leader (true) so a lock-layer
//     fault never silently stops all trading — the current deployment is a single replica, and "no
//     worker at all" is a worse outcome than a (theoretical) double worker. The heartbeat watchdog and
//     /health/worker still cover an actually-stuck worker.
func (lock *LeaderLock) TryAcquire(acquireContext context.Context) (bool, error) {
	connection, connectionError := lock.database.Conn(acquireContext)
	if connectionError != nil {
		return true, connectionError
	}

	var acquired bool
	scanError := connection.QueryRowContext(acquireContext, `SELECT pg_try_advisory_lock($1)`, lock.lockKey).Scan(&acquired)
	if scanError != nil {
		_ = connection.Close()
		return true, scanError
	}
	if !acquired {
		// Someone else is leader — release this connection back to the pool and stay passive.
		_ = connection.Close()
		return false, nil
	}

	lock.heldConn = connection
	return true, nil
}

// Release unlocks the advisory lock and closes the dedicated connection, so another replica can take
// leadership. Safe to call when nothing is held.
func (lock *LeaderLock) Release(releaseContext context.Context) {
	if lock.heldConn == nil {
		return
	}
	if _, unlockError := lock.heldConn.ExecContext(releaseContext, `SELECT pg_advisory_unlock($1)`, lock.lockKey); unlockError != nil {
		log.Printf("leader lock: could not release advisory lock %d: %v", lock.lockKey, unlockError)
	}
	_ = lock.heldConn.Close()
	lock.heldConn = nil
}
