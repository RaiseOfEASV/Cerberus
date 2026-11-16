package store_test

import (
	"testing"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/auth"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

var alice = auth.User{Sub: "alice", Name: "Alice", Email: "alice@example.com", Role: domain.RoleAnalyst}

func TestSessionCreateTouch(t *testing.T) {
	e := newEnv(t)
	ss := e.s.Sessions()
	raw, err := ss.Create(ctx, alice, 8*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 43 {
		t.Fatalf("session id too short: %q", raw)
	}
	var stored []byte
	_ = e.s.Pool().QueryRow(ctx, `SELECT id_hash FROM sessions`).Scan(&stored)
	if string(stored) == raw || len(stored) != 32 {
		t.Fatal("session id must be stored only as its SHA-256")
	}
	u, ok, err := ss.Touch(ctx, raw, 30*time.Minute)
	if err != nil || !ok || u != alice {
		t.Fatalf("Touch = %+v %v %v", u, ok, err)
	}
	if _, ok, _ := ss.Touch(ctx, "not-a-session", 30*time.Minute); ok {
		t.Fatal("unknown id accepted")
	}
}

func TestIdleExpiry(t *testing.T) {
	e := newEnv(t)
	ss := e.s.Sessions()
	raw, _ := ss.Create(ctx, alice, 8*time.Hour)
	*e.now = t0.Add(29 * time.Minute)
	if _, ok, _ := ss.Touch(ctx, raw, 30*time.Minute); !ok {
		t.Fatal("active session rejected")
	}
	*e.now = t0.Add(29*time.Minute + 31*time.Minute) // idle 31m since last touch
	if _, ok, _ := ss.Touch(ctx, raw, 30*time.Minute); ok {
		t.Fatal("session idle for 31 minutes still valid")
	}
}

func TestAbsoluteExpiry(t *testing.T) {
	e := newEnv(t)
	ss := e.s.Sessions()
	raw, _ := ss.Create(ctx, alice, 8*time.Hour)
	for m := 20; m <= 8*60+20; m += 20 { // keep touching every 20 minutes
		*e.now = t0.Add(time.Duration(m) * time.Minute)
		_, ok, _ := ss.Touch(ctx, raw, 30*time.Minute)
		if m < 8*60 && !ok {
			t.Fatalf("rejected at %d minutes", m)
		}
		if m > 8*60 && ok {
			t.Fatalf("still valid after the 8h absolute lifetime (%d min)", m)
		}
	}
}

func TestDeleteBySubKillsSessions(t *testing.T) {
	e := newEnv(t)
	ss := e.s.Sessions()
	r1, _ := ss.Create(ctx, alice, time.Hour)
	r2, _ := ss.Create(ctx, alice, time.Hour)
	bob := alice
	bob.Sub = "bob"
	r3, _ := ss.Create(ctx, bob, time.Hour)
	if err := ss.DeleteBySub(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	for _, r := range []string{r1, r2} {
		if _, ok, _ := ss.Touch(ctx, r, time.Hour); ok {
			t.Fatal("alice's session survived DeleteBySub")
		}
	}
	if _, ok, _ := ss.Touch(ctx, r3, time.Hour); !ok {
		t.Fatal("bob's session was deleted too")
	}
	if err := ss.Delete(ctx, r3); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := ss.Touch(ctx, r3, time.Hour); ok {
		t.Fatal("deleted session still valid")
	}
}

func TestPurgeExpiredSessions(t *testing.T) {
	e := newEnv(t)
	ss := e.s.Sessions()
	_, _ = ss.Create(ctx, alice, 8*time.Hour) // never touched again: idle at t0+35m
	live, _ := ss.Create(ctx, alice, 8*time.Hour)
	*e.now = t0.Add(10 * time.Minute)
	if _, ok, _ := ss.Touch(ctx, live, 30*time.Minute); !ok {
		t.Fatal("setup: touch failed")
	}
	*e.now = t0.Add(35 * time.Minute)
	n, err := ss.Purge(ctx, 30*time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("purged %d, %v; want 1 (only the idle one)", n, err)
	}
	if _, ok, _ := ss.Touch(ctx, live, 30*time.Minute); !ok {
		t.Fatal("purge removed a live session")
	}
}
