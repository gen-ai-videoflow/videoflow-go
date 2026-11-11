package db

import (
	"6eek-go/internal/models"
	"database/sql"
	"encoding/json"
	"fmt"
)

// ClaimRenderSubmission fences concurrent workers before the one permitted POST.
func (d *DB) ClaimRenderSubmission(id, model string, request []byte) (bool, error) {
	result, err := d.conn.Exec(`UPDATE render_jobs SET status='running',stage='submitting',model=?,request_json=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status IN ('pending','running') AND coalesce(provider_job_id,'')='' AND coalesce(stage,'')<>'submitting'`, model, string(request), id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
func (d *DB) InterruptOperations() error {
	_, err := d.conn.Exec(`UPDATE ops SET status='reconciliation_required',error='Server restarted during this operation. Review saved work before starting another request.',updated_at=CURRENT_TIMESTAMP WHERE status='running'`)
	return err
}
func (d *DB) ResolveRenderSubmission(id, providerID, reason string) error {
	evidence, _ := json.Marshal(map[string]string{"reconciliation_reason": reason, "adopted_provider_job_id": providerID})
	result, err := d.conn.Exec(`UPDATE render_jobs SET response_json=?,provider_job_id=?,status='running',stage='polling',error=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='reconciliation_required'`, string(evidence), providerID, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("job does not require reconciliation")
	}
	return nil
}

// CreateRenderRetry serializes retries of the same original, including headerless clients.
func (d *DB) CreateRenderRetry(original string, fresh *models.RenderJob) (bool, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS render_retries(original_job_id TEXT PRIMARY KEY,child_job_id TEXT NOT NULL)`); err != nil {
		return false, err
	}
	var child, status string
	err = tx.QueryRow(`SELECT j.id,j.status FROM render_retries r JOIN render_jobs j ON j.id=r.child_job_id WHERE original_job_id=?`, original).Scan(&child, &status)
	if err == nil && (status == "pending" || status == "running" || status == "reconciliation_required") {
		fresh.ID = child
		fresh.Status = status
		return false, tx.Commit()
	}
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	if err = tx.QueryRow(`SELECT status FROM render_jobs WHERE id=?`, original).Scan(&status); err != nil {
		return false, err
	}
	if status != "failed" && status != "succeeded" {
		return false, fmt.Errorf("original job is still active")
	}
	if err = insertRenderJob(tx, fresh); err != nil {
		return false, err
	}
	if _, err = tx.Exec(`INSERT INTO render_retries(original_job_id,child_job_id)VALUES(?,?) ON CONFLICT(original_job_id) DO UPDATE SET child_job_id=excluded.child_job_id`, original, fresh.ID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
