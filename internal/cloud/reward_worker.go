package cloud

import (
	"context"
	"time"
)

// ReleaseRewardPage bounds a worker pass. Cursor pagination avoids one failed
// account starving all later accounts; errors are returned for operator review.
func (s *Store) ReleaseRewardPage(ctx context.Context, after string) (string, []error, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id FROM (SELECT a.user_id FROM cloud_reward_allocations a JOIN cloud_reward_releases r ON r.allocation_id=a.id WHERE r.released_cents<a.cents UNION SELECT c.user_id FROM cloud_reward_corrections c JOIN cloud_reward_correction_releases r ON r.correction_id=c.id WHERE c.approved_by IS NOT NULL AND c.cents>r.released_cents) u WHERE user_id>$1 ORDER BY user_id LIMIT 100`, after)
	if err != nil {
		return after, nil, err
	}
	users := []string{}
	for rows.Next() {
		var user string
		if err = rows.Scan(&user); err != nil {
			rows.Close()
			return after, nil, err
		}
		users = append(users, user)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return after, nil, err
	}
	failures := []error{}
	cursor := ""
	for _, user := range users {
		if err = s.ReleaseRewardVesting(ctx, user, time.Now().UTC()); err != nil {
			failures = append(failures, err)
		}
		cursor = user
	}
	return cursor, failures, nil
}
