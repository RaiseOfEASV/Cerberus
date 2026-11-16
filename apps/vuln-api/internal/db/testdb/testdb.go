// Package testdb gives each test its own freshly migrated database, set up
// the way the postgres chart's initdb script sets up production (owner and
// app roles, PUBLIC revoked). VULN_TEST_DATABASE_URL must point at a
// superuser connection; run tests via `just test` (scripts/with-test-db.sh).
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/db"
)

const (
	ownerPassword = "owner-test"
	appPassword   = "app-test"
)

var (
	rolesOnce sync.Once
	rolesErr  error
	mu        sync.Mutex
	owners    = map[*pgxpool.Pool]*pgxpool.Pool{}
)

func superURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("VULN_TEST_DATABASE_URL")
	if u == "" {
		t.Fatal("VULN_TEST_DATABASE_URL is not set: run via `just test` (scripts/with-test-db.sh)")
	}
	return u
}

func withUser(t *testing.T, base, user, pass, dbname string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(user, pass)
	u.Path = "/" + dbname
	return u.String()
}

func fresh(t *testing.T) (app, owner *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	base := superURL(t)
	su, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer su.Close()
	rolesOnce.Do(func() {
		// `go test ./...` runs packages as parallel processes against the same
		// server; sync.Once only covers this process, so serialize the
		// cluster-wide CREATE ROLE with an advisory lock on one connection.
		conn, err := su.Acquire(ctx)
		if err != nil {
			rolesErr = err
			return
		}
		defer conn.Release()
		if _, rolesErr = conn.Exec(ctx, `SELECT pg_advisory_lock(424242)`); rolesErr != nil {
			return
		}
		defer func() { _, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock(424242)`) }()
		_, rolesErr = conn.Exec(ctx, `DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'vulnman_owner') THEN CREATE ROLE vulnman_owner LOGIN PASSWORD '`+ownerPassword+`'; END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'vulnman_app') THEN CREATE ROLE vulnman_app LOGIN PASSWORD '`+appPassword+`'; END IF;
		END $$`)
	})
	if rolesErr != nil {
		t.Fatal(rolesErr)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "t_" + hex.EncodeToString(b)
	for _, q := range []string{
		`CREATE DATABASE ` + name + ` OWNER vulnman_owner`,
		`REVOKE ALL ON DATABASE ` + name + ` FROM PUBLIC`,
		`GRANT CONNECT ON DATABASE ` + name + ` TO vulnman_app`,
	} {
		if _, err := su.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	suDB, err := pgxpool.New(ctx, withUser(t, base, "postgres", mustPass(t, base), name))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`ALTER SCHEMA public OWNER TO vulnman_owner`, `REVOKE CREATE ON SCHEMA public FROM PUBLIC`, `GRANT USAGE ON SCHEMA public TO vulnman_app`} {
		if _, err := suDB.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	suDB.Close()
	owner, err = db.Open(ctx, withUser(t, base, "vulnman_owner", ownerPassword, name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Migrate(ctx, owner); err != nil {
		t.Fatal(err)
	}
	app, err = db.Open(ctx, withUser(t, base, "vulnman_app", appPassword, name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		app.Close()
		owner.Close()
		mu.Lock()
		delete(owners, app)
		mu.Unlock()
		if su2, err := pgxpool.New(context.Background(), base); err == nil {
			_, _ = su2.Exec(context.Background(), `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`)
			su2.Close()
		}
	})
	mu.Lock()
	owners[app] = owner
	mu.Unlock()
	return app, owner
}

func mustPass(t *testing.T, base string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := u.User.Password()
	return p
}

// New returns an app-role pool on a fresh, migrated database.
func New(t *testing.T) *pgxpool.Pool {
	app, _ := fresh(t)
	return app
}

// Owner returns an owner-role pool on a fresh, migrated database.
func Owner(t *testing.T) *pgxpool.Pool {
	_, owner := fresh(t)
	return owner
}

// OwnerOf returns the owner-role pool for the database behind an app pool
// from New - for tests that tamper with data as a privileged user.
func OwnerOf(t *testing.T, app *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	o, ok := owners[app]
	if !ok {
		t.Fatal("pool not from testdb.New")
	}
	return o
}
