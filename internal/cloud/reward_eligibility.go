package cloud

import (
	"context"
	"database/sql"
	"time"
)

func (s *Store) EnrollRewards(ctx context.Context, user, version string) error {
	result, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_reward_enrollments(user_id,policy_version) SELECT $1,version FROM cloud_reward_policies WHERE version=$2 AND active ON CONFLICT DO NOTHING`, user, version)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		var enrolled bool
		err = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_reward_enrollments e JOIN cloud_reward_policies p ON p.version=e.policy_version WHERE user_id=$1 AND policy_version=$2 AND p.active)`, user, version).Scan(&enrolled)
		if err != nil {
			return err
		}
		if !enrolled {
			return ErrUnavailable
		}
	}
	return nil
}

type RewardVideoEligibility struct {
	VideoID          string     `json:"video_id"`
	Title            string     `json:"title"`
	Status           string     `json:"status"`
	Reason           string     `json:"reason_code"`
	FirstPublishedAt *time.Time `json:"first_published_at"`
	WindowEndsAt     *time.Time `json:"window_ends_at"`
}

func (s *Store) RewardVideos(ctx context.Context, user string, limit, offset int) ([]RewardVideoEligibility, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrValidation
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT v.id,v.title,v.status,v.visibility,v.first_published_at,v.reward_window_seconds,EXISTS(SELECT 1 FROM cloud_reward_enrollments e JOIN cloud_reward_policies p ON p.version=e.policy_version WHERE e.user_id=$1 AND p.active) FROM cloud_videos v JOIN cloud_channels c ON c.id=v.channel_id WHERE c.user_id=$1 ORDER BY v.created_at DESC,v.id LIMIT $2 OFFSET $3`, user, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RewardVideoEligibility{}
	now := time.Now().UTC()
	for rows.Next() {
		var v RewardVideoEligibility
		var status, visibility string
		var first sql.NullTime
		var enrolled bool
		var window int64
		if err = rows.Scan(&v.VideoID, &v.Title, &status, &visibility, &first, &window, &enrolled); err != nil {
			return nil, err
		}
		v.Status = "ineligible"
		v.Reason = "not_published"
		if first.Valid {
			v.FirstPublishedAt = &first.Time
			end := first.Time.Add(time.Duration(window) * time.Second)
			v.WindowEndsAt = &end
			switch {
			case !now.Before(end):
				v.Status = "ended"
				v.Reason = "window_ended"
			case status != "published" || visibility != "public":
				v.Reason = "not_public"
			case !enrolled:
				v.Reason = "program_unavailable_or_not_enrolled"
			default:
				v.Status = "review"
				v.Reason = "evidence_review_required"
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
