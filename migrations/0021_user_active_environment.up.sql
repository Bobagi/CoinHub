-- The user's active Binance environment is now an explicit preference, independent of whether keys
-- exist for it — so a user can switch TO production (and see it marked "not connected") before saving
-- keys. Previously "active" was inferred from binance_credentials.is_active, which required keys.
ALTER TABLE users ADD COLUMN active_binance_environment TEXT NOT NULL DEFAULT 'TESTNET'
  CHECK (active_binance_environment IN ('TESTNET', 'PRODUCTION'));

-- Seed existing users from their currently-active credential so nothing changes for them.
UPDATE users u
SET active_binance_environment = c.environment
FROM binance_credentials c
WHERE c.user_id = u.id AND c.is_active = true
  AND c.environment IN ('TESTNET', 'PRODUCTION');
