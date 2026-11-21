package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/crypt"
)

// MaxAttachments per finding (D13).
const MaxAttachments = 20

type Attachment struct {
	ID          int64     `json:"id"`
	FindingID   int64     `json:"findingId"`
	Name        string    `json:"name"`
	ContentType string    `json:"contentType"`
	Size        int       `json:"size"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

func attAAD(findingID, attID int64) []byte {
	return []byte(strconv.FormatInt(findingID, 10) + ":attachment:" + strconv.FormatInt(attID, 10))
}

// AddAttachment stores bytes sealed under the finding's data key. Type and
// size checks happen in the API (sniffed type, 10 MiB cap).
// requireAssignee (optional) must still be the assignee under the row lock.
func (s *Store) AddAttachment(ctx context.Context, a Actor, findingID int64, name, contentType string, data []byte, requireAssignee ...string) (Attachment, error) {
	var att Attachment
	err := s.mutate(ctx, func(tx pgx.Tx) error {
		var wrapped, assignee string
		if err := tx.QueryRow(ctx, `SELECT wrapped_dek, assignee_sub FROM findings WHERE id=$1 FOR UPDATE`, findingID).Scan(&wrapped, &assignee); err != nil {
			return notFound(err)
		}
		if len(requireAssignee) > 0 && requireAssignee[0] != "" && assignee != requireAssignee[0] {
			return ErrForbidden
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM attachments WHERE finding_id=$1`, findingID).Scan(&n); err != nil {
			return err
		}
		if n >= MaxAttachments {
			return fmt.Errorf("%w: at most %d attachments per finding", ErrConflict, MaxAttachments)
		}
		dek, err := s.transit.Unwrap(ctx, wrapped)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrDecrypt, err)
		}
		var id int64
		if err := tx.QueryRow(ctx, `SELECT nextval('attachments_id_seq')`).Scan(&id); err != nil {
			return err
		}
		ct, err := crypt.Seal(dek, data, attAAD(findingID, id))
		if err != nil {
			return err
		}
		att = Attachment{ID: id, FindingID: findingID, Name: name, ContentType: contentType, Size: len(data), CreatedBy: a.Name, CreatedAt: s.ts()}
		if _, err := tx.Exec(ctx, `INSERT INTO attachments (id, finding_id, name, content_type, size, data_ct, created_by, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, id, findingID, name, contentType, len(data), ct, a.Name, att.CreatedAt); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, a, "attachment.add", &findingID, map[string]any{"name": name, "size": len(data), "type": contentType})
	})
	return att, err
}

// Attachment returns one attachment of one finding, decrypted.
func (s *Store) Attachment(ctx context.Context, findingID, attID int64) (Attachment, []byte, error) {
	var att Attachment
	var ct []byte
	var wrapped string
	err := s.pool.QueryRow(ctx, `SELECT a.id, a.finding_id, a.name, a.content_type, a.size, a.created_by, a.created_at, a.data_ct, f.wrapped_dek
		FROM attachments a JOIN findings f ON f.id = a.finding_id WHERE a.id=$1 AND a.finding_id=$2`, attID, findingID).
		Scan(&att.ID, &att.FindingID, &att.Name, &att.ContentType, &att.Size, &att.CreatedBy, &att.CreatedAt, &ct, &wrapped)
	if err != nil {
		return Attachment{}, nil, notFound(err)
	}
	dek, err := s.transit.Unwrap(ctx, wrapped)
	if err != nil {
		return Attachment{}, nil, fmt.Errorf("%w: %v", ErrDecrypt, err)
	}
	data, err := crypt.Open(dek, ct, attAAD(findingID, attID))
	if err != nil {
		return Attachment{}, nil, fmt.Errorf("%w: attachment %d", ErrDecrypt, attID)
	}
	return att, data, nil
}

func (s *Store) Attachments(ctx context.Context, findingID int64) ([]Attachment, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, finding_id, name, content_type, size, created_by, created_at FROM attachments
		WHERE finding_id=$1 ORDER BY id`, findingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attachment{}
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.FindingID, &a.Name, &a.ContentType, &a.Size, &a.CreatedBy, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
