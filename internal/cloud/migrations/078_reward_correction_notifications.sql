ALTER TABLE cloud_reward_notifications DROP CONSTRAINT cloud_reward_notifications_kind_check;
ALTER TABLE cloud_reward_notifications ADD CONSTRAINT cloud_reward_notifications_kind_check CHECK(kind IN('confirmation','hold','release','payout','appeal','support','correction'));
