package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/db"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/db/testdb"
)

func TestMigrateIdempotent(t *testing.T) {
	owner := testdb.Owner(t)
	applied, err := db.Migrate(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Fatalf("second migrate applied %v, want none", applied)
	}
}

func TestAppRoleCannotUpdateAudit(t *testing.T) {
	app := testdb.New(t)
	ctx := context.Background()
	if _, err := app.Exec(ctx, `INSERT INTO audit_log (at, actor_sub, actor_role, action, details, hash) VALUES (now(), 's', 'pm', 'x', '{}', '\x00')`); err != nil {
		t.Fatalf("app role must be able to insert: %v", err)
	}
	_, err := app.Exec(ctx, `UPDATE audit_log SET action = 'y'`)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("want permission denied, got %v", err)
	}
	_, err = app.Exec(ctx, `DELETE FROM audit_log`)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("want permission denied on delete, got %v", err)
	}
}

func TestAuditTriggerBlocksOwner(t *testing.T) {
	app := testdb.New(t)
	owner := testdb.OwnerOf(t, app)
	ctx := context.Background()
	if _, err := app.Exec(ctx, `INSERT INTO audit_log (at, actor_sub, actor_role, action, details, hash) VALUES (now(), 's', 'pm', 'x', '{}', '\x00')`); err != nil {
		t.Fatal(err)
	}
	_, err := owner.Exec(ctx, `DELETE FROM audit_log`)
	if err == nil || !strings.Contains(err.Error(), "audit_log is append-only") {
		t.Fatalf("want trigger error, got %v", err)
	}
}

func TestAppRoleHasNoDDL(t *testing.T) {
	app := testdb.New(t)
	_, err := app.Exec(context.Background(), `CREATE TABLE evil (x int)`)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("want permission denied, got %v", err)
	}
}
