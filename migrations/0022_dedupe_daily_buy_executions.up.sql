-- The daily DCA used to log two history rows per purchase: a plain BUY (from the shared buy path) and
-- a DAILY_BUY marker, both sharing the same Binance order id. The buy path now records a single
-- DAILY_BUY for daily purchases, so drop the legacy duplicate BUY rows: a BUY whose order_id, user and
-- environment match an existing DAILY_BUY row (bot-initiated). Manual user BUYs have no DAILY_BUY twin
-- and are left untouched.
DELETE FROM trading_operation_executions AS buy
WHERE buy.operation_type = 'BUY'
  AND buy.order_id IS NOT NULL
  AND EXISTS (
    SELECT 1
    FROM trading_operation_executions AS daily
    WHERE daily.operation_type = 'DAILY_BUY'
      AND daily.order_id = buy.order_id
      AND daily.user_id IS NOT DISTINCT FROM buy.user_id
      AND daily.binance_environment = buy.binance_environment
  );
