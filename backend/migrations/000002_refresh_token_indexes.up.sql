-- Indexes supporting the refresh-token lifecycle added in
-- feature/auth-refresh-logout. Lookups and deletions by token_hash are already
-- served by the token_hash UNIQUE constraint; these cover the remaining access
-- patterns.
--
-- IF NOT EXISTS keeps the migration safe to re-run by hand, since this project
-- has no migration tool tracking applied versions.

-- Listing/revoking every session for a user ("log out everywhere").
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id
    ON refresh_tokens (user_id);

-- Sweeping expired rows without a sequential scan.
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at
    ON refresh_tokens (expires_at);
