package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func readSLA(ctx context.Context, q querier) (domain.SLA, error) {
	rows, err := q.Query(ctx, `SELECT severity, days FROM sla_targets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sla := domain.SLA{}
	for rows.Next() {
		var sev string
		var days int
		if err := rows.Scan(&sev, &days); err != nil {
			return nil, err
		}
		sla[domain.Severity(sev)] = days
	}
	return sla, rows.Err()
}

func slaTx(ctx context.Context, tx pgx.Tx) (domain.SLA, error) { return readSLA(ctx, tx) }

func (s *Store) SLA(ctx context.Context) (domain.SLA, error) { return readSLA(ctx, s.pool) }

// SetSLA replaces all four targets (1-365 days each).
func (s *Store) SetSLA(ctx context.Context, a Actor, sla domain.SLA) error {
	if len(sla) != len(domain.Severities) {
		return invalid("all four severities are required")
	}
	for _, sev := range domain.Severities {
		d, ok := sla[sev]
		if !ok || d < 1 || d > 365 {
			return invalid("%s: days must be 1-365", sev)
		}
	}
	return s.mutate(ctx, func(tx pgx.Tx) error {
		old, err := slaTx(ctx, tx)
		if err != nil {
			return err
		}
		for _, sev := range domain.Severities {
			if _, err := tx.Exec(ctx, `UPDATE sla_targets SET days=$1 WHERE severity=$2`, sla[sev], string(sev)); err != nil {
				return err
			}
		}
		return s.appendAudit(ctx, tx, a, "sla.set", nil, map[string]any{"old": old, "new": sla})
	})
}
