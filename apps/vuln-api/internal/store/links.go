package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type LinkKind string

const (
	LinkDuplicate LinkKind = "duplicate"
	LinkRelated   LinkKind = "related"
	LinkRootCause LinkKind = "same_root_cause"
)

func ParseLinkKind(s string) (LinkKind, error) {
	switch k := LinkKind(s); k {
	case LinkDuplicate, LinkRelated, LinkRootCause:
		return k, nil
	}
	return "", invalid("unknown link kind %q", s)
}

// Link is a relation seen from one finding: the other side and the kind.
type Link struct {
	FindingID int64    `json:"findingId"`
	Ref       string   `json:"ref"`
	Title     string   `json:"title"`
	Kind      LinkKind `json:"kind"`
}

func order(x, y int64) (int64, int64) {
	if x < y {
		return x, y
	}
	return y, x
}

// Link relates two findings (undirected; stored once with a < b).
func (s *Store) Link(ctx context.Context, a Actor, x, y int64, k LinkKind) error {
	if x == y {
		return invalid("a finding cannot be linked to itself")
	}
	if _, err := ParseLinkKind(string(k)); err != nil {
		return err
	}
	lo, hi := order(x, y)
	return s.mutate(ctx, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM findings WHERE id IN ($1,$2)`, lo, hi).Scan(&n); err != nil {
			return err
		}
		if n != 2 {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `INSERT INTO finding_links (a, b, kind) VALUES ($1,$2,$3)`, lo, hi, string(k)); err != nil {
			if isUniqueViolation(err) {
				return ErrConflict
			}
			return err
		}
		details := map[string]any{"other": y, "kind": k}
		if err := s.appendAudit(ctx, tx, a, "finding.link", &x, details); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, a, "finding.link", &y, map[string]any{"other": x, "kind": k})
	})
}

func (s *Store) Unlink(ctx context.Context, a Actor, x, y int64) error {
	lo, hi := order(x, y)
	return s.mutate(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM finding_links WHERE a=$1 AND b=$2`, lo, hi)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if err := s.appendAudit(ctx, tx, a, "finding.unlink", &x, map[string]any{"other": y}); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, a, "finding.unlink", &y, map[string]any{"other": x})
	})
}

func (s *Store) Links(ctx context.Context, id int64) ([]Link, error) {
	rows, err := s.pool.Query(ctx, `SELECT f.id, f.ref, f.title, l.kind FROM finding_links l
		JOIN findings f ON f.id = CASE WHEN l.a = $1 THEN l.b ELSE l.a END
		WHERE l.a = $1 OR l.b = $1 ORDER BY f.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Link{}
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.FindingID, &l.Ref, &l.Title, &l.Kind); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
