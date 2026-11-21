package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// insertRefreshTokenQuery is shared by Create and Rotate so the column list and
// the Scan targets can never drift apart.
const insertRefreshTokenQuery = `
	INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent, ip_address)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING id, created_at
`

const selectRefreshTokenByHashQuery = `
	SELECT id, user_id, token_hash, expires_at, user_agent, ip_address, created_at
	FROM refresh_tokens
	WHERE token_hash = $1
`

type PostgresRefreshTokenRepository struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) *PostgresRefreshTokenRepository {
	return &PostgresRefreshTokenRepository{pool: pool}
}

// Create stores a new refresh-token session and populates the generated columns
// (id, created_at) back onto the supplied model. Only the SHA-256 hash is
// persisted; the raw token exists solely in the service's return value.
func (r *PostgresRefreshTokenRepository) Create(ctx context.Context, token *RefreshToken) error {
	if err := insertRefreshToken(ctx, r.pool, token); err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}

	return nil
}

// GetByHash returns the session matching the supplied hash. Expiry and
// revocation are policy decisions and are deliberately left to the service; this
// method reports only whether a row exists.
func (r *PostgresRefreshTokenRepository) GetByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	var token RefreshToken
	err := r.pool.QueryRow(ctx, selectRefreshTokenByHashQuery, hash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.UserAgent,
		&token.IPAddress,
		&token.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRefreshTokenNotFound
		}
		return nil, fmt.Errorf("get refresh token by hash: %w", err)
	}

	return &token, nil
}

// DeleteByHash removes the session matching the supplied hash. Deleting zero
// rows is not an error: logout must be idempotent and must not reveal whether the
// token ever existed.
func (r *PostgresRefreshTokenRepository) DeleteByHash(ctx context.Context, hash string) error {
	const query = `DELETE FROM refresh_tokens WHERE token_hash = $1`

	if _, err := r.pool.Exec(ctx, query, hash); err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}

	return nil
}

// Rotate consumes oldHash and persists newToken in one transaction. The DELETE
// doubles as the concurrency guard: because it only matches a row that is still
// unexpired, exactly one caller can ever consume a given refresh token. A second,
// concurrent (or replayed) request deletes nothing and receives
// ErrRefreshTokenNotFound, which the service reports as an opaque invalid
// token.
func (r *PostgresRefreshTokenRepository) Rotate(ctx context.Context, oldHash string, newToken *RefreshToken) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("rotate refresh token: begin: %w", err)
	}
	// Rollback is a no-op once the transaction has been committed.
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const deleteQuery = `
		DELETE FROM refresh_tokens
		WHERE token_hash = $1 AND expires_at > now()
		RETURNING user_id
	`

	var ownerID int64
	if err := tx.QueryRow(ctx, deleteQuery, oldHash).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRefreshTokenNotFound
		}
		return fmt.Errorf("rotate refresh token: consume old: %w", err)
	}

	// Guard against a caller wiring the wrong user onto a rotation. The service
	// always derives both sides from the same row, so this is a programming
	// error rather than a client error: keep it loud and roll back.
	if ownerID != newToken.UserID {
		return fmt.Errorf("rotate refresh token: owner mismatch (old token user %d, new token user %d)", ownerID, newToken.UserID)
	}

	if err := insertRefreshToken(ctx, tx, newToken); err != nil {
		return fmt.Errorf("rotate refresh token: insert new: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("rotate refresh token: commit: %w", err)
	}

	return nil
}

// insertRefreshToken runs the shared INSERT on either the pool or an open
// transaction, so Create and Rotate cannot diverge.
func insertRefreshToken(ctx context.Context, q queryRower, token *RefreshToken) error {
	return q.QueryRow(ctx, insertRefreshTokenQuery,
		token.UserID,
		token.TokenHash,
		token.ExpiresAt,
		token.UserAgent,
		token.IPAddress,
	).Scan(&token.ID, &token.CreatedAt)
}

// queryRower is the slice of pgx shared by *pgxpool.Pool and pgx.Tx.
type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
