package cloud

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"strings"
)

func rewardNotifyTx(ctx context.Context, tx *sql.Tx, user, key, kind, message string) error {
	if user == "" || len(key) == 0 || len(key) > 200 || strings.TrimSpace(message) == "" || len(message) > 2000 {
		return ErrValidation
	}
	var id string
	e := tx.QueryRowContext(ctx, `INSERT INTO cloud_reward_notifications(id,user_id,dedup_key,kind,message)VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id,dedup_key) DO UPDATE SET dedup_key=excluded.dedup_key RETURNING id`, uuid.NewString(), user, key, kind, message).Scan(&id)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_notification_deliveries(notification_id)VALUES($1) ON CONFLICT DO NOTHING`, id)
	return e
}

// RecordRewardNotification is safe for worker retries. In-app delivery is durable;
// external delivery is explicitly unsupported until a configured adapter exists.
func (s *Store) RecordRewardNotification(ctx context.Context, user, key, kind, message string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = rewardNotifyTx(ctx, tx, user, key, kind, message); e != nil {
		return e
	}
	return tx.Commit()
}

func rewardNotifyBatchTx(ctx context.Context, tx *sql.Tx, batch string) error {
	rows, e := tx.QueryContext(ctx, `SELECT DISTINCT user_id FROM cloud_reward_allocations WHERE batch_id=$1 ORDER BY user_id`, batch)
	if e != nil {
		return e
	}
	users := []string{}
	for rows.Next() {
		var user string
		if e = rows.Scan(&user); e != nil {
			rows.Close()
			return e
		}
		users = append(users, user)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, user := range users {
		if e = rewardNotifyTx(ctx, tx, user, "reward-confirmed:"+batch, "confirmation", "Creator rewards have been confirmed and will unlock gradually. View Earnings for the schedule."); e != nil {
			return e
		}
	}
	return nil
}
