package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/crypt"
	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

type Finding struct {
	ID            int64           `json:"id"`
	Ref           string          `json:"ref"`
	Title         string          `json:"title"`
	Description   string          `json:"description,omitempty"`
	Asset         string          `json:"asset"`
	Severity      domain.Severity `json:"severity"`
	CVSS          *float64        `json:"cvss,omitempty"`
	CVE           string          `json:"cve,omitempty"`
	Status        domain.Status   `json:"status"`
	Source        string          `json:"source"`
	ReporterEmail string          `json:"reporterEmail,omitempty"`
	AssigneeSub   string          `json:"assigneeSub,omitempty"`
	AssigneeName  string          `json:"assigneeName,omitempty"`
	DueDate       *time.Time      `json:"dueDate,omitempty"`
	DiscoveredAt  time.Time       `json:"discoveredAt"`
	TriagedAt     *time.Time      `json:"triagedAt,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
	Fingerprint   string          `json:"-"`
}

type NewFinding struct {
	Title, Description, Asset, CVE, Source, ReporterEmail, Fingerprint string
	Severity                                                           domain.Severity
	CVSS                                                               *float64
	DiscoveredAt                                                       time.Time
}

// Patch: nil fields are left unchanged. ClearCVSS removes the score.
type Patch struct {
	Title, Description, Asset, CVE *string
	Severity                       *domain.Severity
	CVSS                           *float64
	ClearCVSS                      bool
}

type Filter struct {
	Status      []domain.Status
	Severity    []domain.Severity
	AssigneeSub string
	Asset       string
	Q           string // substring of title, asset or ref (case-insensitive, literal)
	Overdue     bool
	Limit       int // default 50, max 200
	Offset      int
}

const findingCols = `id, ref, title, asset, severity, cvss, cve, status, source, assignee_sub, assignee_name,
	due_date, discovered_at, triaged_at, created_at, updated_at, fingerprint`

func scanFinding(row pgx.Row, f *Finding) error {
	var cvss *float64
	err := row.Scan(&f.ID, &f.Ref, &f.Title, &f.Asset, &f.Severity, &cvss, &f.CVE, &f.Status, &f.Source, &f.AssigneeSub,
		&f.AssigneeName, &f.DueDate, &f.DiscoveredAt, &f.TriagedAt, &f.CreatedAt, &f.UpdatedAt, &f.Fingerprint)
	f.CVSS = cvss
	return err
}

func aad(id int64, field string) []byte { return []byte(strconv.FormatInt(id, 10) + ":" + field) }

// CreateFinding stores a new finding (status New) with its description and
// reporter e-mail sealed under a fresh per-finding data key (D9).
func (s *Store) CreateFinding(ctx context.Context, a Actor, nf NewFinding) (Finding, error) {
	if !validSource(nf.Source) {
		return Finding{}, invalid("unknown source %q", nf.Source)
	}
	dek, wrapped, err := s.transit.DataKey(ctx)
	if err != nil {
		return Finding{}, fmt.Errorf("data key: %w", err)
	}
	var f Finding
	err = s.mutate(ctx, func(tx pgx.Tx) error {
		var id int64
		if err := tx.QueryRow(ctx, `SELECT nextval('findings_id_seq')`).Scan(&id); err != nil {
			return err
		}
		desc, err := crypt.Seal(dek, []byte(nf.Description), aad(id, "description"))
		if err != nil {
			return err
		}
		email, err := crypt.Seal(dek, []byte(nf.ReporterEmail), aad(id, "reporter_email"))
		if err != nil {
			return err
		}
		now := s.ts()
		for attempt := 0; ; attempt++ {
			_, err = tx.Exec(ctx, `SAVEPOINT ins`)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO findings (id, ref, title, asset, severity, cvss, cve, status, source,
				description_ct, reporter_email_ct, wrapped_dek, discovered_at, created_at, updated_at, fingerprint)
				VALUES ($1,$2,$3,$4,$5,$6,$7,'New',$8,$9,$10,$11,$12,$13,$13,$14)`,
				id, NewRef(), nf.Title, nf.Asset, string(nf.Severity), nf.CVSS, nf.CVE, nf.Source, desc, email, wrapped,
				nf.DiscoveredAt.UTC(), now, nf.Fingerprint)
			if err == nil {
				break
			}
			if !isUniqueViolation(err) || attempt >= 3 {
				return err
			}
			if _, rerr := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT ins`); rerr != nil {
				return rerr
			}
			// fingerprint collisions are not retried - only the random ref is
			var n int
			_ = tx.QueryRow(ctx, `SELECT count(*) FROM findings WHERE fingerprint <> '' AND fingerprint = $1
				AND status NOT IN ('Remediated','Accepted Risk')`, nf.Fingerprint).Scan(&n)
			if n > 0 {
				return ErrConflict
			}
		}
		if err := scanFinding(tx.QueryRow(ctx, `SELECT `+findingCols+` FROM findings WHERE id=$1`, id), &f); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, a, "finding.create", &id, map[string]any{
			"ref": f.Ref, "title": f.Title, "asset": f.Asset, "severity": f.Severity, "source": f.Source,
		})
	})
	if err != nil {
		return Finding{}, err
	}
	f.Description, f.ReporterEmail = nf.Description, nf.ReporterEmail
	return f, nil
}

func validSource(s string) bool {
	switch s {
	case "internal", "external", "trivy", "csv":
		return true
	}
	return false
}

// GetFinding returns a finding with its sealed fields decrypted.
func (s *Store) GetFinding(ctx context.Context, id int64) (Finding, error) {
	var f Finding
	var desc, email []byte
	var wrapped string
	row := s.pool.QueryRow(ctx, `SELECT `+findingCols+`, description_ct, reporter_email_ct, wrapped_dek FROM findings WHERE id=$1`, id)
	var cvss *float64
	err := row.Scan(&f.ID, &f.Ref, &f.Title, &f.Asset, &f.Severity, &cvss, &f.CVE, &f.Status, &f.Source, &f.AssigneeSub,
		&f.AssigneeName, &f.DueDate, &f.DiscoveredAt, &f.TriagedAt, &f.CreatedAt, &f.UpdatedAt, &f.Fingerprint,
		&desc, &email, &wrapped)
	if err != nil {
		return Finding{}, notFound(err)
	}
	f.CVSS = cvss
	dek, err := s.transit.Unwrap(ctx, wrapped)
	if err != nil {
		return Finding{}, fmt.Errorf("%w: unwrap data key: %v", ErrDecrypt, err)
	}
	d, err := crypt.Open(dek, desc, aad(id, "description"))
	if err != nil {
		return Finding{}, fmt.Errorf("%w: description of finding %d", ErrDecrypt, id)
	}
	e, err := crypt.Open(dek, email, aad(id, "reporter_email"))
	if err != nil {
		return Finding{}, fmt.Errorf("%w: reporter e-mail of finding %d", ErrDecrypt, id)
	}
	f.Description, f.ReporterEmail = string(d), string(e)
	return f, nil
}

// ListFindings filters findings. Rows carry no decrypted fields.
func (s *Store) ListFindings(ctx context.Context, fl Filter) ([]Finding, int, error) {
	var where []string
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }
	if len(fl.Status) > 0 {
		ss := make([]string, len(fl.Status))
		for i, v := range fl.Status {
			ss[i] = string(v)
		}
		where = append(where, "status = ANY("+arg(ss)+")")
	}
	if len(fl.Severity) > 0 {
		ss := make([]string, len(fl.Severity))
		for i, v := range fl.Severity {
			ss[i] = string(v)
		}
		where = append(where, "severity = ANY("+arg(ss)+")")
	}
	if fl.AssigneeSub != "" {
		where = append(where, "assignee_sub = "+arg(fl.AssigneeSub))
	}
	if fl.Asset != "" {
		where = append(where, "asset = "+arg(fl.Asset))
	}
	if fl.Q != "" {
		p := arg("%" + likeEscape(strings.ToLower(fl.Q)) + "%")
		where = append(where, "(lower(title) LIKE "+p+" OR lower(asset) LIKE "+p+" OR lower(ref) LIKE "+p+")")
	}
	if fl.Overdue {
		where = append(where, "status NOT IN ('Remediated','Accepted Risk') AND due_date < "+arg(s.ts()))
	}
	limit := fl.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	q := `SELECT ` + findingCols + `, count(*) OVER () FROM findings`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY updated_at DESC, id DESC LIMIT " + strconv.Itoa(limit) + " OFFSET " + strconv.Itoa(max(fl.Offset, 0))
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Finding
	total := 0
	for rows.Next() {
		var f Finding
		var cvss *float64
		if err := rows.Scan(&f.ID, &f.Ref, &f.Title, &f.Asset, &f.Severity, &cvss, &f.CVE, &f.Status, &f.Source, &f.AssigneeSub,
			&f.AssigneeName, &f.DueDate, &f.DiscoveredAt, &f.TriagedAt, &f.CreatedAt, &f.UpdatedAt, &f.Fingerprint, &total); err != nil {
			return nil, 0, err
		}
		f.CVSS = cvss
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(out) == 0 && fl.Offset > 0 {
		// count(*) OVER () has no rows to ride on past the end
		if err := s.countOnly(ctx, where, args, &total); err != nil {
			return nil, 0, err
		}
	}
	return out, total, nil
}

func (s *Store) countOnly(ctx context.Context, where []string, args []any, total *int) error {
	q := `SELECT count(*) FROM findings`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	return s.pool.QueryRow(ctx, q, args...).Scan(total)
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// lockFinding loads a finding FOR UPDATE inside a mutation.
func lockFinding(ctx context.Context, tx pgx.Tx, id int64) (Finding, error) {
	var f Finding
	err := scanFinding(tx.QueryRow(ctx, `SELECT `+findingCols+` FROM findings WHERE id=$1 FOR UPDATE`, id), &f)
	return f, notFound(err)
}

// UpdateFinding applies a patch. The audit row lists changed fields, with
// old/new values for the plaintext ones - never the description.
func (s *Store) UpdateFinding(ctx context.Context, a Actor, id int64, p Patch) (Finding, error) {
	err := s.mutate(ctx, func(tx pgx.Tx) error {
		cur, err := lockFinding(ctx, tx, id)
		if err != nil {
			return err
		}
		changes := map[string]any{}
		sets := []string{}
		var args []any
		set := func(col string, v any) {
			args = append(args, v)
			sets = append(sets, col+" = $"+strconv.Itoa(len(args)))
		}
		if p.Title != nil && *p.Title != cur.Title {
			changes["title"] = map[string]any{"old": cur.Title, "new": *p.Title}
			set("title", *p.Title)
		}
		if p.Asset != nil && *p.Asset != cur.Asset {
			changes["asset"] = map[string]any{"old": cur.Asset, "new": *p.Asset}
			set("asset", *p.Asset)
		}
		if p.Severity != nil && *p.Severity != cur.Severity {
			changes["severity"] = map[string]any{"old": cur.Severity, "new": *p.Severity}
			set("severity", string(*p.Severity))
		}
		if p.CVE != nil && *p.CVE != cur.CVE {
			changes["cve"] = map[string]any{"old": cur.CVE, "new": *p.CVE}
			set("cve", *p.CVE)
		}
		if p.ClearCVSS && cur.CVSS != nil {
			changes["cvss"] = map[string]any{"old": *cur.CVSS, "new": nil}
			set("cvss", nil)
		} else if p.CVSS != nil && (cur.CVSS == nil || *cur.CVSS != *p.CVSS) {
			changes["cvss"] = map[string]any{"old": cur.CVSS, "new": *p.CVSS}
			set("cvss", *p.CVSS)
		}
		if p.Description != nil {
			var wrapped string
			if err := tx.QueryRow(ctx, `SELECT wrapped_dek FROM findings WHERE id=$1`, id).Scan(&wrapped); err != nil {
				return err
			}
			dek, err := s.transit.Unwrap(ctx, wrapped)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrDecrypt, err)
			}
			ct, err := crypt.Seal(dek, []byte(*p.Description), aad(id, "description"))
			if err != nil {
				return err
			}
			changes["description"] = "changed"
			set("description_ct", ct)
		}
		if len(sets) == 0 {
			return nil
		}
		set("updated_at", s.ts())
		args = append(args, id)
		if _, err := tx.Exec(ctx, `UPDATE findings SET `+strings.Join(sets, ", ")+` WHERE id = $`+strconv.Itoa(len(args)), args...); err != nil {
			return err
		}
		fields := make([]string, 0, len(changes))
		for k := range changes {
			fields = append(fields, k)
		}
		changes["fields"] = fields
		return s.appendAudit(ctx, tx, a, "finding.update", &id, changes)
	})
	if err != nil {
		return Finding{}, err
	}
	return s.GetFinding(ctx, id)
}

// ChangeStatus moves a finding through D6's state machine for the actor's
// role. Triaging sets triaged_at and, unless a due date exists, the SLA due date.
func (s *Store) ChangeStatus(ctx context.Context, a Actor, id int64, to domain.Status, note string) (Finding, error) {
	err := s.mutate(ctx, func(tx pgx.Tx) error {
		cur, err := lockFinding(ctx, tx, id)
		if err != nil {
			return err
		}
		if !domain.CanTransition(a.Role, cur.Status, to) {
			return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, cur.Status, to)
		}
		now := s.ts()
		details := map[string]any{"from": cur.Status, "to": to}
		if note != "" {
			details["note"] = note
		}
		if to == domain.StTriaged {
			due := cur.DueDate
			if due == nil {
				sla, err := slaTx(ctx, tx)
				if err != nil {
					return err
				}
				d := domain.DueDate(now, cur.Severity, sla)
				due = &d
				details["dueDate"] = d
			}
			_, err = tx.Exec(ctx, `UPDATE findings SET status=$1, triaged_at=$2, due_date=$3, updated_at=$2 WHERE id=$4`, string(to), now, due, id)
		} else {
			_, err = tx.Exec(ctx, `UPDATE findings SET status=$1, updated_at=$2 WHERE id=$3`, string(to), now, id)
		}
		if err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, a, "finding.status", &id, details)
	})
	if err != nil {
		return Finding{}, err
	}
	return s.GetFinding(ctx, id)
}

// Assign sets the assignee and, when due is non-nil, the due date.
func (s *Store) Assign(ctx context.Context, a Actor, id int64, assigneeSub, assigneeName string, due *time.Time) (Finding, error) {
	err := s.mutate(ctx, func(tx pgx.Tx) error {
		cur, err := lockFinding(ctx, tx, id)
		if err != nil {
			return err
		}
		details := map[string]any{
			"assignee": map[string]any{"old": cur.AssigneeSub, "new": assigneeSub},
		}
		newDue := cur.DueDate
		if due != nil {
			d := due.UTC().Truncate(time.Microsecond)
			newDue = &d
			details["dueDate"] = map[string]any{"old": cur.DueDate, "new": d}
		}
		if _, err := tx.Exec(ctx, `UPDATE findings SET assignee_sub=$1, assignee_name=$2, due_date=$3, updated_at=$4 WHERE id=$5`,
			assigneeSub, assigneeName, newDue, s.ts(), id); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, a, "finding.assign", &id, details)
	})
	if err != nil {
		return Finding{}, err
	}
	return s.GetFinding(ctx, id)
}

// FindingIDByRef resolves a "VM-…" reference.
func (s *Store) FindingIDByRef(ctx context.Context, ref string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM findings WHERE ref = $1`, ref).Scan(&id)
	return id, notFound(err)
}
