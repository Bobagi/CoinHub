BEGIN;

-- Durable, append-only record of each time a user accepts the Terms of Use + Privacy Policy. A
-- consent record is the legal proof that the user agreed, so it must live on the server (a front-end
-- checkbox alone is neither enforceable nor auditable). One row per acceptance: who, which document
-- version, when, and from where (IP + user agent), so a later dispute can be answered with evidence.
--
-- document_version is the version tag in force at acceptance time (domain.CurrentAgreementVersion).
-- Bumping that tag (when the legal text materially changes) makes prior rows no longer match the
-- current version, so the user must re-accept — and the history of every version they ever accepted
-- is preserved here. IP + user_agent are PII; the FK cascade erases them when the account is deleted
-- (consistent with the privacy-preserving hard delete).
CREATE TABLE IF NOT EXISTS user_agreement_acceptances (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document_version VARCHAR(40) NOT NULL,
    ip_address VARCHAR(64),
    user_agent TEXT,
    accepted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- "Has this user accepted version X?" is the hot lookup (gate check on every protected request).
CREATE INDEX IF NOT EXISTS user_agreement_acceptances_user_version_idx
    ON user_agreement_acceptances (user_id, document_version);

COMMIT;
