package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate applies every embedded migrations/*.sql not yet recorded in
// schema_migrations, in name order, one transaction each, under an advisory
// lock so two pods starting together can't race. Must run as the owner role.
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var applied []string
	for _, name := range names {
		did, err := applyOne(ctx, pool, name)
		if err != nil {
			return applied, fmt.Errorf("migration %s: %w", name, err)
		}
		if did {
			applied = append(applied, name)
		}
	}
	return applied, nil
}

func applyOne(ctx context.Context, pool *pgxpool.Pool, name string) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(4242)`); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, name).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	sql, err := migrations.ReadFile(name)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
