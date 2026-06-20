BEGIN;

-- Google profile picture URL (lh*.googleusercontent.com), refreshed on each Google sign-in. Used to
-- show the user's avatar in the header; the image is proxied same-origin (/api/v1/account/avatar) so
-- it loads under the strict CSP (img-src 'self'). NULL for password-only accounts.
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT;

COMMIT;
