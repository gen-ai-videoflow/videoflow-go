package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// Maintenance performs bounded, retryable retention work. A storage failure never
// releases a reservation or removes raw analytics. Financial/audit rows are kept.
func (w *Worker) Maintenance(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return errors.Join(w.cleanupVideoUploads(ctx), w.cleanupExpiredOperationalRows(ctx), w.archiveExpiredWatchEvents(ctx), w.cleanupExpiredUploads(ctx))
}
func objectAbsent(err error) bool {
	if err == nil {
		return false
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchUpload", "404":
			return true
		case "NoSuchBucket":
			return false
		}
	}
	var status interface{ HTTPStatusCode() int }
	return errors.As(err, &status) && status.HTTPStatusCode() == 404
}
func (w *Worker) cleanupExpiredUploads(ctx context.Context) error {
	if w.media == nil || w.media.s3 == nil || w.cfg.UploadsBucket == "" {
		return nil
	}
	tx, e := w.store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT object_key FROM cloud_upload_reservations WHERE deleted_at IS NULL AND expires_at<now() AND created_at<now()-interval '24 hours' ORDER BY expires_at LIMIT 100 FOR UPDATE SKIP LOCKED`)
	if e != nil {
		return e
	}
	keys := []string{}
	for rows.Next() {
		var key string
		if e = rows.Scan(&key); e != nil {
			rows.Close()
			return e
		}
		keys = append(keys, key)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	var failures []error
	for _, key := range keys {
		if !validMediaKey(key) {
			failures = append(failures, ErrValidation)
			continue
		}
		_, e = w.media.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(w.cfg.UploadsBucket), Key: aws.String(key)})
		if e == nil {
			_, e = w.media.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(w.cfg.UploadsBucket), Key: aws.String(key)})
			if e != nil {
				failures = append(failures, fmt.Errorf("delete upload quarantine: %w", e))
				continue
			}
			_, e = w.media.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(w.cfg.UploadsBucket), Key: aws.String(key)})
		}
		if !objectAbsent(e) {
			if e == nil {
				e = errors.New("object still present after deletion")
			}
			failures = append(failures, fmt.Errorf("confirm upload quarantine deletion: %w", e))
			continue
		}
		if _, e = tx.ExecContext(ctx, `UPDATE cloud_upload_reservations SET deleted_at=now() WHERE object_key=$1 AND deleted_at IS NULL`, key); e != nil {
			return e
		}
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	return errors.Join(failures...)
}

type archivedWatchEvent struct {
	EventHash string    `json:"event_hash"`
	VideoID   string    `json:"video_id"`
	Seconds   int       `json:"seconds"`
	CreatedAt time.Time `json:"created_at"`
}

func (w *Worker) archiveExpiredWatchEvents(ctx context.Context) error {
	if w.media == nil || w.media.s3 == nil || w.cfg.AnalyticsBucket == "" {
		return nil
	}
	tx, e := w.store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var locked bool
	e = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(814037223)`).Scan(&locked)
	if e != nil || !locked {
		return e
	}
	var day time.Time
	e = tx.QueryRowContext(ctx, `SELECT date_trunc('day',created_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' FROM cloud_watch_events WHERE created_at<now()-interval '90 days' ORDER BY created_at LIMIT 1`).Scan(&day)
	if e == sql.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	rows, e := tx.QueryContext(ctx, `SELECT id,video_id,seconds,created_at FROM cloud_watch_events WHERE created_at>=$1 AND created_at<$2 AND created_at<now()-interval '90 days' ORDER BY id LIMIT 1000 FOR UPDATE`, day, day.AddDate(0, 0, 1))
	if e != nil {
		return e
	}
	events := []archivedWatchEvent{}
	ids := []string{}
	for rows.Next() {
		var id string
		var event archivedWatchEvent
		if e = rows.Scan(&id, &event.VideoID, &event.Seconds, &event.CreatedAt); e != nil {
			rows.Close()
			return e
		}
		event.CreatedAt = event.CreatedAt.UTC()
		hash := sha256.Sum256([]byte(id))
		event.EventHash = hex.EncodeToString(hash[:])
		events = append(events, event)
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if len(ids) == 0 {
		return nil
	}
	// No account IDs, email addresses, client identifiers, or session IDs leave PG.
	payload, e := json.Marshal(events)
	if e != nil {
		return e
	}
	payloadHash := sha256.Sum256(payload)
	idJSON, _ := json.Marshal(ids)
	batchHash := sha256.Sum256(idJSON)
	batchID := hex.EncodeToString(batchHash[:])
	digest := hex.EncodeToString(payloadHash[:])
	key := "watch/" + day.Format("2006-01-02") + "/" + batchID + ".json"
	_, e = w.media.s3.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(w.cfg.AnalyticsBucket), Key: aws.String(key), Body: bytes.NewReader(payload), ContentType: aws.String("application/json"), ChecksumSHA256: aws.String(base64SHA256(payloadHash))})
	if e != nil {
		return fmt.Errorf("archive watch batch: %w", e)
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_analytics_archive_batches(id,object_key,payload_sha256,event_count,day)VALUES($1,$2,$3,$4,$5)ON CONFLICT(id)DO NOTHING`, batchID, key, digest, len(events), day)
	if e != nil {
		return e
	}
	// Deletes bind the selected immutable IDs, not a broad moving time window.
	for _, id := range ids {
		if _, e = tx.ExecContext(ctx, `DELETE FROM cloud_watch_events WHERE id=$1`, id); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func base64SHA256(hash [32]byte) string { return base64.StdEncoding.EncodeToString(hash[:]) }
func (w *Worker) cleanupExpiredOperationalRows(ctx context.Context) error {
	tx, e := w.store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `DELETE FROM cloud_playback_grants WHERE token_hash IN (SELECT token_hash FROM cloud_playback_grants WHERE expires_at<now() LIMIT 5000)`)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `DELETE FROM cloud_sessions WHERE id IN (SELECT id FROM cloud_sessions WHERE expires_at<now() OR last_seen_at<now()-interval '7 days' LIMIT 5000)`)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `DELETE FROM cloud_auth_challenges WHERE token_hash IN (SELECT token_hash FROM cloud_auth_challenges WHERE expires_at<now() LIMIT 5000)`)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `DELETE FROM cloud_job_events WHERE id IN (SELECT e.id FROM cloud_job_events e JOIN cloud_jobs j ON j.id=e.job_id WHERE e.created_at<now()-interval '7 days' AND j.status IN ('succeeded','failed','cancelled') LIMIT 5000)`)
	if e != nil {
		return e
	}
	return tx.Commit()
}

// clearHistory removes user-linkable viewing history, preserving aggregate
// counters and all financial evidence. This does not claim archive erasure.
func (s *Server) clearHistory(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u.ID == "" {
		s.fail(w, ErrUnauthorized)
		return
	}
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	statements := []string{
		`DELETE FROM cloud_video_progress WHERE user_id=$1`,
		`UPDATE cloud_watch_events SET user_id=NULL WHERE user_id=$1`,
		`UPDATE cloud_watch_sessions SET user_id=NULL,viewer_hash='cleared:'||id WHERE user_id=$1`,
		`DELETE FROM cloud_daily_viewers WHERE viewer_hash=$1`,
		`DELETE FROM cloud_viewer_preferences WHERE user_id=$1`,
	}
	for _, statement := range statements {
		args := []any{u.ID}
		if statement == `DELETE FROM cloud_daily_viewers WHERE viewer_hash=$1` {
			args = []any{tokenHash(u.ID)}
		}
		if _, e = tx.ExecContext(r.Context(), statement, args...); e != nil {
			s.fail(w, e)
			return
		}
	}
	e = tx.Commit()
	s.result(w, map[string]bool{"ok": true}, e)
}
func (s *Server) requestAccountDeletion(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u.ID == "" {
		s.fail(w, ErrUnauthorized)
		return
	}
	var credentials struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !s.decode(w, r, &credentials) {
		return
	}
	if e := s.reauth(r.Context(), r, u, credentials.Password, credentials.Code, false); e != nil {
		s.fail(w, e)
		return
	}
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	var locked string
	if e = tx.QueryRowContext(r.Context(), `SELECT id FROM cloud_users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&locked); e != nil {
		s.fail(w, e)
		return
	}
	_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_account_deletion_requests(user_id)VALUES($1)ON CONFLICT(user_id)DO NOTHING`, u.ID)
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `DELETE FROM cloud_sessions WHERE user_id=$1`, u.ID)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `DELETE FROM cloud_tokens WHERE user_id=$1`, u.ID)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `DELETE FROM cloud_auth_challenges WHERE user_id=$1`, u.ID)
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		s.fail(w, e)
		return
	}
	s.cookie(w, "", -1)
	s.json(w, http.StatusAccepted, map[string]string{"status": "requested"})
}
