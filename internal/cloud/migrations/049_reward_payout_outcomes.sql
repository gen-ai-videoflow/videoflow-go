CREATE TABLE cloud_reward_payout_run_accounts (
 run_id text NOT NULL REFERENCES cloud_reward_payout_runs(id),user_id text NOT NULL REFERENCES cloud_users(id),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN('pending','prepared','skipped','failed')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5),batch_id text REFERENCES cloud_payout_batches(id),error text NOT NULL DEFAULT '',updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(run_id,user_id)
);
