package cloud

import (
	"context"
	"net/http"
	"time"
)

const MaxUploadBytes int64 = 100 << 20
const WorkspaceUploadQuotaBytes int64 = 1000 << 20

// Reservations remain charged until a cleanup process confirms object deletion.
func (s *Store) ReserveUpload(ctx context.Context, workspace, key string, bytes int64) error {
	if workspace == "" || !validMediaKey(key) || bytes < 1 || bytes > MaxUploadBytes {
		return ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked string
	if err = tx.QueryRowContext(ctx, `SELECT workspace_id FROM cloud_credit_accounts WHERE workspace_id=$1 FOR UPDATE`, workspace).Scan(&locked); err != nil {
		return err
	}
	var used int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(sum(bytes),0) FROM cloud_upload_reservations WHERE workspace_id=$1 AND deleted_at IS NULL`, workspace).Scan(&used); err != nil {
		return err
	}
	if used > WorkspaceUploadQuotaBytes-bytes {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_upload_reservations(object_key,workspace_id,bytes)VALUES($1,$2,$3)`, key, workspace, bytes); err != nil {
		return dbError(err)
	}
	return tx.Commit()
}
func (s *Server) createUploadGrant(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if !u.Verified {
		s.fail(w, ErrUnauthorized)
		return
	}
	if !s.rate(w, r, "upload:"+u.ID, 10, time.Hour, true) {
		return
	}
	if s.Media == nil {
		s.fail(w, ErrUnavailable)
		return
	}
	var in struct {
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Bytes       int64  `json:"bytes"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	grant, err := s.Media.UploadPostGrant(r.Context(), u.WorkspaceID, in.Filename, in.ContentType, in.Bytes)
	if err != nil {
		s.fail(w, err)
		return
	}
	key, _ := grant["key"].(string)
	if err = s.Store.ReserveUpload(r.Context(), u.WorkspaceID, key, in.Bytes); err != nil {
		s.fail(w, err)
		return
	}
	s.json(w, 200, grant)
}
