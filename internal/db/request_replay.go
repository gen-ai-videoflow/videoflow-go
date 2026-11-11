package db

import (
	"database/sql"
	"fmt"
)

type Replay struct {
	Status int
	Body   []byte
}

func (d *DB) ensureRequestReplay() error {
	_, err := d.conn.Exec(`CREATE TABLE IF NOT EXISTS generation_requests(request_key TEXT PRIMARY KEY,fingerprint TEXT NOT NULL,status INTEGER NOT NULL DEFAULT 0,body BLOB NOT NULL DEFAULT '',created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);CREATE INDEX IF NOT EXISTS generation_request_fingerprint ON generation_requests(fingerprint);`)
	return err
}

// A pending response is deliberately retained after a crash: its side effects are uncertain.
func (d *DB) BeginRequest(key, fingerprint string) (*Replay, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var previous string
	var r Replay
	err = tx.QueryRow(`SELECT fingerprint,status,body FROM generation_requests g WHERE request_key=? OR (fingerprint=? AND (status=0 OR (status=202 AND (EXISTS(SELECT 1 FROM ops o WHERE o.id=json_extract(CAST(g.body AS TEXT),'$.op_id') AND (o.status IN ('running','reconciliation_required') OR (o.status='succeeded' AND EXISTS(SELECT 1 FROM json_each(o.result_json,'$.render_job_ids') ids JOIN render_jobs j ON j.id=ids.value WHERE j.status IN ('pending','running','reconciliation_required'))))) OR EXISTS(SELECT 1 FROM render_jobs WHERE id=json_extract(CAST(g.body AS TEXT),'$.job_id') AND status IN ('pending','running','reconciliation_required')))))) ORDER BY (request_key=?) DESC LIMIT 1`, key, fingerprint, key).Scan(&previous, &r.Status, &r.Body)
	if err == nil {
		if previous != fingerprint {
			return nil, fmt.Errorf("request key was used for different inputs")
		}
		return &r, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	if _, err = tx.Exec(`INSERT INTO generation_requests(request_key,fingerprint)VALUES(?,?)`, key, fingerprint); err != nil {
		return nil, err
	}
	return nil, tx.Commit()
}
func (d *DB) FinishRequest(key string, status int, body []byte) error {
	_, err := d.conn.Exec(`UPDATE generation_requests SET status=?,body=? WHERE request_key=?`, status, body, key)
	return err
}

func (d *DB) PendingRequests() ([]map[string]string, error) {
	rows, err := d.conn.Query(`SELECT request_key,created_at FROM generation_requests WHERE status=0 OR (status=202 AND EXISTS(SELECT 1 FROM ops WHERE id=json_extract(CAST(generation_requests.body AS TEXT),'$.op_id') AND status='reconciliation_required')) ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []map[string]string{}
	for rows.Next() {
		var key, at string
		if err = rows.Scan(&key, &at); err != nil {
			return nil, err
		}
		result = append(result, map[string]string{"key": key, "created_at": at})
	}
	return result, rows.Err()
}
func (d *DB) AcknowledgeRequest(key, reason string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var eligible int
	if err = tx.QueryRow(`SELECT count(*) FROM generation_requests g WHERE request_key=? AND (status=0 OR (status=202 AND EXISTS(SELECT 1 FROM ops WHERE id=json_extract(CAST(g.body AS TEXT),'$.op_id') AND status='reconciliation_required')))`, key).Scan(&eligible); err != nil {
		return err
	}
	if eligible != 1 {
		return fmt.Errorf("request is not awaiting recovery")
	}
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS generation_recovery_audit(request_key TEXT NOT NULL,reason TEXT NOT NULL,created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE ops SET status='failed' WHERE status='reconciliation_required' AND id=(SELECT json_extract(CAST(body AS TEXT),'$.op_id') FROM generation_requests WHERE request_key=?)`, key); err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE generation_requests SET status=409,body='{"detail":"Interrupted request reviewed. Starting again is a new generation and may incur another charge."}' WHERE request_key=? AND (status=0 OR status=202)`, key)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("request is not awaiting recovery")
	}
	if _, err = tx.Exec(`INSERT INTO generation_recovery_audit(request_key,reason)VALUES(?,?)`, key, reason); err != nil {
		return err
	}
	return tx.Commit()
}
