package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type AuditEntry struct {
	ID        int64          `json:"id"`
	At        time.Time      `json:"at"`
	ActorSub  string         `json:"actorSub"`
	ActorRole string         `json:"actorRole"`
	Action    string         `json:"action"`
	FindingID *int64         `json:"findingId,omitempty"`
	Details   map[string]any `json:"details"`
	Hash      string         `json:"hash"`
}

type AuditFilter struct {
	FindingID *int64
	ActorSub  string
	From, To  *time.Time
	Limit     int // default 100, max 1000
}

type VerifyResult struct {
	OK         bool   `json:"ok"`
	Checked    int    `json:"checked"`
	FirstBadID *int64 `json:"firstBadId,omitempty"`
}

// normalize round-trips details through JSON, the way jsonb storage and the
// verifier will see them, so the hash is computed over the same bytes.
func normalize(details map[string]any) (map[string]any, error) {
	if details == nil {
		details = map[string]any{}
	}
	b, err := json.Marshal(details)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	return out, json.Unmarshal(b, &out)
}

// chainHash = SHA-256(prev ‖ JSON{id, at, actor_sub, actor_role, action,
// finding_id, details}) with the keys in this fixed order.
func chainHash(prev []byte, id int64, at time.Time, actorSub, actorRole, action string, findingID *int64, details map[string]any) ([]byte, error) {
	det, err := json.Marshal(details)
	if err != nil {
		return nil, err
	}
	var fid []byte = []byte("null")
	if findingID != nil {
		fid = []byte(strconv.FormatInt(*findingID, 10))
	}
	str := func(s string) []byte { b, _ := json.Marshal(s); return b }
	var buf bytes.Buffer
	buf.WriteString(`{"id":` + strconv.FormatInt(id, 10))
	buf.WriteString(`,"at":`)
	buf.Write(str(at.UTC().Format(time.RFC3339Nano)))
	buf.WriteString(`,"actor_sub":`)
	buf.Write(str(actorSub))
	buf.WriteString(`,"actor_role":`)
	buf.Write(str(actorRole))
	buf.WriteString(`,"action":`)
	buf.Write(str(action))
	buf.WriteString(`,"finding_id":`)
	buf.Write(fid)
	buf.WriteString(`,"details":`)
	buf.Write(det)
	buf.WriteString(`}`)
	h := sha256.New()
	h.Write(prev)
	h.Write(buf.Bytes())
	return h.Sum(nil), nil
}

// appendAudit adds one row to the chain. The caller holds the chain lock (mutate).
func (s *Store) appendAudit(ctx context.Context, tx pgx.Tx, a Actor, action string, findingID *int64, details map[string]any) error {
	det, err := normalize(details)
	if err != nil {
		return err
	}
	var prev []byte
	if err := tx.QueryRow(ctx, `SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&prev); err != nil && err != pgx.ErrNoRows {
		return err
	}
	var id int64
	if err := tx.QueryRow(ctx, `SELECT nextval('audit_log_id_seq')`).Scan(&id); err != nil {
		return err
	}
	at := s.ts()
	h, err := chainHash(prev, id, at, a.Sub, string(a.Role), action, findingID, det)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_log (id, at, actor_sub, actor_role, action, finding_id, details, prev_hash, hash)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, at, a.Sub, string(a.Role), action, findingID, det, prev, h)
	return err
}

// Audit lists audit entries, oldest first.
func (s *Store) Audit(ctx context.Context, f AuditFilter) ([]AuditEntry, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	if f.FindingID != nil {
		add("finding_id = ?", *f.FindingID)
	}
	if f.ActorSub != "" {
		add("actor_sub = ?", f.ActorSub)
	}
	if f.From != nil {
		add("at >= ?", *f.From)
	}
	if f.To != nil {
		add("at < ?", *f.To)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	q := `SELECT id, at, actor_sub, actor_role, action, finding_id, details, hash FROM audit_log`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY id LIMIT " + strconv.Itoa(limit)
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var h []byte
		if err := rows.Scan(&e.ID, &e.At, &e.ActorSub, &e.ActorRole, &e.Action, &e.FindingID, &e.Details, &h); err != nil {
			return nil, err
		}
		e.Hash = hex.EncodeToString(h)
		out = append(out, e)
	}
	return out, rows.Err()
}

// VerifyAudit recomputes the whole chain and reports the first row whose
// stored hash or link to its predecessor doesn't match.
func (s *Store) VerifyAudit(ctx context.Context) (VerifyResult, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, at, actor_sub, actor_role, action, finding_id, details, prev_hash, hash FROM audit_log ORDER BY id`)
	if err != nil {
		return VerifyResult{}, err
	}
	defer rows.Close()
	res := VerifyResult{OK: true}
	var prev []byte
	for rows.Next() {
		var (
			id                     int64
			at                     time.Time
			sub, role, action      string
			fid                    *int64
			det                    map[string]any
			storedPrev, storedHash []byte
		)
		if err := rows.Scan(&id, &at, &sub, &role, &action, &fid, &det, &storedPrev, &storedHash); err != nil {
			return VerifyResult{}, err
		}
		res.Checked++
		want, err := chainHash(prev, id, at, sub, role, action, fid, det)
		if err != nil {
			return VerifyResult{}, err
		}
		if !bytes.Equal(storedPrev, prev) || !bytes.Equal(storedHash, want) {
			res.OK = false
			res.FirstBadID = &id
			return res, nil
		}
		prev = storedHash
	}
	return res, rows.Err()
}
