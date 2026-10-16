CREATE TABLE cloud_reward_appeals (
 id text PRIMARY KEY, user_id text NOT NULL REFERENCES cloud_users(id),
 allocation_id text REFERENCES cloud_reward_allocations(id),
 subject text NOT NULL CHECK(subject IN('eligibility','hold','allocation','support','payout')),
 evidence text NOT NULL CHECK(length(evidence) BETWEEN 1 AND 4000),
 status text NOT NULL DEFAULT 'open' CHECK(status IN('open','upheld','declined')),
 reviewer text, resolution text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), resolved_at timestamptz,
 CHECK(reviewer IS NULL OR reviewer<>user_id)
);
CREATE INDEX cloud_appeals_owner ON cloud_reward_appeals(user_id,created_at DESC);
CREATE UNIQUE INDEX cloud_appeals_allocation_open ON cloud_reward_appeals(user_id,allocation_id) WHERE status='open' AND allocation_id IS NOT NULL;
