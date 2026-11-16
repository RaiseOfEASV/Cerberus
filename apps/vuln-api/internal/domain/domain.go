// Package domain holds VulnMan's business rules: roles, severities,
// statuses, the permission matrix and the status state machine. It is pure
// (no I/O) so every rule is table-tested; the API enforces these on the
// server, and the web UI only mirrors them for display.
package domain

import (
	"errors"
	"fmt"
	"time"
)

type Role string

const (
	RoleAnalyst   Role = "analyst"
	RoleDeveloper Role = "developer"
	RolePM        Role = "pm"
	RoleAuditor   Role = "auditor"
)

// Roles is every app role, in priority order (highest first) - D4.
var Roles = []Role{RolePM, RoleAnalyst, RoleDeveloper, RoleAuditor}

type Severity string

const (
	SevCritical Severity = "Critical"
	SevHigh     Severity = "High"
	SevMedium   Severity = "Medium"
	SevLow      Severity = "Low"
)

var Severities = []Severity{SevCritical, SevHigh, SevMedium, SevLow}

type Status string

const (
	StNew        Status = "New"
	StTriaged    Status = "Triaged"
	StInProgress Status = "In Progress"
	StRemediated Status = "Remediated"
	StAccepted   Status = "Accepted Risk"
)

var Statuses = []Status{StNew, StTriaged, StInProgress, StRemediated, StAccepted}

type Action string

const (
	ActView         Action = "view"
	ActCreate       Action = "create"
	ActEdit         Action = "edit"
	ActChangeStatus Action = "change_status"
	ActAssign       Action = "assign"
	ActLink         Action = "link"
	ActAttach       Action = "attach"
	ActImport       Action = "import"
	ActExport       Action = "export"
	ActViewAudit    Action = "view_audit"
	ActSetSLA       Action = "set_sla"
	ActManageRoles  Action = "manage_roles"
)

// SLA is the remediation target in days per severity.
type SLA map[Severity]int

// DefaultSLA - D8.
var DefaultSLA = SLA{SevCritical: 7, SevHigh: 30, SevMedium: 90, SevLow: 180}

var ErrNoAppRole = errors.New("no VulnMan role assigned")

func ParseRole(s string) (Role, error) {
	for _, r := range Roles {
		if string(r) == s {
			return r, nil
		}
	}
	return "", fmt.Errorf("unknown role %q", s)
}

func ParseSeverity(s string) (Severity, error) {
	for _, v := range Severities {
		if string(v) == s {
			return v, nil
		}
	}
	return "", fmt.Errorf("unknown severity %q", s)
}

func ParseStatus(s string) (Status, error) {
	for _, v := range Statuses {
		if string(v) == s {
			return v, nil
		}
	}
	return "", fmt.Errorf("unknown status %q", s)
}

// RoleFromClaims picks the user's app role from Keycloak realm roles: the
// highest of pm > analyst > developer > auditor; none is ErrNoAppRole.
func RoleFromClaims(roles []string) (Role, error) {
	have := map[string]bool{}
	for _, r := range roles {
		have[r] = true
	}
	for _, r := range Roles {
		if have[string(r)] {
			return r, nil
		}
	}
	return "", ErrNoAppRole
}

var matrix = map[Role]map[Action]bool{
	RoleAnalyst: set(ActView, ActCreate, ActImport, ActEdit, ActChangeStatus, ActAssign, ActLink, ActAttach, ActExport),
	// Developer: edit/status/link/attach only on findings assigned to them (CanOnFinding).
	RoleDeveloper: set(ActView, ActEdit, ActChangeStatus, ActLink, ActAttach),
	RolePM: set(ActView, ActCreate, ActImport, ActEdit, ActChangeStatus, ActAssign, ActLink, ActAttach, ActExport,
		ActViewAudit, ActSetSLA, ActManageRoles),
	RoleAuditor: set(ActView, ActExport, ActViewAudit),
}

func set(as ...Action) map[Action]bool {
	m := map[Action]bool{}
	for _, a := range as {
		m[a] = true
	}
	return m
}

// Can reports whether a role may perform an action at all.
func Can(r Role, a Action) bool { return matrix[r][a] }

// ownedActions are the ones a developer may only take on findings assigned to them (D5).
var ownedActions = set(ActEdit, ActChangeStatus, ActLink, ActAttach)

// CanOnFinding adds D5's ownership rule to Can.
func CanOnFinding(r Role, a Action, actorSub, assigneeSub string) bool {
	if !Can(r, a) {
		return false
	}
	if r == RoleDeveloper && ownedActions[a] {
		return assigneeSub != "" && actorSub == assigneeSub
	}
	return true
}

// transitions is D6's state machine.
var transitions = map[Status]map[Status]bool{
	StNew:        {StTriaged: true, StAccepted: true},
	StTriaged:    {StInProgress: true, StAccepted: true},
	StInProgress: {StRemediated: true, StAccepted: true},
	StRemediated: {StInProgress: true},
	StAccepted:   {StTriaged: true},
}

// CanTransition reports whether a role may move a finding from one status
// to another. Developers may only move to In Progress or Remediated;
// auditors never.
func CanTransition(r Role, from, to Status) bool {
	if !Can(r, ActChangeStatus) || !transitions[from][to] {
		return false
	}
	if r == RoleDeveloper {
		return to == StInProgress || to == StRemediated
	}
	return true
}

// IsOpen: not Remediated and not Accepted Risk.
func IsOpen(s Status) bool { return s != StRemediated && s != StAccepted }

// DueDate is the SLA deadline for a finding triaged at triagedAt.
func DueDate(triagedAt time.Time, sev Severity, sla SLA) time.Time {
	return triagedAt.Add(time.Duration(sla[sev]) * 24 * time.Hour)
}
