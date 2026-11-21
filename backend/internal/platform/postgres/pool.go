package postgres

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool parses dbUrl, opens a pgx pool with the API's connection limits and
// verifies it with a ping. Any failure is fatal: the process cannot serve
// requests without a database.
func NewPool(ctx context.Context, dbUrl string) *pgxpool.Pool {
	config, err := pgxpool.ParseConfig(dbUrl)
	if err != nil {
		log.Fatalf("Unable to parse DB URL: %v\n", err)
	}

	config.MaxConns = 10
	config.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatalf("Unable to create connection pool: %v\n", err)
	}

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Database ping failed: %v\n", err)
	}

	log.Println("Successfully connected to PostgreSQL")
	return pool
}
