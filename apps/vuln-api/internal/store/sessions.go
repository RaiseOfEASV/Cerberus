package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/auth"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

// Sessions is the Postgres auth.SessionStore. Only SHA-256(id) is stored,
// so reading the table doesn't yield usable session cookies.
type Sessions struct{ s *Store }

func (s *Store) Sessions() *Sessions { return &Sessions{s: s} }

var _ auth.SessionStore = (*Sessions)(nil)

func (ss *Sessions) Create(ctx context.Context, u auth.User, absTTL time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	now := ss.s.ts()
	_, err := ss.s.pool.Exec(ctx, `INSERT INTO sessions (id_hash, sub, name, email, role, created_at, last_seen_at, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$6,$7)`, auth.HashID(raw), u.Sub, u.Name, u.Email, string(u.Role), now, now.Add(absTTL))
	if err != nil {
		return "", err
	}
	return raw, nil
}

// Touch returns the session's user if it exists, hasn't passed its
// absolute expiry and was used within idleTTL; it then records the use.
func (ss *Sessions) Touch(ctx context.Context, raw string, idleTTL time.Duration) (auth.User, bool, error) {
	now := ss.s.ts()
	var u auth.User
	var role string
	err := ss.s.pool.QueryRow(ctx, `UPDATE sessions SET last_seen_at = $2
		WHERE id_hash = $1 AND expires_at > $2 AND last_seen_at > $3
		RETURNING sub, name, email, role`, auth.HashID(raw), now, now.Add(-idleTTL)).Scan(&u.Sub, &u.Name, &u.Email, &role)
	if err == pgx.ErrNoRows {
		return auth.User{}, false, nil
	}
	if err != nil {
		return auth.User{}, false, err
	}
	u.Role = domain.Role(role)
	return u, true, nil
}

func (ss *Sessions) Delete(ctx context.Context, raw string) error {
	_, err := ss.s.pool.Exec(ctx, `DELETE FROM sessions WHERE id_hash = $1`, auth.HashID(raw))
	return err
}

func (ss *Sessions) DeleteBySub(ctx context.Context, sub string) error {
	_, err := ss.s.pool.Exec(ctx, `DELETE FROM sessions WHERE sub = $1`, sub)
	return err
}

// RecordLogin appends an auth.login audit row.
func (s *Store) RecordLogin(ctx context.Context, u auth.User) error {
	return s.mutate(ctx, func(tx pgx.Tx) error {
		return s.appendAudit(ctx, tx, Actor{Sub: u.Sub, Name: u.Name, Role: u.Role}, "auth.login", nil, map[string]any{"name": u.Name})
	})
}
