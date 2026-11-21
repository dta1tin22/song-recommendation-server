package apikey

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// apiKeyColumns is the single source of truth for the column order that
// scanAPIKey expects. Every read path shares it so a new column cannot silently
// desync the SELECT list from the Scan targets.
const apiKeyColumns = `id, user_id, name, key_prefix, key_hash, scopes,
		       expires_at, revoked_at, last_used_at, created_at`

const insertAPIKeyQuery = `
	INSERT INTO api_keys (user_id, name, key_prefix, key_hash, scopes, expires_at)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id, created_at
`

const listAPIKeysQuery = `
	SELECT ` + apiKeyColumns + `
	FROM api_keys
	WHERE user_id = $1 AND revoked_at IS NULL
	ORDER BY created_at DESC, id DESC
`

// revokeAPIKeyQuery is the only write path for revocation and is scoped by
// user_id. Filtering on revoked_at IS NULL makes the operation idempotent from
// the database's point of view while still letting the service report a
// not-found for a second attempt.
const revokeAPIKeyQuery = `
	UPDATE api_keys
	SET revoked_at = now()
	WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
`

// getAPIKeyByHashQuery is the authentication lookup. key_hash is UNIQUE, so this
// is a single index probe; revoked and expired rows are deliberately returned
// too, because usability is service policy rather than a query predicate.
const getAPIKeyByHashQuery = `
	SELECT ` + apiKeyColumns + `
	FROM api_keys
	WHERE key_hash = $1
`

// touchAPIKeyLastUsedQuery writes the usage stamp at most once per throttle
// window. The interval is supplied by the service as a parameter and cast
// server-side, keeping the policy out of the repository. Zero rows affected is
// the normal throttled case and therefore not an error.
const touchAPIKeyLastUsedQuery = `
	UPDATE api_keys
	SET last_used_at = now()
	WHERE id = $1
	  AND (last_used_at IS NULL OR last_used_at < now() - $2::interval)
`

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Create stores a new API key and populates the generated columns (id,
// created_at) back onto the supplied model. Only the SHA-256 hash is persisted;
// the raw key exists solely in the service's return value.
func (r *PostgresRepository) Create(ctx context.Context, key *Key) error {
	err := r.pool.QueryRow(ctx, insertAPIKeyQuery,
		key.UserID,
		key.Name,
		key.KeyPrefix,
		key.KeyHash,
		key.Scopes,
		key.ExpiresAt,
	).Scan(&key.ID, &key.CreatedAt)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}

	return nil
}

// ListByUser returns the user's active keys. Revoked rows are kept for audit but
// are never listed, and the user_id predicate is what keeps one account's keys
// out of another account's response.
func (r *PostgresRepository) ListByUser(ctx context.Context, userID int64) ([]Key, error) {
	rows, err := r.pool.Query(ctx, listAPIKeysQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	// A non-nil slice keeps the JSON response an array ("[]") rather than null
	// when the user has no keys yet.
	keys := make([]Key, 0)
	for rows.Next() {
		key, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}

	return keys, nil
}

// Revoke soft-deletes a key by stamping revoked_at. Zero rows affected means the
// key does not exist, belongs to another user, or was already revoked; all three
// collapse into ErrAPIKeyNotFound so the handler cannot leak existence.
func (r *PostgresRepository) Revoke(ctx context.Context, userID, keyID int64) error {
	tag, err := r.pool.Exec(ctx, revokeAPIKeyQuery, keyID, userID)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}

	return nil
}

// GetByHash returns the key matching the supplied SHA-256 digest. The caller
// passes a digest, never the raw credential: nothing in this layer can observe
// the secret. Revoked and expired rows are returned as-is so the service owns
// the decision, and a missing row becomes ErrAPIKeyNotFound.
func (r *PostgresRepository) GetByHash(ctx context.Context, hash string) (*Key, error) {
	key, err := scanAPIKey(r.pool.QueryRow(ctx, getAPIKeyByHashQuery, hash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}

	return key, nil
}

// TouchLastUsed stamps last_used_at unless the previous stamp is inside the
// throttle window. No rows affected means "recently stamped", which is success:
// the caller only records telemetry and must not treat it as a failure.
func (r *PostgresRepository) TouchLastUsed(ctx context.Context, keyID int64, throttle time.Duration) error {
	if throttle <= 0 {
		return fmt.Errorf("touch api key last used: throttle must be positive, got %s", throttle)
	}

	// pgx encodes a time.Duration as a PostgreSQL interval, so the policy value
	// travels as a parameter instead of being baked into the statement.
	if _, err := r.pool.Exec(ctx, touchAPIKeyLastUsedQuery, keyID, throttle); err != nil {
		return fmt.Errorf("touch api key last used: %w", err)
	}

	return nil
}

// scanAPIKey scans one row selected in apiKeyColumns order. pgx.Rows satisfies
// pgx.Row, so the same helper serves both the list loop and any future
// single-row read.
func scanAPIKey(row pgx.Row) (*Key, error) {
	var key Key
	err := row.Scan(
		&key.ID,
		&key.UserID,
		&key.Name,
		&key.KeyPrefix,
		&key.KeyHash,
		&key.Scopes,
		&key.ExpiresAt,
		&key.RevokedAt,
		&key.LastUsedAt,
		&key.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan api key: %w", err)
	}

	return &key, nil
}
