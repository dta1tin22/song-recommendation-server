-- Reverses 000002_refresh_token_indexes.up.sql. Drops only the indexes this
-- migration created; the refresh_tokens table itself belongs to 000001.
DROP INDEX IF EXISTS idx_refresh_tokens_expires_at;
DROP INDEX IF EXISTS idx_refresh_tokens_user_id;
