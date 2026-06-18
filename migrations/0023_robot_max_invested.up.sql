-- A per-robot maximum total invested (open-allocation) ceiling for its coin. The daily DCA buy is
-- skipped while open positions for that coin already hold this much (cost basis), so the robot waits
-- for a take-profit/stop-loss to free capital before buying again. 0 = no cap (default).
ALTER TABLE trading_robots ADD COLUMN IF NOT EXISTS max_invested DOUBLE PRECISION NOT NULL DEFAULT 0;
