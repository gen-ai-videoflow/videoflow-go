CREATE TABLE cloud_reward_funding_requests (
 id text PRIMARY KEY, reference text NOT NULL UNIQUE, kind text NOT NULL CHECK(kind IN('generation','advertising','support')),
 cents bigint NOT NULL CHECK(cents>0), evidence_reference text NOT NULL CHECK(length(evidence_reference)>0),
 proposed_by text NOT NULL REFERENCES cloud_users(id), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER cloud_reward_funding_request_immutable BEFORE UPDATE OR DELETE ON cloud_reward_funding_requests FOR EACH ROW EXECUTE FUNCTION cloud_reward_immutable();
