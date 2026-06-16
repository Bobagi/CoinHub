-- Step-up ("sudo") re-authentication: records when a session most recently re-proved identity
-- (via password or a fresh Google re-confirm). Sensitive actions require this to be recent.
ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS step_up_verified_at TIMESTAMPTZ;
