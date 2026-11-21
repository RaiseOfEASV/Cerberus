package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

// ScanResult is what one nightly scan of one asset changed.
type ScanResult struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
	Closed  int `json:"closed"`
}

// ScanClosedNote is the status note on findings a scan no longer reports.
const ScanClosedNote = "No longer found by the nightly Trivy scan."

// SyncScan makes the open Trivy findings for one asset match a fresh scan,
// in one transaction:
//   - every reported vulnerability without an open finding is created
//     (an open one with the same fingerprint is left alone, like Import);
//   - every open Trivy finding for the asset that the scan no longer reports
//     is set to Remediated.
//
// Closing skips the D6 state machine on purpose: the scanner is the source
// of truth for "is this still in the image", and a finding still in New or
// Triaged can be fixed by an upgrade nobody tracked. Each closure gets its
// own finding.status audit row (auto: true) so people can see who did it.
// Accepted Risk findings are never touched, and a vulnerability that comes
// back after being closed gets a new finding.
func (s *Store) SyncScan(ctx context.Context, a Actor, asset string, nfs []NewFinding) (ScanResult, error) {
	keys := make([]dataKey, len(nfs))
	for i := range nfs {
		k, err := s.newKey(ctx)
		if err != nil {
			return ScanResult{}, err
		}
		keys[i] = k
	}
	seen := make([]string, 0, len(nfs))
	for _, nf := range nfs {
		if nf.Fingerprint != "" {
			seen = append(seen, nf.Fingerprint)
		}
	}
	var res ScanResult
	err := s.mutate(ctx, func(tx pgx.Tx) error {
		res = ScanResult{}
		for i, nf := range nfs {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM findings WHERE fingerprint = $1
				AND status NOT IN ('Remediated','Accepted Risk'))`, nf.Fingerprint).Scan(&exists); err != nil {
				return err
			}
			if exists {
				res.Skipped++
				continue
			}
			if _, err := s.createTx(ctx, tx, a, nf, map[string]any{"import": "trivy-scan"}, keys[i]); err != nil {
				return err
			}
			res.Created++
		}

		rows, err := tx.Query(ctx, `SELECT id, status FROM findings
			WHERE source = 'trivy' AND asset = $1 AND fingerprint IS NOT NULL
			AND status NOT IN ('Remediated','Accepted Risk')
			AND NOT (fingerprint = ANY($2))
			ORDER BY id FOR UPDATE`, asset, seen)
		if err != nil {
			return err
		}
		type gone struct {
			id   int64
			from string
		}
		var closing []gone
		for rows.Next() {
			var g gone
			if err := rows.Scan(&g.id, &g.from); err != nil {
				rows.Close()
				return err
			}
			closing = append(closing, g)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		now := s.ts()
		for _, g := range closing {
			if _, err := tx.Exec(ctx, `UPDATE findings SET status = $1, updated_at = $2 WHERE id = $3`,
				string(domain.StRemediated), now, g.id); err != nil {
				return err
			}
			id := g.id
			if err := s.appendAudit(ctx, tx, a, "finding.status", &id, map[string]any{
				"from": g.from, "to": domain.StRemediated, "note": ScanClosedNote, "auto": true,
			}); err != nil {
				return err
			}
			res.Closed++
		}
		return s.appendAudit(ctx, tx, a, "scan.trivy", nil, map[string]any{
			"asset": asset, "created": res.Created, "skipped": res.Skipped, "closed": res.Closed,
		})
	})
	return res, err
}
