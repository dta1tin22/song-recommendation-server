-- Index supporting the API-key lifecycle added in feature/api-keys.
-- Lookups by key_hash are already served by that column's UNIQUE constraint;
-- this covers listing every key for one user and revoking a specific key with
-- the user_id ownership filter applied.
--
-- IF NOT EXISTS keeps the migration safe to re-run by hand, since this project
-- has no migration tool tracking applied versions.

CREATE INDEX IF NOT EXISTS idx_api_keys_user_id
    ON api_keys (user_id);
