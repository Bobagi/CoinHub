-- Irreversible data cleanup: the duplicate BUY rows carried no information not already present in the
-- surviving DAILY_BUY rows (same order id, price, quantity and total), so there is nothing to restore.
-- This down migration is intentionally a no-op.
SELECT 1;
