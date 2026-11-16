package domain

import (
	"errors"
	"testing"
	"time"
)

var roles = []Role{RoleAnalyst, RoleDeveloper, RolePM, RoleAuditor}

func TestCanMatrix(t *testing.T) {
	// Rows of the plan's permission matrix; developer "own assigned" cells
	// are true here (role level) - ownership is CanOnFinding's job.
	want := map[Action][4]bool{ // analyst, developer, pm, auditor
		ActView:         {true, true, true, true},
		ActCreate:       {true, false, true, false},
		ActImport:       {true, false, true, false},
		ActEdit:         {true, true, true, false},
		ActChangeStatus: {true, true, true, false},
		ActAssign:       {true, false, true, false},
		ActLink:         {true, true, true, false},
		ActAttach:       {true, true, true, false},
		ActExport:       {true, false, true, true},
		ActViewAudit:    {false, false, true, true},
		ActSetSLA:       {false, false, true, false},
		ActManageRoles:  {false, false, true, false},
	}
	for a, row := range want {
		for i, r := range roles {
			if got := Can(r, a); got != row[i] {
				t.Errorf("Can(%s, %s) = %v, want %v", r, a, got, row[i])
			}
		}
	}
	if Can(Role("admin"), ActView) {
		t.Error("unknown role must have no permissions")
	}
}

func TestDeveloperOwnership(t *testing.T) {
	for _, a := range []Action{ActEdit, ActChangeStatus, ActLink, ActAttach} {
		if CanOnFinding(RoleDeveloper, a, "u1", "u2") {
			t.Errorf("developer %s on someone else's finding allowed", a)
		}
		if !CanOnFinding(RoleDeveloper, a, "u1", "u1") {
			t.Errorf("developer %s on own finding denied", a)
		}
		if CanOnFinding(RoleDeveloper, a, "u1", "") {
			t.Errorf("developer %s on unassigned finding allowed", a)
		}
	}
	if !CanOnFinding(RoleAnalyst, ActEdit, "u1", "u2") {
		t.Error("analyst must edit any finding")
	}
	if CanOnFinding(RoleAuditor, ActEdit, "u1", "u1") {
		t.Error("auditor must never edit")
	}
	if !CanOnFinding(RoleDeveloper, ActView, "u1", "u2") {
		t.Error("developer views every finding")
	}
}

func TestTransitions(t *testing.T) {
	legal := map[[2]Status]bool{
		{StNew, StTriaged}:           true,
		{StTriaged, StInProgress}:    true,
		{StInProgress, StRemediated}: true,
		{StRemediated, StInProgress}: true,
		{StNew, StAccepted}:          true,
		{StTriaged, StAccepted}:      true,
		{StInProgress, StAccepted}:   true,
		{StAccepted, StTriaged}:      true,
	}
	devTargets := map[Status]bool{StInProgress: true, StRemediated: true}
	all := []Status{StNew, StTriaged, StInProgress, StRemediated, StAccepted}
	for _, from := range all {
		for _, to := range all {
			l := legal[[2]Status{from, to}]
			for _, r := range roles {
				want := l
				switch r {
				case RoleDeveloper:
					want = l && devTargets[to]
				case RoleAuditor:
					want = false
				}
				if got := CanTransition(r, from, to); got != want {
					t.Errorf("CanTransition(%s, %q -> %q) = %v, want %v", r, from, to, got, want)
				}
			}
		}
	}
	// Spot checks named in the plan.
	if CanTransition(RoleDeveloper, StInProgress, StAccepted) {
		t.Error("developer must not accept risk")
	}
	if CanTransition(RoleAnalyst, StNew, StRemediated) {
		t.Error("New -> Remediated must be illegal")
	}
}

func TestRoleFromClaims(t *testing.T) {
	cases := []struct {
		in   []string
		want Role
		err  error
	}{
		{[]string{"developer", "pm"}, RolePM, nil},
		{[]string{"auditor", "developer"}, RoleDeveloper, nil},
		{[]string{"offline_access", "analyst", "uma_authorization"}, RoleAnalyst, nil},
		{[]string{"auditor"}, RoleAuditor, nil},
		{[]string{"offline_access"}, "", ErrNoAppRole},
		{nil, "", ErrNoAppRole},
	}
	for _, c := range cases {
		got, err := RoleFromClaims(c.in)
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("RoleFromClaims(%v) = %q, %v; want %q, %v", c.in, got, err, c.want, c.err)
		}
	}
}

func TestParse(t *testing.T) {
	if s, err := ParseStatus("In Progress"); err != nil || s != StInProgress {
		t.Errorf("ParseStatus: %q %v", s, err)
	}
	if _, err := ParseStatus("in progress"); err == nil {
		t.Error("status parsing must be exact")
	}
	if s, err := ParseSeverity("Critical"); err != nil || s != SevCritical {
		t.Errorf("ParseSeverity: %q %v", s, err)
	}
	if _, err := ParseSeverity("Severe"); err == nil {
		t.Error("unknown severity accepted")
	}
	if r, err := ParseRole("pm"); err != nil || r != RolePM {
		t.Errorf("ParseRole: %q %v", r, err)
	}
	if _, err := ParseRole("admin"); err == nil {
		t.Error("unknown role accepted")
	}
}

func TestDueDate(t *testing.T) {
	triaged := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	got := DueDate(triaged, SevCritical, DefaultSLA)
	if want := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("DueDate = %v, want %v", got, want)
	}
	if DefaultSLA[SevHigh] != 30 || DefaultSLA[SevMedium] != 90 || DefaultSLA[SevLow] != 180 {
		t.Errorf("DefaultSLA = %v", DefaultSLA)
	}
}

func TestIsOpen(t *testing.T) {
	for s, want := range map[Status]bool{StNew: true, StTriaged: true, StInProgress: true, StRemediated: false, StAccepted: false} {
		if IsOpen(s) != want {
			t.Errorf("IsOpen(%q) != %v", s, want)
		}
	}
}
