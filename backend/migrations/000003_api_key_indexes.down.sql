-- Reverses 000003_api_key_indexes.up.sql. Drops only the index this migration
-- created; the api_keys table itself belongs to 000001.
DROP INDEX IF EXISTS idx_api_keys_user_id;
