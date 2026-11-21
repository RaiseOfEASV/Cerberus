package store_test

import (
	"context"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/crypt"
)

// lockSpyTransit records how many DataKey calls happened while the audit
// chain's advisory lock (7001) was held - asked of Postgres itself.
type lockSpyTransit struct {
	crypt.Transit
	pool   *pgxpool.Pool
	during atomic.Int32
}

func (s *lockSpyTransit) DataKey(ctx context.Context) ([]byte, string, error) {
	var held int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND objid = 7001 AND granted`).Scan(&held)
	if held > 0 {
		s.during.Add(1)
	}
	return s.Transit.DataKey(ctx)
}
