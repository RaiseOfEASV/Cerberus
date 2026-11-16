// Package store is VulnMan's persistence: findings with encrypted fields,
// links, attachments, SLA targets and the hash-chained audit log. Every
// mutation and its audit row commit in one transaction.
package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/crypt"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrInvalid           = errors.New("invalid")
	ErrIllegalTransition = errors.New("illegal status transition")
	// ErrDecrypt: a stored ciphertext failed to decrypt (tampering, moved
	// data, or Vault refusing). Never returned with partial plaintext.
	ErrDecrypt = errors.New("decryption failed")
)

// Actor is who performs a change; recorded in the audit log.
type Actor struct {
	Sub, Name string
	Role      domain.Role
}

type Store struct {
	pool    *pgxpool.Pool
	transit crypt.Transit
	now     func() time.Time
}

func New(pool *pgxpool.Pool, t crypt.Transit, now func() time.Time) *Store {
	return &Store{pool: pool, transit: t, now: now}
}

// Pool exposes the connection pool (tests and health checks).
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ts returns the current time at Postgres precision (microseconds, UTC),
// so values hashed into the audit chain survive the round-trip.
func (s *Store) ts() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

// auditLockKey serializes writers of the audit chain. A transaction-scoped
// advisory lock rather than LOCK TABLE: the app role has no UPDATE/DELETE
// on audit_log, which LOCK TABLE's exclusive modes would require.
const auditLockKey = 7001

// mutate runs fn and its audit append in one transaction holding the chain lock.
func (s *Store) mutate(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, auditLockKey); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewRef returns "VM-" + 10 random Crockford base32 characters.
func NewRef() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	out := []byte("VM-")
	for _, c := range b {
		out = append(out, crockford[int(c)%32])
	}
	return string(out)
}

func isUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}
