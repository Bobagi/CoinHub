BEGIN;

-- Liveness heartbeat for the AutomationWorker. The worker runs as a single goroutine pair inside one
-- API process; if it stalls or that process dies, robots silently stop trading (real money no longer
-- managed) and nothing notices. This single-row table is written by the worker on every monitor tick,
-- so any API replica can read last_tick_at to tell whether automation is alive and recent — it powers
-- the /health/worker probe, the in-app operational-status indicator, and the stalled-worker alert.
--
-- It is also the natural companion to the worker's leader lock (pg_advisory_lock): exactly one replica
-- holds leadership and writes the heartbeat, and every other replica reads this row for status, so the
-- liveness signal is correct even when the API is scaled to >1 replica.
CREATE TABLE IF NOT EXISTS worker_heartbeat (
    id           SMALLINT PRIMARY KEY DEFAULT 1,
    last_tick_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    instance_id  TEXT NOT NULL DEFAULT '',
    CONSTRAINT worker_heartbeat_singleton CHECK (id = 1)
);

-- Seed the single row so the worker only ever UPDATEs it.
INSERT INTO worker_heartbeat (id, last_tick_at) VALUES (1, NOW())
ON CONFLICT (id) DO NOTHING;

COMMIT;
