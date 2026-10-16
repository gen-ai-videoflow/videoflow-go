package cloud

import (
	"context"
	"time"
)

// ReleaseRewardPage bounds a worker pass. Cursor pagination avoids one failed
// account starving all later accounts; errors are returned for operator review.
func (s *Store) ReleaseRewardPage(ctx context.Context, after string) (string, []error, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT a.user_id FROM cloud_reward_allocations a JOIN cloud_reward_releases r ON r.allocation_id=a.id WHERE a.user_id>$1 AND r.released_cents<a.cents ORDER BY a.user_id LIMIT 100`, after)
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
