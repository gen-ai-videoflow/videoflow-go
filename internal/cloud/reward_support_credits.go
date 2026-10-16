package cloud

import (
	"context"
	"database/sql"
)

// Account row must already be locked. Lots expire for NEW reservations only;
// jobs that reserved before expiry retain that backing through settlement.
func reservePromotionalCredits(ctx context.Context, tx *sql.Tx, workspace, job string, wanted int64) (int64, error) {
	rows, e := tx.QueryContext(ctx, `SELECT id,credits-consumed-reserved FROM cloud_promotional_credits WHERE workspace_id=$1 AND expires_at>now() AND credits>consumed+reserved ORDER BY expires_at,id FOR UPDATE`, workspace)
	if e != nil {
		return 0, e
	}
	type lot struct {
		id string
		n  int64
	}
	lots := []lot{}
	for rows.Next() {
		var l lot
		if e = rows.Scan(&l.id, &l.n); e != nil {
			rows.Close()
			return 0, e
		}
		lots = append(lots, l)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	var used int64
	for _, l := range lots {
		n := l.n
		if n > wanted-used {
			n = wanted - used
		}
		if n == 0 {
			break
		}
		if _, e = tx.ExecContext(ctx, `UPDATE cloud_promotional_credits SET reserved=reserved+$2 WHERE id=$1`, l.id, n); e != nil {
			return 0, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_promotional_credit_reservations(job_id,grant_id,credits)VALUES($1,$2,$3)`, job, l.id, n); e != nil {
			return 0, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_promotional_credit_events(grant_id,job_id,kind,credits)VALUES($1,$2,'reserve',$3)`, l.id, job, n); e != nil {
			return 0, e
		}
		used += n
	}
	if _, e = tx.ExecContext(ctx, `UPDATE cloud_jobs SET promotional_credits=$2 WHERE id=$1`, job, used); e != nil {
		return 0, e
	}
	return used, nil
}
func settlePromotionalCredits(ctx context.Context, tx *sql.Tx, job, workspace string, success bool) (int64, error) {
	// Match SubmitJob's account-before-lots lock order. A finished job is already locked.
	var lock string
	if e := tx.QueryRowContext(ctx, `SELECT workspace_id FROM cloud_credit_accounts WHERE workspace_id=$1 FOR UPDATE`, workspace).Scan(&lock); e != nil {
		return 0, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT grant_id,credits FROM cloud_promotional_credit_reservations WHERE job_id=$1 AND status='reserved' ORDER BY grant_id`, job)
	if e != nil {
		return 0, e
	}
	type reservation struct {
		id string
		n  int64
	}
	all := []reservation{}
	for rows.Next() {
		var x reservation
		if e = rows.Scan(&x.id, &x.n); e != nil {
			rows.Close()
			return 0, e
		}
		all = append(all, x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	var total int64
	for _, x := range all {
		consumed := int64(0)
		kind, status := "release", "released"
		if success {
			consumed = x.n
			kind, status = "consume", "consumed"
		}
		if _, e = tx.ExecContext(ctx, `UPDATE cloud_promotional_credits SET reserved=reserved-$2,consumed=consumed+$3 WHERE id=$1`, x.id, x.n, consumed); e != nil {
			return 0, e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE cloud_promotional_credit_reservations SET status=$3 WHERE job_id=$1 AND grant_id=$2`, job, x.id, status); e != nil {
			return 0, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_promotional_credit_events(grant_id,job_id,kind,credits)VALUES($1,$2,$3,$4)`, x.id, job, kind, x.n); e != nil {
			return 0, e
		}
		total += x.n
	}
	return total, nil
}
