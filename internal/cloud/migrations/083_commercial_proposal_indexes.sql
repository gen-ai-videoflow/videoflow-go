CREATE INDEX cloud_commercial_pending ON cloud_commercial_proposals(created_at DESC) WHERE approved_by IS NULL;
