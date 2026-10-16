CREATE TABLE cloud_reward_notifications (
 id text PRIMARY KEY, user_id text NOT NULL REFERENCES cloud_users(id), dedup_key text NOT NULL,
 kind text NOT NULL CHECK(kind IN('confirmation','hold','release','payout','appeal','support')),
 message text NOT NULL CHECK(length(message) BETWEEN 1 AND 2000),
 read_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(user_id,dedup_key)
);
CREATE INDEX cloud_reward_notifications_owner ON cloud_reward_notifications(user_id,created_at DESC);
-- External transport is deliberately unsupported: a durable terminal record is
-- visible to operators, never an unbounded retry loop or a false delivered claim.
CREATE TABLE cloud_reward_notification_deliveries (
 notification_id text PRIMARY KEY REFERENCES cloud_reward_notifications(id),
 transport text NOT NULL DEFAULT 'external' CHECK(transport='external'),
 status text NOT NULL DEFAULT 'unsupported' CHECK(status IN('unsupported','pending','delivered','dead_letter')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5),
 last_error text NOT NULL DEFAULT 'External delivery is not configured', updated_at timestamptz NOT NULL DEFAULT now()
);
