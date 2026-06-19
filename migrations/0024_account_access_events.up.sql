BEGIN;

-- Durable, append-only log of successful account sign-ins ("access history"). Unlike user_sessions
-- (which are purged on expiry), these rows survive so the user can review past accesses in settings
-- and so we can tell a "new" device/IP from a known one to send a security alert email.
--
-- device_fingerprint = SHA-256(user_agent + '|' + ip_address): a stable id for a device/network
-- combination. is_new_device records whether that fingerprint had never been seen for this user when
-- the access happened. IP + user_agent are PII; the FK cascade erases them when the account is deleted.
CREATE TABLE IF NOT EXISTS account_access_events (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ip_address VARCHAR(64),
    user_agent TEXT,
    auth_method VARCHAR(20) NOT NULL,
    device_fingerprint CHAR(64) NOT NULL,
    is_new_device BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Listing is "this user's accesses, newest first"; new-device detection looks up by fingerprint.
CREATE INDEX IF NOT EXISTS account_access_events_user_created_idx ON account_access_events (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS account_access_events_user_fp_idx ON account_access_events (user_id, device_fingerprint);

COMMIT;
