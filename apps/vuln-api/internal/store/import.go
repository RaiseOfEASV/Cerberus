package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type ImportResult struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
}

// Import creates findings in one transaction: all or nothing. A finding
// whose fingerprint matches an open finding is skipped (D14). Each created
// finding gets its own finding.create audit row, plus one import.<source>
// summary row.
func (s *Store) Import(ctx context.Context, a Actor, nfs []NewFinding, source string) (ImportResult, error) {
	// Data keys first: a 2000-row import would otherwise make 2000 Vault
	// calls while holding the audit lock, blocking every other mutation.
	keys := make([]dataKey, len(nfs))
	for i := range nfs {
		k, err := s.newKey(ctx)
		if err != nil {
			return ImportResult{}, err
		}
		keys[i] = k
	}
	var res ImportResult
	err := s.mutate(ctx, func(tx pgx.Tx) error {
		res = ImportResult{}
		for i, nf := range nfs {
			if nf.Fingerprint != "" {
				var exists bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM findings WHERE fingerprint = $1
					AND status NOT IN ('Remediated','Accepted Risk'))`, nf.Fingerprint).Scan(&exists); err != nil {
					return err
				}
				if exists {
					res.Skipped++
					continue
				}
			}
			if _, err := s.createTx(ctx, tx, a, nf, map[string]any{"import": source}, keys[i]); err != nil {
				return err
			}
			res.Created++
		}
		return s.appendAudit(ctx, tx, a, "import."+source, nil, map[string]any{"created": res.Created, "skipped": res.Skipped})
	})
	return res, err
}

// RecordEvent appends an audit row for an action that changes no data but
// must be traceable (exports, report downloads).
func (s *Store) RecordEvent(ctx context.Context, a Actor, action string, details map[string]any) error {
	return s.mutate(ctx, func(tx pgx.Tx) error { return s.appendAudit(ctx, tx, a, action, nil, details) })
}
